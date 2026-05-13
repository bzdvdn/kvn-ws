package routing

import (
	"net/netip"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

// @sk-test routing-split-tunnel#T4.1: BenchmarkRoute (AC-010)
func BenchmarkRoute(b *testing.B) {
	cfg := &config.RoutingCfg{
		DefaultRoute:  "direct",
		IncludeRanges: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
		IncludeIPs:    []string{"10.10.10.10", "10.10.10.11"},
		ExcludeRanges: []string{"10.0.0.0/16"},
	}
	rs, err := NewRuleSet(cfg)
	if err != nil {
		b.Fatal(err)
	}
	ips := []netip.Addr{
		netip.MustParseAddr("10.0.0.1"),    // excluded (Range match)
		netip.MustParseAddr("10.10.10.10"), // included (exact IP match)
		netip.MustParseAddr("172.16.0.1"),  // included (Range match)
		netip.MustParseAddr("8.8.8.8"),     // default (direct)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rs.Route(ips[i%len(ips)])
	}
}
