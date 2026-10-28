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

// handleDirectDNS intercepts DNS queries for excluded domains, resolves them
// outside the tunnel and marks the returned IPs for direct TCP/UDP.
func (c *Client) handleDirectDNS(packet []byte, dev interface {
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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", domain)
	if err != nil || len(ips) == 0 {
		return false
	}

	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{
		ID: header.ID, Response: true, RecursionAvailable: true,
	})
	builder.EnableCompression()
	if err := builder.StartQuestions(); err != nil {
		return false
	}
	if err := builder.Question(question); err != nil {
		return false
	}
	if err := builder.StartAnswers(); err != nil {
		return false
	}
	for _, raw := range ips {
		v4 := raw.To4()
		if v4 == nil {
			continue
		}
		var a [4]byte
		copy(a[:], v4)
		cache.Add(netip.AddrFrom4(a), 60*time.Second)
		c.logger.Debug("v2 direct DNS address", zap.String("domain", domain), zap.String("ip", v4.String()))
		if err := builder.AResource(dnsmessage.ResourceHeader{
			Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60,
		}, dnsmessage.AResource{A: a}); err != nil {
			return false
		}
	}
	resp, err := builder.Finish()
	if err != nil || len(resp) == 0 {
		return false
	}
	reply := buildUDPReply(dstIP, srcIP, 53, srcPort, resp)
	_, _ = dev.Write(reply)
	return true
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
