package routing

import (
	"encoding/binary"
	"net/netip"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/dns"
)

var nopLogger = zap.NewNop()

// @sk-test routing-split-tunnel#T2.5: TestDefaultRouteServer (AC-001)
func TestDefaultRouteServer(t *testing.T) {
	cfg := &config.RoutingCfg{DefaultRoute: "server"}
	rs, err := NewRuleSet(cfg, nopLogger)
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
	rs, err := NewRuleSet(cfg, nopLogger)
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
	rs, err := NewRuleSet(cfg, nopLogger)
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
	rs, err := NewRuleSet(cfg, nopLogger)
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
	rs, err := NewRuleSet(cfg, nopLogger)
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
	rs, err := NewRuleSet(cfg, nopLogger)
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
	rs, err := NewRuleSet(cfg, nopLogger)
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
	rs, err := NewRuleSet(cfg, nopLogger)
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
	_, err := NewRuleSet(cfg, nopLogger)
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
	_, err := NewRuleSet(cfg, nopLogger)
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

// @sk-test routing-split-tunnel#T4.2: TestNilConfig (AC-006)
func TestNilConfig(t *testing.T) {
	_, err := NewRuleSet(nil, nopLogger)
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
	rs, err := NewRuleSet(cfg, nopLogger)
	if err != nil {
		t.Fatal(err)
	}
	ip := netip.MustParseAddr("10.10.10.10")
	// exclude is checked first, so it should be direct
	if action := rs.Route(ip); action != RouteDirect {
		t.Errorf("expected RouteDirect (exclude wins), got %d", action)
	}
}

// @sk-test dns-routing#T7.2: RuleSet MatchDomain exclude (AC-001)
func TestRuleSetMatchDomainExclude(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:   "server",
		ExcludeDomains: []string{".ru", ".ozon.ru"},
	}
	rs, err := NewRuleSet(cfg, nopLogger)
	if err != nil {
		t.Fatal(err)
	}

	if action := rs.MatchDomain("hh.ru"); action != RouteDirect {
		t.Errorf("expected RouteDirect for hh.ru, got %d", action)
	}
	if action := rs.MatchDomain("api.ozon.ru"); action != RouteDirect {
		t.Errorf("expected RouteDirect for api.ozon.ru, got %d", action)
	}
	if action := rs.MatchDomain("google.com"); action != RouteNone {
		t.Errorf("expected RouteNone for google.com, got %d", action)
	}
}

// @sk-test dns-routing#T7.2: RuleSet MatchDomain include (AC-002)
func TestRuleSetMatchDomainInclude(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:   "direct",
		IncludeDomains: []string{".corp"},
	}
	rs, err := NewRuleSet(cfg, nopLogger)
	if err != nil {
		t.Fatal(err)
	}

	if action := rs.MatchDomain("internal.corp"); action != RouteServer {
		t.Errorf("expected RouteServer for internal.corp, got %d", action)
	}
	if action := rs.MatchDomain("google.com"); action != RouteNone {
		t.Errorf("expected RouteNone for google.com, got %d", action)
	}
}

// @sk-test dns-routing#T7.2: RuleSet MatchDomain no suffix domains (AC-003)
func TestRuleSetMatchDomainEmpty(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute: "server",
	}
	rs, err := NewRuleSet(cfg, nopLogger)
	if err != nil {
		t.Fatal(err)
	}

	if action := rs.MatchDomain("hh.ru"); action != RouteNone {
		t.Errorf("expected RouteNone for empty suffix rules, got %d", action)
	}
}

// @sk-test routing-split-tunnel#T4.2: TestIsDNSQuery (AC-008)
func TestIsDNSQuery(t *testing.T) {
	// Build a minimal IPv4 UDP packet to port 53
	pkt := make([]byte, 28)
	pkt[0] = 0x45 // ver=4, ihl=5
	pkt[9] = 17   // UDP
	pkt[12] = 10
	pkt[13] = 0
	pkt[14] = 0
	pkt[15] = 1 // src IP
	pkt[16] = 8
	pkt[17] = 8
	pkt[18] = 8
	pkt[19] = 8 // dst IP
	pkt[20] = 0
	pkt[21] = 53 // src port (not used)
	pkt[22] = 0
	pkt[23] = 53 // dst port = 53

	if !isDNSQuery(pkt) {
		t.Error("expected DNS query detection")
	}
}

