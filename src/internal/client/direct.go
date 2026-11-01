package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"go.uber.org/zap"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/bzdvdn/kvn-ws/src/internal/routing"
)

const directDNSWorkers = 8

// lookupIP is injectable so tests can simulate a slow resolver.
var lookupIP = func(ctx context.Context, network, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, network, host)
}

type directDNSQuery struct {
	domain   string
	srcIP    net.IP
	dstIP    net.IP
	srcPort  uint16
	id       uint16
	question dnsmessage.Question
}

type directCache struct {
	mu    sync.Mutex
	until map[netip.Addr]time.Time
}

func newDirectCache() *directCache {
	return &directCache{until: make(map[netip.Addr]time.Time)}
}

func (c *directCache) Add(ip netip.Addr, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	c.mu.Lock()
	c.until[ip] = time.Now().Add(ttl)
	c.mu.Unlock()
}

func (c *directCache) Has(ip netip.Addr) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	exp, ok := c.until[ip]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(c.until, ip)
		return false
	}
	return true
}

func (c *directCache) HasTarget(target string) bool {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		host = target
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && c.Has(ip)
}

// handleDirectDNS intercepts DNS queries for excluded domains, dispatches
// resolution off the packet pump and marks the returned IPs for direct TCP/UDP.
func (c *Client) handleDirectDNS(ctx context.Context, packet []byte, dev interface {
	Write([]byte) (int, error)
}, rules *routing.RuleSet, cache *directCache) bool {
	if rules == nil || len(packet) < 28 || packet[0]>>4 != 4 || packet[9] != 17 {
		return false
	}
	ihl := int(packet[0]&0x0f) * 4
	if len(packet) < ihl+8 {
		return false
	}
	dstPort := binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
	if dstPort != 53 {
		return false
	}
	srcIP := net.IP(packet[12:16]).To4()
	dstIP := net.IP(packet[16:20]).To4()
	srcPort := binary.BigEndian.Uint16(packet[ihl : ihl+2])
	query := packet[ihl+8:]
	if srcIP == nil || dstIP == nil {
		return false
	}

	var parser dnsmessage.Parser
	header, err := parser.Start(query)
	if err != nil {
		return false
	}
	question, err := parser.Question()
	if err != nil {
		return false
	}
	domain := question.Name.String()
	domain = domain[:len(domain)-1] // drop trailing dot
	if rules.MatchDomain(domain) != routing.RouteDirect {
		return false
	}
	c.logger.Debug("v2 direct DNS", zap.String("domain", domain))

	c.dispatchDirectDNS(ctx, directDNSQuery{
		domain:   domain,
		srcIP:    append(net.IP(nil), srcIP...),
		dstIP:    append(net.IP(nil), dstIP...),
		srcPort:  srcPort,
		id:       header.ID,
		question: question,
	}, dev, cache)
	return true
}

func (c *Client) dispatchDirectDNS(ctx context.Context, q directDNSQuery, dev interface {
	Write([]byte) (int, error)
}, cache *directCache) {
	if c.directDNSSem == nil {
		c.resolveDirectDNS(ctx, q, dev, cache)
		return
	}
	select {
	case c.directDNSSem <- struct{}{}:
		go func() {
			defer func() { <-c.directDNSSem }()
			c.resolveDirectDNS(ctx, q, dev, cache)
		}()
	default:
		c.logger.Debug("v2 direct dns busy; dropping query", zap.String("domain", q.domain))
	}
}

func (c *Client) resolveDirectDNS(ctx context.Context, q directDNSQuery, dev interface {
	Write([]byte) (int, error)
}, cache *directCache) {
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ips, err := lookupIP(rctx, "ip4", q.domain)
	if err != nil || len(ips) == 0 {
		return
	}

	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{
		ID: q.id, Response: true, RecursionAvailable: true,
	})
	builder.EnableCompression()
	if err := builder.StartQuestions(); err != nil {
		return
	}
	if err := builder.Question(q.question); err != nil {
		return
	}
	if err := builder.StartAnswers(); err != nil {
		return
	}
	for _, raw := range ips {
		v4 := raw.To4()
		if v4 == nil {
			continue
		}
		var a [4]byte
		copy(a[:], v4)
		cache.Add(netip.AddrFrom4(a), 60*time.Second)
		c.logger.Debug("v2 direct DNS address", zap.String("domain", q.domain), zap.String("ip", v4.String()))
		if err := builder.AResource(dnsmessage.ResourceHeader{
			Name: q.question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60,
		}, dnsmessage.AResource{A: a}); err != nil {
			return
		}
	}
	resp, err := builder.Finish()
	if err != nil || len(resp) == 0 {
		return
	}
	reply := buildUDPReply(q.dstIP, q.srcIP, 53, q.srcPort, resp)
	_, _ = dev.Write(reply)
}

type directUDPFlow struct {
	conn *net.UDPConn
}

func newDirectUDPFlow(target string) (*directUDPFlow, error) {
	addr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}
	return &directUDPFlow{conn: conn}, nil
}

func (f *directUDPFlow) WritePacket(payload []byte) error {
	_, err := f.conn.Write(payload)
	return err
}

func (f *directUDPFlow) ReadPacket() ([]byte, error) {
	buf := make([]byte, 65535)
	n, err := f.conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (f *directUDPFlow) Close() error { return f.conn.Close() }

var _ = io.EOF
var _ = fmt.Sprintf
