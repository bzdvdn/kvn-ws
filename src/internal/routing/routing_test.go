package routing

import (
	"net/netip"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

// @sk-test routing-split-tunnel#T2.5: TestDefaultRouteServer (AC-001)
func TestDefaultRouteServer(t *testing.T) {
	cfg := &config.RoutingCfg{DefaultRoute: "server"}
	rs, err := NewRuleSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ip := netip.MustParseAddr("8.8.8.8")
	if action := rs.Route(ip); action != RouteServer {
		t.Errorf("expected RouteServer, got %d", action)
	}
}

// @sk-test routing-split-tunnel#T2.5: TestDefaultRouteDirect (AC-001)
func TestDefaultRouteDirect(t *testing.T) {
	cfg := &config.RoutingCfg{DefaultRoute: "direct"}
	rs, err := NewRuleSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ip := netip.MustParseAddr("8.8.8.8")
	if action := rs.Route(ip); action != RouteDirect {
		t.Errorf("expected RouteDirect, got %d", action)
	}
}

// @sk-test routing-split-tunnel#T2.5: TestCIDRInclude (AC-002)
func TestCIDRInclude(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:  "direct",
		IncludeRanges: []string{"10.0.0.0/8"},
	}
	rs, err := NewRuleSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	included := netip.MustParseAddr("10.10.10.10")
	if action := rs.Route(included); action != RouteServer {
		t.Errorf("expected RouteServer for included IP, got %d", action)
	}
	excluded := netip.MustParseAddr("8.8.8.8")
	if action := rs.Route(excluded); action != RouteDirect {
		t.Errorf("expected RouteDirect for non-included IP, got %d", action)
	}
}

// @sk-test routing-split-tunnel#T2.5: TestCIDRExclude (AC-002)
func TestCIDRExclude(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:  "server",
		ExcludeRanges: []string{"10.0.0.0/8"},
	}
	rs, err := NewRuleSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	excluded := netip.MustParseAddr("10.10.10.10")
	if action := rs.Route(excluded); action != RouteDirect {
		t.Errorf("expected RouteDirect for excluded IP, got %d", action)
	}
	included := netip.MustParseAddr("8.8.8.8")
	if action := rs.Route(included); action != RouteServer {
		t.Errorf("expected RouteServer for non-excluded IP, got %d", action)
	}
}

// @sk-test routing-split-tunnel#T2.5: TestExactIPInclude (AC-003)
func TestExactIPInclude(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute: "direct",
		IncludeIPs:   []string{"192.168.1.100"},
	}
	rs, err := NewRuleSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	included := netip.MustParseAddr("192.168.1.100")
	if action := rs.Route(included); action != RouteServer {
		t.Errorf("expected RouteServer for included IP, got %d", action)
	}
	excluded := netip.MustParseAddr("192.168.1.101")
	if action := rs.Route(excluded); action != RouteDirect {
		t.Errorf("expected RouteDirect for non-included IP, got %d", action)
	}
}

// @sk-test routing-split-tunnel#T2.5: TestOrderedExcludeWins (AC-006)
func TestOrderedExcludeWins(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:  "direct",
		ExcludeIPs:    []string{"10.10.10.10"},
		IncludeRanges: []string{"10.0.0.0/8"},
	}
	rs, err := NewRuleSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	excluded := netip.MustParseAddr("10.10.10.10")
	if action := rs.Route(excluded); action != RouteDirect {
		t.Errorf("expected RouteDirect for excluded IP (exclude wins), got %d", action)
	}
	included := netip.MustParseAddr("10.10.10.11")
	if action := rs.Route(included); action != RouteServer {
		t.Errorf("expected RouteServer for included IP, got %d", action)
	}
}

// @sk-test routing-split-tunnel#T2.5: TestEmptyRules (AC-001)
func TestEmptyRules(t *testing.T) {
	cfg := &config.RoutingCfg{DefaultRoute: "server", ExcludeRanges: []string{}, IncludeRanges: []string{}}
	rs, err := NewRuleSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ip := netip.MustParseAddr("1.2.3.4")
	if action := rs.Route(ip); action != RouteServer {
		t.Errorf("expected RouteServer for empty rules, got %d", action)
	}
}

// @sk-test routing-split-tunnel#T2.5: TestZeroPrefix (AC-002)
func TestZeroPrefix(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:  "direct",
		IncludeRanges: []string{"0.0.0.0/0"},
	}
	rs, err := NewRuleSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ip := netip.MustParseAddr("1.2.3.4")
	if action := rs.Route(ip); action != RouteServer {
		t.Errorf("expected RouteServer for /0 include, got %d", action)
	}
}

// @sk-test routing-split-tunnel#T4.2: TestInvalidCIDR (AC-002)
func TestInvalidCIDR(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:  "server",
		IncludeRanges: []string{"invalid-cidr"},
	}
	_, err := NewRuleSet(cfg)
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

// @sk-test routing-split-tunnel#T4.2: TestInvalidIP (AC-003)
func TestInvalidIP(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
		IncludeIPs:   []string{"not-an-ip"},
	}
	_, err := NewRuleSet(cfg)
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

// @sk-test routing-split-tunnel#T4.2: TestNilConfig (AC-006)
func TestNilConfig(t *testing.T) {
	_, err := NewRuleSet(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

// @sk-test routing-split-tunnel#T4.2: TestBothExcludeAndInclude (AC-006)
func TestBothExcludeAndInclude(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:  "direct",
		ExcludeRanges: []string{"10.0.0.0/8"},
		IncludeRanges: []string{"10.0.0.0/8"},
	}
	rs, err := NewRuleSet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ip := netip.MustParseAddr("10.10.10.10")
	// exclude is checked first, so it should be direct
	if action := rs.Route(ip); action != RouteDirect {
		t.Errorf("expected RouteDirect (exclude wins), got %d", action)
	}
}

// @sk-test routing-split-tunnel#T4.2: TestIsDNSQuery (AC-008)
func TestIsDNSQuery(t *testing.T) {
	// Build a minimal IPv4 UDP packet to port 53
	pkt := make([]byte, 28)
	pkt[0] = 0x45                               // ver=4, ihl=5
	pkt[9] = 17                                  // UDP
	pkt[12] = 10; pkt[13] = 0; pkt[14] = 0; pkt[15] = 1 // src IP
	pkt[16] = 8; pkt[17] = 8; pkt[18] = 8; pkt[19] = 8 // dst IP
	pkt[20] = 0; pkt[21] = 53                           // src port (not used)
	pkt[22] = 0; pkt[23] = 53                           // dst port = 53

	if !isDNSQuery(pkt) {
		t.Error("expected DNS query detection")
	}
}

// @sk-test routing-split-tunnel#T4.2: TestIsNotDNSQuery (AC-008)
func TestIsNotDNSQuery(t *testing.T) {
	// TCP packet to port 53 should not be detected as DNS query (UDP only)
	pkt := make([]byte, 28)
	pkt[0] = 0x45
	pkt[9] = 6                                   // TCP
	pkt[16] = 8; pkt[17] = 8; pkt[18] = 8; pkt[19] = 8
	pkt[22] = 0; pkt[23] = 53

	if isDNSQuery(pkt) {
		t.Error("expected no DNS query for TCP")
	}
}
