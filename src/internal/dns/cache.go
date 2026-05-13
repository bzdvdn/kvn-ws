package dns

import (
	"net/netip"
	"sync"
	"time"
)

// @sk-task routing-split-tunnel#T1.1: dns cache struct (AC-004)
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

func (c *Cache) Get(domain string) ([]netip.Addr, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[domain]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.deadline) {
		delete(c.entries, domain)
		return nil, false
	}
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
