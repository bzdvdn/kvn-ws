package routing

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/dns"
)

// @sk-task routing-split-tunnel#T3.1: domain matcher (AC-005)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
// @sk-task prod-issue#T1.2: add local cache to avoid per-packet DNS lookups (AC-002)
// @sk-task dns-routing#T1.1: suffix domain support (AC-001, AC-002)
type DomainMatcher struct {
	domains       []string
	suffixes      []string
	resolver      dns.Resolver
	logger        *zap.Logger
	mu            sync.RWMutex
	resolved      map[string][]netip.Addr
	lastRefresh   time.Time
	refreshPeriod time.Duration
}

// @sk-task routing-split-tunnel#T3.1: new domain matcher (AC-005)
// @sk-task production-readiness-hardening#T1.1: add logger DI (AC-006)
// @sk-task prod-issue#T1.2: init local cache (AC-002)
// @sk-task dns-routing#T1.1: split exact/suffix domains (AC-001, AC-002)
func NewDomainMatcher(domains []string, resolver dns.Resolver, logger *zap.Logger) *DomainMatcher {
	var exacts, suffixes []string
	for _, d := range domains {
		if strings.HasPrefix(d, ".") {
			suffixes = append(suffixes, d)
		} else {
			exacts = append(exacts, d)
		}
	}
	return &DomainMatcher{
		domains:       exacts,
		suffixes:      suffixes,
		resolver:      resolver,
		logger:        logger,
		resolved:      make(map[string][]netip.Addr),
		refreshPeriod: 30 * time.Second,
	}
}

// @sk-task routing-split-tunnel#T3.1: domain match (AC-005)
// @sk-task production-readiness-hardening#T2.6: log.Printf → zap (AC-006)
// @sk-task production-readiness-hardening#T3.2: DNS context timeout (AC-008)
// @sk-task prod-issue#T1.2: use local cache, DNS lookup only on refresh (AC-002)
// @sk-task dns-routing#T1.1: MatchDomain suffix check (AC-001, AC-002)
func (m *DomainMatcher) MatchDomain(domain string) bool {
	for _, suffix := range m.suffixes {
		if strings.HasSuffix(domain, suffix) {
			return true
		}
	}
	return false
}

// @sk-task routing-split-tunnel#T3.1: domain match (AC-005)
// @sk-task production-readiness-hardening#T2.6: log.Printf → zap (AC-006)
// @sk-task production-readiness-hardening#T3.2: DNS context timeout (AC-008)
// @sk-task prod-issue#T1.2: use local cache, DNS lookup only on refresh (AC-002)
func (m *DomainMatcher) Match(ip netip.Addr) bool {
	m.refreshCache()
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, d := range m.domains {
		ips, ok := m.resolved[d]
		if !ok {
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

// @sk-task prod-issue#T1.2: refresh domain resolution cache periodically (AC-002)
func (m *DomainMatcher) refreshCache() {
	m.mu.RLock()
	needRefresh := time.Since(m.lastRefresh) >= m.refreshPeriod
	m.mu.RUnlock()
	if !needRefresh {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Since(m.lastRefresh) < m.refreshPeriod {
		return
	}
	for _, d := range m.domains {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ips, err := m.resolver.Lookup(ctx, d)
		cancel()
		if err != nil {
			m.logger.Warn("domain resolve failed", zap.String("domain", d), zap.Error(err))
			continue
		}
		m.resolved[d] = ips
	}
	m.lastRefresh = time.Now()
}
