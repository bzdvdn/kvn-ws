package routing

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"go.uber.org/zap"
)

type mockDomainResolver struct {
	ips     []netip.Addr
	lookups int
}

func (m *mockDomainResolver) Lookup(ctx context.Context, domain string) ([]netip.Addr, error) {
	m.lookups++
	return m.ips, nil
}

func (m *mockDomainResolver) Lookups() int {
	return m.lookups
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

// @sk-test prod-issue#T1.2: domain matcher cache — resolver called once per domain (AC-002)
func TestDomainMatcherCacheHit(t *testing.T) {
	resolver := &mockDomainResolver{ips: []netip.Addr{netip.MustParseAddr("10.10.10.10")}}
	m := NewDomainMatcher([]string{"cache.corp.ru"}, resolver, zap.NewNop())
	m.refreshPeriod = 10 * time.Minute
	m.refreshCache()

	m.Match(netip.MustParseAddr("10.10.10.10"))
	m.Match(netip.MustParseAddr("10.10.10.10"))
	m.Match(netip.MustParseAddr("10.10.10.10"))

	if n := resolver.Lookups(); n > 1 {
		t.Errorf("resolver called %d times, expected 1 (cache should serve subsequent matches)", n)
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
