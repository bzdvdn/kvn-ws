package relay

import (
	"encoding/binary"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
)

// @sk-test arch-fix-critical-paths#T3.2: insertDNSCache enforces size limit (AC-003)
func TestInsertDNSCacheLimit(t *testing.T) {
	r := &Relay{
		dnsCache: make(map[netip.Addr]time.Time),
		logger:   zap.NewNop(),
	}

	// Insert more than limit
	now := time.Now()
	for i := 0; i < defaultMaxDNSCacheSize+100; i++ {
		var ip [4]byte
		binary.BigEndian.PutUint32(ip[:], uint32(i+1))
		r.insertDNSCache(netip.AddrFrom4(ip), now.Add(time.Hour))
	}

	if len(r.dnsCache) > defaultMaxDNSCacheSize {
		t.Fatalf("dns cache size %d exceeds limit %d", len(r.dnsCache), defaultMaxDNSCacheSize)
	}
}

// @sk-test arch-fix-critical-paths#T3.2: insertDNSCache removes expired entries first (AC-003)
func TestInsertDNSCacheEvictExpired(t *testing.T) {
	r := &Relay{
		dnsCache: make(map[netip.Addr]time.Time),
		logger:   zap.NewNop(),
	}

	now := time.Now()

	// Fill cache with mostly expired entries
	for i := 0; i < defaultMaxDNSCacheSize; i++ {
		var ip [4]byte
		binary.BigEndian.PutUint32(ip[:], uint32(i+1))
		r.dnsCache[netip.AddrFrom4(ip)] = now.Add(-time.Hour) // expired
	}

	// Insert one new entry — should evict expired ones
	var newIP [4]byte
	binary.BigEndian.PutUint32(newIP[:], 999999)
	r.insertDNSCache(netip.AddrFrom4(newIP), now.Add(time.Hour))

	if len(r.dnsCache) > defaultMaxDNSCacheSize {
		t.Fatalf("dns cache size %d exceeds limit %d", len(r.dnsCache), defaultMaxDNSCacheSize)
	}
}

// @sk-test arch-fix-critical-paths#T3.1: getDNSConn returns conn from pool (AC-002)
func TestGetDNSConnPool(t *testing.T) {
	var dialCount int32
	r := &Relay{
		dnsUpstream: "127.0.0.1:15353",
		dnsConnPool: &sync.Pool{
			New: func() interface{} {
				atomic.AddInt32(&dialCount, 1)
				conn, err := net.DialTimeout("udp", "127.0.0.1:15353", 100*time.Millisecond)
				if err != nil {
					return nil
				}
				return conn
			},
		},
		logger: zap.NewNop(),
	}

	// First call should dial
	conn1 := r.getDNSConn()
	if conn1 == nil {
		// Expected: no DNS server on 127.0.0.1:15353
		// But pool.New was called
		if dialCount == 0 {
			t.Error("getDNSConn: pool.New was not called")
		}
		return
	}
	defer conn1.Close()

	// Put back and get again — should reuse the connection
	r.putDNSConn(conn1)
	conn2 := r.getDNSConn()
	if conn2 == nil {
		t.Fatal("getDNSConn returned nil after pool put")
	}
	defer conn2.Close()

	if dialCount > 2 {
		t.Errorf("getDNSConn: too many dials (%d), expected pool reuse", dialCount)
	}
}

// @sk-test arch-fix-critical-paths#T1.1: TestBuildDNSRespPacketOverflow (AC-005)
func TestBuildDNSRespPacketOverflow(t *testing.T) {
	dnsResp := make([]byte, 70000)
	// Valid IPv4 header + UDP header + huge DNS payload
	pkt := []byte{
		0x45, 0x00, 0x00, 0x14,
		0x00, 0x00, 0x00, 0x00,
		0x40, 0x11, 0x00, 0x00,
		0x0a, 0x00, 0x00, 0x01,
		0xac, 0x10, 0x00, 0x01,
	}
	out := buildDNSRespPacket(pkt, dnsResp)
	if out != nil {
		t.Fatal("buildDNSRespPacket: expected nil for oversized payload, got non-nil")
	}
}

// @sk-test arch-fix-critical-paths#T1.2: TestIsDNSQueryShort (AC-006)
func TestIsDNSQueryShort(t *testing.T) {
	if isDNSQuery(nil) {
		t.Error("isDNSQuery(nil) = true, want false")
	}
	if isDNSQuery([]byte{}) {
		t.Error("isDNSQuery(empty) = true, want false")
	}
	if isDNSQuery([]byte{0x45}) {
		t.Error("isDNSQuery(1 byte) = true, want false")
	}
	// 19 bytes — one less than minimum IPv4 header
	pkt := make([]byte, 19)
	if isDNSQuery(pkt) {
		t.Error("isDNSQuery(19 bytes) = true, want false")
	}
}

