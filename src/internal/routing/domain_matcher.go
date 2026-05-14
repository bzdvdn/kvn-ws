package routing

import (
	"context"
	"net/netip"
	"time"

	"go.uber.org/zap"
)

// @sk-task routing-split-tunnel#T3.1: dns resolver interface for domain matcher (AC-005)
type DomainResolver interface {
	Lookup(ctx context.Context, domain string) ([]netip.Addr, error)
}

// @sk-task routing-split-tunnel#T3.1: domain matcher (AC-005)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
type DomainMatcher struct {
	domains  []string
	resolver DomainResolver
	logger   *zap.Logger
}

// @sk-task routing-split-tunnel#T3.1: new domain matcher (AC-005)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
func NewDomainMatcher(domains []string, resolver DomainResolver, logger *zap.Logger) *DomainMatcher {
	return &DomainMatcher{domains: domains, resolver: resolver, logger: logger}
}

// @sk-task routing-split-tunnel#T3.1: domain match (AC-005)
// @sk-task production-readiness-hardening#T2.6: log.Printf → zap (AC-006)
// @sk-task production-readiness-hardening#T3.2: DNS context timeout (AC-008)
func (m *DomainMatcher) Match(ip netip.Addr) bool {
	for _, d := range m.domains {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ips, err := m.resolver.Lookup(ctx, d)
		cancel()
		if err != nil {
			m.logger.Warn("domain resolve failed", zap.String("domain", d), zap.Error(err))
			continue
		}
		for _, resolved := range ips {
			if ip == resolved {
				return true
			}
		}
	}
	return false
}