// @sk-test routing-split-tunnel#T4.2: TestIsNotDNSQuery (AC-008)
func TestIsNotDNSQuery(t *testing.T) {
	// TCP packet to port 53 should not be detected as DNS query (UDP only)
	pkt := make([]byte, 28)
	pkt[0] = 0x45
	pkt[9] = 6 // TCP
	pkt[16] = 8
	pkt[17] = 8
	pkt[18] = 8
	pkt[19] = 8
	pkt[22] = 0
	pkt[23] = 53

	if isDNSQuery(pkt) {
		t.Error("expected no DNS query for TCP")
	}
}

// @sk-test ipv6-dual-stack#T4.1: TestParseDstIP6 (AC-005)
func TestParseDstIP6(t *testing.T) {
	pkt := make([]byte, 40)
	pkt[0] = 0x60 // IPv6, version 6
	// dst addr at offset 24: fd00::2
	pkt[24] = 0xfd
	pkt[39] = 0x02

	addr, err := parseDstIP6(pkt)
	if err != nil {
		t.Fatalf("parseDstIP6: %v", err)
	}
	expected := netip.MustParseAddr("fd00::2")
	if addr != expected {
		t.Errorf("got %s, want %s", addr, expected)
	}
}

// @sk-test ipv6-dual-stack#T4.1: TestParseDstIP6Truncated (AC-005)
func TestParseDstIP6Truncated(t *testing.T) {
	pkt := make([]byte, 10)
	_, err := parseDstIP6(pkt)
	if err != nil {
		t.Fatal("expected no error for short packet")
	}
}

// @sk-test ipv6-dual-stack#T4.1: TestRoutePacketIPv6 (AC-005)
func TestRoutePacketIPv6(t *testing.T) {
	cfg := &config.RoutingCfg{DefaultRoute: "server"}
	rs, err := NewRuleSet(cfg, nopLogger)
	if err != nil {
		t.Fatal(err)
	}

	tunRead := func([]byte) (int, error) { return 0, nil }
	tunWrite := func([]byte) (int, error) { return 0, nil }
	tunnelSend := func([]byte) error { return nil }

	router := NewTunRouter(rs, tunRead, tunWrite, tunnelSend, nopLogger)

	// IPv6 packet to 2001:db8::1
	pkt := make([]byte, 40)
	pkt[0] = 0x60
	pkt[24] = 0x20
	pkt[25] = 0x01
	pkt[26] = 0x0d
	pkt[27] = 0xb8
	pkt[39] = 0x01

	err = router.RoutePacket(pkt)
	if err != nil {
		t.Fatalf("RoutePacket: %v", err)
	}
}

// @sk-test dns-response-tracker#T2.1: TestRuleSetRoutesWithTrackerLookup (AC-003)
func TestRuleSetRoutesWithTrackerLookup(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:   "server",
		ExcludeDomains: []string{".ru"},
	}
	rs, err := NewRuleSet(cfg, nopLogger)
	if err != nil {
		t.Fatal(err)
	}
	tr := dns.NewTracker(60 * time.Second)
	tr.Track("ozon.ru", []netip.Addr{netip.MustParseAddr("95.163.249.123")})
	rs.SetTracker(tr)

	// Route direct for IP matching excluded domain
	action := rs.Route(netip.MustParseAddr("95.163.249.123"))
	if action != RouteDirect {
		t.Errorf("expected RouteDirect for excluded domain IP, got %d", action)
	}

	// Route server for unrelated IP
	action = rs.Route(netip.MustParseAddr("8.8.8.8"))
	if action != RouteServer {
		t.Errorf("expected RouteServer for unrelated IP, got %d", action)
	}
}

