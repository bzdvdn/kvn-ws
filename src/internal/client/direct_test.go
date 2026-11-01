package client

import (
	"context"
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
)

type fakeDev struct {
	ch chan []byte
}

func (d *fakeDev) Write(p []byte) (int, error) {
	cp := append([]byte(nil), p...)
	select {
	case d.ch <- cp:
	default:
	}
	return len(p), nil
}

func buildDNSQuery(t *testing.T, domain string) []byte {
	t.Helper()
	name, err := dnsmessage.NewName(domain)
	if err != nil {
		t.Fatalf("new name: %v", err)
	}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 0x1234, RecursionDesired: true})
	b.EnableCompression()
	if err := b.StartQuestions(); err != nil {
		t.Fatalf("start questions: %v", err)
	}
	if err := b.Question(dnsmessage.Question{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}); err != nil {
		t.Fatalf("question: %v", err)
	}
	msg, err := b.Finish()
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	return msg
}

func wrapUDP(src, dst net.IP, srcPort, dstPort uint16, payload []byte) []byte {
	total := 20 + 8 + len(payload)
	pkt := make([]byte, total)
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], uint16(total))
	pkt[9] = 17
	copy(pkt[12:16], src.To4())
	copy(pkt[16:20], dst.To4())
	binary.BigEndian.PutUint16(pkt[20:22], srcPort)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	binary.BigEndian.PutUint16(pkt[24:26], uint16(8+len(payload)))
	copy(pkt[28:], payload)
	return pkt
}

// TestHandleDirectDNSEscapesReadLoop proves a slow resolver does not block the
// packet pump: the handler returns immediately and the reply is written later.
func TestHandleDirectDNSEscapesReadLoop(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()

	started := make(chan struct{})
	original := lookupIP
	lookupIP = func(ctx context.Context, _, _ string) ([]net.IP, error) {
		select {
		case <-started:
		default:
			close(started)
		}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	defer func() { lookupIP = original }()

	c := New(&config.V2ClientConfig{}, zap.NewNop())
	rules, err := routing.NewRuleSet(&config.RoutingCfg{
		DefaultRoute:   "server",
		ExcludeDomains: []string{"direct.example"},
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("rules: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dev := &fakeDev{ch: make(chan []byte, 4)}
	cache := newDirectCache()

	// A non-excluded domain is not intercepted and never reaches the resolver.
	other := wrapUDP(net.ParseIP("10.66.0.10"), net.ParseIP("1.1.1.1"), 5555, 53, buildDNSQuery(t, "other.example."))
	if c.handleDirectDNS(ctx, other, dev, rules, cache) {
		t.Fatal("non-excluded domain must not be intercepted")
	}

	packet := wrapUDP(net.ParseIP("10.66.0.10"), net.ParseIP("1.1.1.1"), 5555, 53, buildDNSQuery(t, "direct.example."))
	start := time.Now()
	if !c.handleDirectDNS(ctx, packet, dev, rules, cache) {
		t.Fatal("excluded domain must be intercepted")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("handleDirectDNS blocked for %v, want immediate dispatch", elapsed)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("resolver was not invoked")
	}

	unblock()
	select {
	case reply := <-dev.ch:
		if len(reply) < 28 {
			t.Fatalf("reply too short: %d", len(reply))
		}
		var p dnsmessage.Parser
		hdr, err := p.Start(reply[28:])
		if err != nil {
			t.Fatalf("parse reply: %v", err)
		}
		if hdr.ID != 0x1234 || !hdr.Response {
			t.Fatalf("reply header id=%#x response=%v", hdr.ID, hdr.Response)
		}
		if _, err := p.Question(); err != nil {
			t.Fatalf("reply question: %v", err)
		}
		if err := p.SkipAllQuestions(); err != nil {
			t.Fatalf("skip questions: %v", err)
		}
		ans, err := p.Answer()
		if err != nil {
			t.Fatalf("reply answer: %v", err)
		}
		ar, ok := ans.Body.(*dnsmessage.AResource)
		if !ok {
			t.Fatalf("answer type = %T, want *dnsmessage.AResource", ans.Body)
		}
		if got := net.IP(ar.A[:]).String(); got != "93.184.216.34" {
			t.Fatalf("answer = %s, want 93.184.216.34", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no DNS reply written to TUN")
	}
}
