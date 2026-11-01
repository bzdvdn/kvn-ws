package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestNewQUICClientBoundedOnBlackhole proves an unresponsive UDP endpoint
// cannot stall the client startup indefinitely.
func TestNewQUICClientBoundedOnBlackhole(t *testing.T) {
	sink, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer func() { _ = sink.Close() }()
	go func() {
		buf := make([]byte, 2048)
		for {
			if _, _, rerr := sink.ReadFromUDP(buf); rerr != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tlsCfg := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h3"}, MinVersion: tls.VersionTLS13}

	start := time.Now()
	_, err = NewQUICClient(ctx, sink.LocalAddr().String(), "token", tlsCfg, zap.NewNop())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("NewQUICClient succeeded against a blackhole endpoint")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("QUIC dial took %v, want bounded by handshake/ctx timeout", elapsed)
	}
}

// TestQUICClientSurvivesDialContextCancel proves the datagram read loop is not
// tied to the short-lived dial timeout used during startup. Canceling the dial
// context after the client is created must not blackhole UDP flows.
func TestQUICClientSurvivesDialContextCancel(t *testing.T) {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = udpConn.Close() }()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, rerr := udpConn.ReadFromUDP(buf)
			if rerr != nil {
				return
			}
			_, _ = udpConn.WriteToUDP(buf[:n], addr)
		}
	}()

	cert := selfSignedCert(t)
	server, err := ListenQUIC("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h3"}},
		func(token string) (string, bool) { return "t", token == "secret" }, zap.NewNop())
	if err != nil {
		t.Fatalf("ListenQUIC: %v", err)
	}
	serveCtx, serveCancel := context.WithCancel(context.Background())
	defer serveCancel()
	go func() { _ = server.Serve(serveCtx) }()

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := NewQUICClient(dialCtx, server.Addr().String(), "secret", &tls.Config{InsecureSkipVerify: true}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewQUICClient: %v", err)
	}
	defer func() { _ = client.Close() }()
	dialCancel()

	flow, err := client.DialFlow(context.Background(), udpConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("DialFlow: %v", err)
	}
	defer func() { _ = flow.Close() }()

	payload := []byte("video-udp")
	if err := flow.WritePacket(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	type result struct {
		b   []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		got, rerr := flow.ReadPacket()
		ch <- result{got, rerr}
	}()
	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("read: %v", res.err)
		}
		if !bytes.Equal(res.b, payload) {
			t.Fatalf("echo = %q, want %q", res.b, payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no datagram reply: QUIC read loop was canceled with the dial context")
	}
}

func newQUICPair(t *testing.T) *QUICClient {
	t.Helper()
	cert := selfSignedCert(t)
	server, err := ListenQUIC("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h3"}},
		func(token string) (string, bool) { return "t", token == "secret" }, zap.NewNop())
	if err != nil {
		t.Fatalf("ListenQUIC: %v", err)
	}
	serveCtx, serveCancel := context.WithCancel(context.Background())
	t.Cleanup(serveCancel)
	go func() { _ = server.Serve(serveCtx) }()

	client, err := NewQUICClient(context.Background(), server.Addr().String(), "secret",
		&tls.Config{InsecureSkipVerify: true}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewQUICClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestQUICFlowRejectsOversizeDatagram(t *testing.T) {
	client := newQUICPair(t)
	flow, err := client.DialFlow(context.Background(), "127.0.0.1:9")
	if err != nil {
		t.Fatalf("DialFlow: %v", err)
	}
	defer func() { _ = flow.Close() }()
	if err := flow.WritePacket(make([]byte, MaxQUICPayload+1)); !errors.Is(err, ErrDatagramTooLarge) {
		t.Fatalf("WritePacket err = %v, want ErrDatagramTooLarge", err)
	}
}

func TestQUICDialFlowFailsAfterClose(t *testing.T) {
	client := newQUICPair(t)
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := client.DialFlow(context.Background(), "127.0.0.1:53"); err == nil {
		t.Fatal("DialFlow succeeded on a closed QUIC connection")
	}
}

// TestQUICFallbackForLargeReplies proves the server asks the client to move a
// flow to the reliable HTTP/2 UDP stream when a reply exceeds the datagram
// budget (video-sized UDP packets).
func TestQUICFallbackForLargeReplies(t *testing.T) {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = udpConn.Close() }()
	go func() {
		buf := make([]byte, 2048)
		for {
			if _, addr, rerr := udpConn.ReadFromUDP(buf); rerr != nil {
				return
			} else {
				_, _ = udpConn.WriteToUDP(make([]byte, MaxQUICPayload+100), addr)
			}
		}
	}()

	client := newQUICPair(t)
	fallback := make(chan string, 1)
	client.SetFallbackHandler(func(target string) {
		select {
		case fallback <- target:
		default:
		}
	})

	target := udpConn.LocalAddr().String()
	flow, err := client.DialFlow(context.Background(), target)
	if err != nil {
		t.Fatalf("DialFlow: %v", err)
	}
	defer func() { _ = flow.Close() }()
	if err := flow.WritePacket([]byte("hi")); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case got := <-fallback:
		if got != target {
			t.Fatalf("fallback target = %q, want %q", got, target)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not request HTTP/2 fallback for a large reply")
	}
}
