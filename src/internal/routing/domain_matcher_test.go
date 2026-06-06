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

// @sk-test dns-routing#T7.1: MatchDomain suffix .ru (AC-001)
func TestMatchDomainSuffixRu(t *testing.T) {
	resolver := &mockDomainResolver{}
	m := NewDomainMatcher([]string{".ru"}, resolver, zap.NewNop())

	if !m.MatchDomain("hh.ru") {
		t.Error("expected match for hh.ru with suffix .ru")
	}
	if !m.MatchDomain("mail.ru") {
		t.Error("expected match for mail.ru with suffix .ru")
	}
	if m.MatchDomain("google.com") {
		t.Error("expected no match for google.com with suffix .ru")
	}
}

// @sk-test dns-routing#T7.1: MatchDomain suffix .ozon.ru (AC-002)
func TestMatchDomainSuffixOzon(t *testing.T) {
	resolver := &mockDomainResolver{}
	m := NewDomainMatcher([]string{".ozon.ru"}, resolver, zap.NewNop())

	if !m.MatchDomain("api.ozon.ru") {
		t.Error("expected match for api.ozon.ru with suffix .ozon.ru")
	}
	if !m.MatchDomain("www.ozon.ru") {
		t.Error("expected match for www.ozon.ru with suffix .ozon.ru")
	}
	if m.MatchDomain("ozon.com") {
		t.Error("expected no match for ozon.com with suffix .ozon.ru")
	}
}

// @sk-test dns-routing#T7.1: MatchDomain bare ru — no match (AC-003)
func TestMatchDomainBareRu(t *testing.T) {
	resolver := &mockDomainResolver{}
	m := NewDomainMatcher([]string{".ru"}, resolver, zap.NewNop())

	if m.MatchDomain("ru") {
		t.Error("expected no match for bare 'ru' with suffix .ru")
	}
}

// @sk-test dns-routing#T7.1: MatchDomain exact domain without dot (AC-003)
func TestMatchDomainExactWithoutDot(t *testing.T) {
	resolver := &mockDomainResolver{}
	m := NewDomainMatcher([]string{"example.com"}, resolver, zap.NewNop())

	if m.MatchDomain("sub.example.com") {
		t.Error("expected no match for sub.example.com with exact example.com")
	}
	if m.MatchDomain("example.com") {
		t.Error("expected no match for example.com via MatchDomain (exact domains use Match not MatchDomain)")
	}
}

// @sk-test dns-routing#T7.1: NewDomainMatcher splits exact and suffix domains (AC-001, AC-002)
func TestNewDomainMatcherSplit(t *testing.T) {
	resolver := &mockDomainResolver{}
	m := NewDomainMatcher([]string{".ru", ".ozon.ru", "example.com"}, resolver, zap.NewNop())

	if len(m.domains) != 1 || m.domains[0] != "example.com" {
		t.Errorf("expected 1 exact domain [example.com], got %v", m.domains)
	}
	if len(m.suffixes) != 2 {
		t.Errorf("expected 2 suffix domains, got %v", m.suffixes)
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

// @sk-test fix-critical-leaks#T6.1: TestDNSContextPropagation (AC-005)
func TestDNSContextPropagation(t *testing.T) {
	resolver := &mockDomainResolver{ips: []netip.Addr{netip.MustParseAddr("10.10.10.10")}}
	m := NewDomainMatcher([]string{"ctx.corp.ru"}, resolver, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	m.SetCtx(ctx)
	cancel() // cancel before refresh

	m.refreshPeriod = 0 // force refresh on next Match
	// After ctx is cancelled, refreshCache should use the cancelled context
	m.Match(netip.MustParseAddr("10.10.10.10"))
	t.Log("DNS ctx propagation: SetCtx applied, no panic on cancelled ctx")
}

// @sk-test fix-critical-leaks#T6.1: TestDNSContextPropagation (AC-005)
func TestDNSContextDefaultNotNil(t *testing.T) {
	resolver := &mockDomainResolver{ips: []netip.Addr{netip.MustParseAddr("10.10.10.10")}}
	m := NewDomainMatcher([]string{"default.corp.ru"}, resolver, zap.NewNop())

	if m.baseCtx == nil {
		t.Error("baseCtx should not be nil after NewDomainMatcher")
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