// @sk-test dns-response-tracker#T4.1: TestTunRouterRoutesWithTracker (AC-002)
func TestTunRouterRoutesWithTracker(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:   "server",
		ExcludeDomains: []string{".ru"},
	}
	rs, err := NewRuleSet(cfg, nopLogger)
	if err != nil {
		t.Fatal(err)
	}
	tr := dns.NewTracker(60 * time.Second)
	tr.Track("ozon.ru", []netip.Addr{netip.MustParseAddr("95.163.249.123")})
	rs.SetTracker(tr)

	var mu sync.Mutex
	var sentDirect, sentTunnel bool

	tunRead := func([]byte) (int, error) { return 0, nil }
	tunWrite := func(pkt []byte) (int, error) {
		mu.Lock()
		sentDirect = true
		mu.Unlock()
		return len(pkt), nil
	}
	tunnelSend := func(pkt []byte) error {
		mu.Lock()
		sentTunnel = true
		mu.Unlock()
		return nil
	}

	router := NewTunRouter(rs, tunRead, tunWrite, tunnelSend, nopLogger)

	// Packet to tracked IP → should go direct (excluded domain .ru)
	pkt := buildIPv4Packet("95.163.249.123")
	if err := router.RoutePacket(pkt); err != nil {
		t.Fatalf("RoutePacket tracked: %v", err)
	}
	mu.Lock()
	if !sentDirect {
		t.Error("expected packet to tracked IP to go direct")
	}
	if sentTunnel {
		t.Error("expected packet to tracked IP NOT to go through tunnel")
	}
	mu.Unlock()

	// Reset and test unrelated IP → should go tunnel (default=server)
	sentDirect = false
	sentTunnel = false
	pkt = buildIPv4Packet("8.8.8.8")
	if err := router.RoutePacket(pkt); err != nil {
		t.Fatalf("RoutePacket untracked: %v", err)
	}
	mu.Lock()
	if !sentTunnel {
		t.Error("expected packet to untracked IP to go through tunnel")
	}
	if sentDirect {
		t.Error("expected packet to untracked IP NOT to go direct")
	}
	mu.Unlock()
}

// @sk-test dns-response-tracker#T4.1: TestProxyOnConnTracker (AC-004)
func TestProxyOnConnTracker(t *testing.T) {
	cfg := &config.RoutingCfg{
		DefaultRoute:   "server",
		ExcludeDomains: []string{".ru"},
	}
	rs, err := NewRuleSet(cfg, nopLogger)
	if err != nil {
		t.Fatal(err)
	}
	tr := dns.NewTracker(60 * time.Second)
	tr.Track("ozon.ru", []netip.Addr{netip.MustParseAddr("95.163.249.123")})
	rs.SetTracker(tr)

	// Simulate proxy onConn flow: host not in suffix domains → DNS resolves → Track → Route(ip)
	// "example.com" doesn't match suffix ".ru", so MatchDomain returns RouteNone
	host := "example.com"
	if action := rs.MatchDomain(host); action != RouteNone {
		t.Fatalf("expected RouteNone for non-matching host, got %d", action)
	}

	// After DNS resolution, tracker has IP→domain mapping for "ozon.ru"
	// Route(ip) should find the domain via tracker and apply domain rule
	ip := netip.MustParseAddr("95.163.249.123")
	if action := rs.Route(ip); action != RouteDirect {
		t.Errorf("expected RouteDirect for tracked domain IP, got %d", action)
	}

	// Unrelated IP should fall through to default
	ip = netip.MustParseAddr("1.2.3.4")
	if action := rs.Route(ip); action != RouteServer {
		t.Errorf("expected RouteServer for unrelated IP, got %d", action)
	}
}

func buildIPv4Packet(dst string) []byte {
	ip := netip.MustParseAddr(dst)
	pkt := make([]byte, 20)
	pkt[0] = 0x45 // ver=4, ihl=5
	pkt[1] = 0x00
	binary.BigEndian.PutUint16(pkt[2:4], 20) // total length
	pkt[8] = 64                               // TTL
	pkt[9] = 6                                // TCP
	// dst IP at offset 16
	copy(pkt[16:20], ip.AsSlice())
	return pkt
}
