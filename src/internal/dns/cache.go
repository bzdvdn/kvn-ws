package dns

import (
	"net/netip"
	"sync"
	"time"
)

// @sk-task routing-split-tunnel#T1.1: dns cache struct (AC-004)
// @sk-task prod-issue#T1.1: fix data race — RLock→Lock for expired entries (AC-001)
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	ips      []netip.Addr
	deadline time.Time
}

func NewCache() *Cache {
	return &Cache{entries: make(map[string]cacheEntry)}
}

// @sk-task prod-issue#T1.1: fix data race — release RLock before delete (AC-001)
func (c *Cache) Get(domain string) ([]netip.Addr, bool) {
	c.mu.RLock()
	e, ok := c.entries[domain]
	if !ok {
		c.mu.RUnlock()
		return nil, false
	}
	if time.Now().After(e.deadline) {
		c.mu.RUnlock()
		c.mu.Lock()
		delete(c.entries, domain)
		c.mu.Unlock()
		return nil, false
	}
	c.mu.RUnlock()
	return e.ips, true
}

func (c *Cache) Set(domain string, ips []netip.Addr, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[domain] = cacheEntry{
		ips:      ips,
		deadline: time.Now().Add(ttl),
	}
}
