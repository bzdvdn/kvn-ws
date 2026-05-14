package routing

import (
	"context"
	"net/netip"
	"testing"

	"go.uber.org/zap"
)

type mockDomainResolver struct {
	ips []netip.Addr
}

func (m *mockDomainResolver) Lookup(ctx context.Context, domain string) ([]netip.Addr, error) {
	return m.ips, nil
}

// @sk-test routing-split-tunnel#T3.2: TestDomainMatcherMatch (AC-005)
func TestDomainMatcherMatch(t *testing.T) {
	resolver := &mockDomainResolver{ips: []netip.Addr{netip.MustParseAddr("10.10.10.10")}}
	m := NewDomainMatcher([]string{"internal.corp.ru"}, resolver, zap.NewNop())

	if !m.Match(netip.MustParseAddr("10.10.10.10")) {
		t.Error("expected match for resolved IP")
	}
	if m.Match(netip.MustParseAddr("8.8.8.8")) {
		t.Error("expected no match for non-resolved IP")
	}
}

// @sk-test routing-split-tunnel#T3.2: TestDomainMatcherMultipleIPs (AC-005)
func TestDomainMatcherMultipleIPs(t *testing.T) {
	resolver := &mockDomainResolver{
		ips: []netip.Addr{
			netip.MustParseAddr("10.10.10.10"),
			netip.MustParseAddr("10.10.10.11"),
		},
	}
	m := NewDomainMatcher([]string{"multi.corp.ru"}, resolver, zap.NewNop())

	if !m.Match(netip.MustParseAddr("10.10.10.10")) {
		t.Error("expected match for first resolved IP")
	}
	if !m.Match(netip.MustParseAddr("10.10.10.11")) {
		t.Error("expected match for second resolved IP")
	}
}
