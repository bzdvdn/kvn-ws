package relay

import (
	"net/netip"
	"testing"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
)

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
