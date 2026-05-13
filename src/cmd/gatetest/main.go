// @sk-task routing-split-tunnel#T4.1: gate test program (AC-010)
package main

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
)

type mockResolver struct{}

func (m *mockResolver) Lookup(ctx context.Context, domain string) ([]netip.Addr, error) {
	if domain == "corp.example.com" {
		return []netip.Addr{netip.MustParseAddr("10.10.10.10")}, nil
	}
	return nil, nil
}

func main() {
	fmt.Println("=== Routing Gate Test ===")

	cfg := &config.RoutingCfg{
		DefaultRoute:   "direct",
		IncludeRanges:  []string{"10.0.0.0/8", "172.16.0.0/12"},
		IncludeDomains: []string{"corp.example.com"},
	}

	rs, err := routing.NewRuleSetWithResolver(cfg, &mockResolver{})
	if err != nil {
		fmt.Printf("FAIL: NewRuleSet: %v\n", err)
		return
	}

	tests := []struct {
		ip       string
		expected routing.RouteAction
		desc     string
	}{
		{"10.10.10.10", routing.RouteServer, "corp IP via include_range"},
		{"172.16.0.50", routing.RouteServer, "corp IP via include_range 2"},
		{"8.8.8.8", routing.RouteDirect, "public IP — default direct"},
		{"1.1.1.1", routing.RouteDirect, "public DNS — default direct"},
		{"10.0.0.1", routing.RouteServer, "edge of include_range"},
	}

	allPass := true
	for _, tt := range tests {
		ip := netip.MustParseAddr(tt.ip)
		action := rs.Route(ip)
		status := "PASS"
		if action != tt.expected {
			status = "FAIL"
			allPass = false
		}
		fmt.Printf("  [%s] %-15s -> %d (expected %d) — %s\n", status, tt.ip, action, tt.expected, tt.desc)
	}

	if !allPass {
		fmt.Println("\nFAIL: routing decisions did not match expectations")
	} else {
		fmt.Println("\nPASS: all routing decisions match expectations")
	}
}
