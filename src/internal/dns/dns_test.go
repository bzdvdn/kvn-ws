package dns

import (
	"context"
	"net/netip"
	"testing"
	"time"
)

// @sk-test routing-split-tunnel#T3.2: TestCacheGetSet (AC-004)
func TestCacheGetSet(t *testing.T) {
	c := NewCache()
	ips := []netip.Addr{netip.MustParseAddr("10.10.10.10")}
	c.Set("example.com", ips, 60*time.Second)

	got, ok := c.Get("example.com")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(got) != 1 || got[0] != ips[0] {
		t.Errorf("expected %v, got %v", ips, got)
	}
}

// @sk-test routing-split-tunnel#T3.2: TestCacheExpiry (AC-004)
func TestCacheExpiry(t *testing.T) {
	c := NewCache()
	ips := []netip.Addr{netip.MustParseAddr("10.10.10.10")}
	c.Set("example.com", ips, 1*time.Millisecond)

	time.Sleep(5 * time.Millisecond)

	_, ok := c.Get("example.com")
	if ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

// @sk-test routing-split-tunnel#T3.2: TestCacheTTLZeroExpired (AC-004)
func TestCacheTTLZeroExpired(t *testing.T) {
	c := NewCache()
	ips := []netip.Addr{netip.MustParseAddr("10.10.10.10")}
	c.Set("example.com", ips, -1*time.Second)

	time.Sleep(1 * time.Millisecond)

	_, ok := c.Get("example.com")
	if ok {
		t.Fatal("expected cache miss for expired entry")
	}
}

// @sk-test routing-split-tunnel#T3.2: TestCacheMiss (AC-004)
func TestCacheMiss(t *testing.T) {
	c := NewCache()
	_, ok := c.Get("nonexistent.com")
	if ok {
		t.Fatal("expected cache miss for unknown domain")
	}
}

// @sk-test routing-split-tunnel#T3.2: TestResolverDefaultTTL (AC-004)
func TestResolverDefaultTTL(t *testing.T) {
	c := NewCache()
	r := NewDefaultResolver(c)
	if r.ttl != DefaultTTL {
		t.Errorf("expected default TTL %v, got %v", DefaultTTL, r.ttl)
	}
}

// @sk-test prod-issue#T1.1: concurrent Get/Set race test (AC-001)
func TestCacheConcurrentRace(t *testing.T) {
	c := NewCache()
	ips := []netip.Addr{netip.MustParseAddr("10.10.10.10")}
	c.Set("example.com", ips, 50*time.Millisecond)

	done := make(chan struct{}, 2)
	go func() {
		for i := 0; i < 100; i++ {
			c.Get("example.com")
			c.Set("example.com", ips, 50*time.Millisecond)
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 100; i++ {
			c.Get("example.com")
			c.Set("other.com", ips, 50*time.Millisecond)
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}

// @sk-test production-readiness-hardening#T4.2: TestDNSResolveTimeout (AC-008)
func TestDNSResolveTimeout(t *testing.T) {
	r := NewDefaultResolver(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	_, err := r.Lookup(ctx, "example.com")
	if err == nil {
		t.Error("expected timeout error for expired context")
	}
}