// @sk-test arch-fix-critical-paths#T1.2: TestIsDNSQueryIHLZero (AC-006)
func TestIsDNSQueryIHLZero(t *testing.T) {
	// IHL = 0 (first nibble), should be rejected
	pkt := make([]byte, 20)
	if isDNSQuery(pkt) {
		t.Error("isDNSQuery with IHL=0 = true, want false")
	}
}

// @sk-test arch-fix-critical-paths#T1.2: TestExtractDestIPZero (AC-006)
func TestExtractDestIPZero(t *testing.T) {
	_, ok := extractDestIP(nil)
	if ok {
		t.Error("extractDestIP(nil) = true, want false")
	}
	_, ok = extractDestIP([]byte{})
	if ok {
		t.Error("extractDestIP(empty) = true, want false")
	}
}

func newTestLogger() (*zap.Logger, error) {
	return zap.NewNop(), nil
}

// @sk-test relay-terminator#T4.2: extractDestIP IPv4 (AC-002)
func TestExtractDestIPv4(t *testing.T) {
	pkt := []byte{
		0x45, 0x00, 0x00, 0x14, // version/IHL, DSCP, total length
		0x00, 0x00, 0x00, 0x00, // ID, flags/fragment
		0x40, 0x06, 0x00, 0x00, // TTL, protocol, checksum
		0x0a, 0x00, 0x00, 0x01, // src: 10.0.0.1
		0xac, 0x10, 0x00, 0x01, // dst: 172.16.0.1
	}
	ip, ok := extractDestIP(pkt)
	if !ok {
		t.Fatal("extractDestIP returned false, want true")
	}
	want := netip.AddrFrom4([4]byte{0xac, 0x10, 0x00, 0x01})
	if ip != want {
		t.Fatalf("extractDestIP = %s, want %s", ip, want)
	}
}

// @sk-test relay-terminator#T4.2: extractDestIP IPv6 (AC-002)
func TestExtractDestIPv6(t *testing.T) {
	pkt := []byte{
		0x60, 0x00, 0x00, 0x00, // version, traffic class, flow label
		0x00, 0x14, 0x06, 0x40, // payload length, next header, hop limit
		// src: 2001:db8::1
		0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		// dst: 2001:db8::2
		0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
	}
	ip, ok := extractDestIP(pkt)
	if !ok {
		t.Fatal("extractDestIP returned false, want true")
	}
	want := netip.AddrFrom16([16]byte{
		0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
	})
	if ip != want {
		t.Fatalf("extractDestIP = %s, want %s", ip, want)
	}
}

// @sk-test relay-terminator#T4.2: extractDestIP short packet (AC-002)
func TestExtractDestIPShort(t *testing.T) {
	_, ok := extractDestIP([]byte{0x45})
	if ok {
		t.Fatal("extractDestIP returned true for short packet, want false")
	}
}

// @sk-test relay-terminator#T4.2: extractDestIP non-IP (AC-002)
func TestExtractDestIPNonIP(t *testing.T) {
	_, ok := extractDestIP([]byte{0x00, 0x00, 0x00, 0x00})
	if ok {
		t.Fatal("extractDestIP returned true for non-IP, want false")
	}
}

// @sk-test relay-terminator#T4.2: newDirectRuleSet with direct ranges (AC-002)
func TestNewDirectRuleSet(t *testing.T) {
	cfg := &config.RelayRoutingCfg{
		DirectRanges: []string{"10.0.0.0/8"},
	}
	logger, _ := newTestLogger()
	rs, err := newDirectRuleSet(cfg, logger)
	if err != nil {
		t.Fatalf("newDirectRuleSet: %v", err)
	}
	if rs == nil {
		t.Fatal("newDirectRuleSet returned nil")
	}
	// 10.0.0.1 should be RouteDirect
	action := rs.Route(netip.MustParseAddr("10.0.0.1"))
	if action != routing.RouteDirect {
		t.Fatalf("Route(10.0.0.1) = %d, want RouteDirect(%d)", action, routing.RouteDirect)
	}
	// 8.8.8.8 should be RouteServer (default)
	action = rs.Route(netip.MustParseAddr("8.8.8.8"))
	if action != routing.RouteServer {
		t.Fatalf("Route(8.8.8.8) = %d, want RouteServer(%d)", action, routing.RouteServer)
	}
}
