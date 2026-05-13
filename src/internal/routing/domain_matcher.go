package routing

import (
	"context"
	"log"
	"net/netip"
)

// @sk-task routing-split-tunnel#T3.1: dns resolver interface for domain matcher (AC-005)
type DomainResolver interface {
	Lookup(ctx context.Context, domain string) ([]netip.Addr, error)
}

// @sk-task routing-split-tunnel#T3.1: domain matcher (AC-005)
type DomainMatcher struct {
	domains  []string
	resolver DomainResolver
}

// @sk-task routing-split-tunnel#T3.1: new domain matcher (AC-005)
func NewDomainMatcher(domains []string, resolver DomainResolver) *DomainMatcher {
	return &DomainMatcher{domains: domains, resolver: resolver}
}

// @sk-task routing-split-tunnel#T3.1: domain match (AC-005)
func (m *DomainMatcher) Match(ip netip.Addr) bool {
	for _, d := range m.domains {
		ips, err := m.resolver.Lookup(context.Background(), d)
		if err != nil {
			log.Printf("[routing] domain resolve %q: %v", d, err)
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
