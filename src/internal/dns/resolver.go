package dns

import (
	"context"
	"net"
	"net/netip"
	"time"
)

// @sk-task routing-split-tunnel#T1.1: dns resolver interface (AC-004)
type Resolver interface {
	Lookup(ctx context.Context, domain string) ([]netip.Addr, error)
}

const DefaultTTL = 60 * time.Second

// @sk-task routing-split-tunnel#T1.1: default resolver (AC-004)
type DefaultResolver struct {
	cache *Cache
	ttl   time.Duration
}

func NewDefaultResolver(cache *Cache) *DefaultResolver {
	return &DefaultResolver{cache: cache, ttl: DefaultTTL}
}

func NewDefaultResolverWithTTL(cache *Cache, ttl time.Duration) *DefaultResolver {
	return &DefaultResolver{cache: cache, ttl: ttl}
}

// @sk-task routing-split-tunnel#T3.1: dns lookup with ttl cache (AC-004)
// @sk-task ipv6-dual-stack#T3.2: dual-stack DNS with A+AAAA queries (AC-005)
func (r *DefaultResolver) Lookup(ctx context.Context, domain string) ([]netip.Addr, error) {
	if r.cache != nil {
		if ips, ok := r.cache.Get(domain); ok {
			return ips, nil
		}
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", domain)
	if err != nil {
		return nil, err
	}
	if r.cache != nil && r.ttl > 0 {
		r.cache.Set(domain, ips, r.ttl)
	}
	return ips, nil
}
