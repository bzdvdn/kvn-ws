package session

import (
	"net"
	"testing"
)

func testPool(t *testing.T) *IPPool {
	t.Helper()
	pool, err := NewIPPool(PoolCfg{
		Subnet:     "10.10.0.0/24",
		Gateway:    "10.10.0.1",
		RangeStart: "10.10.0.10",
		RangeEnd:   "10.10.0.20",
	})
	if err != nil {
		t.Fatalf("NewIPPool: %v", err)
	}
	return pool
}

// @sk-test core-tunnel-mvp#T5.1: TestIPPoolAllocateRelease (AC-009)
func TestIPPoolAllocateDifferentSessions(t *testing.T) {
	pool := testPool(t)
	defer pool.Release("sess1")
	defer pool.Release("sess2")

	ip1, err := pool.Allocate("sess1")
	if err != nil {
		t.Fatalf("Allocate(sess1): %v", err)
	}
	ip2, err := pool.Allocate("sess2")
	if err != nil {
		t.Fatalf("Allocate(sess2): %v", err)
	}

	if ip1.Equal(ip2) {
		t.Errorf("expected different IPs, got same %s", ip1)
	}
}

func TestIPPoolSameSessionReturnsSameIP(t *testing.T) {
	pool := testPool(t)
	defer pool.Release("sess1")

	ip1, err := pool.Allocate("sess1")
	if err != nil {
		t.Fatalf("Allocate(sess1): %v", err)
	}
	ip2, err := pool.Allocate("sess1")
	if err != nil {
		t.Fatalf("Allocate(sess1) second: %v", err)
	}

	if !ip1.Equal(ip2) {
		t.Errorf("expected same IP, got %s vs %s", ip1, ip2)
	}
}

func TestIPPoolReleaseAndReallocate(t *testing.T) {
	pool := testPool(t)

	ip1, err := pool.Allocate("sess1")
	if err != nil {
		t.Fatalf("Allocate(sess1): %v", err)
	}
	pool.Release("sess1")

	ip2, err := pool.Allocate("sess1")
	if err != nil {
		t.Fatalf("Allocate(sess1) after release: %v", err)
	}

	if !ip1.Equal(ip2) {
		t.Errorf("expected same IP after release, got %s vs %s", ip1, ip2)
	}
}

func TestIPPoolResolve(t *testing.T) {
	pool := testPool(t)
	defer pool.Release("sess1")

	ip1, err := pool.Allocate("sess1")
	if err != nil {
		t.Fatalf("Allocate(sess1): %v", err)
	}

	ip2, ok := pool.Resolve("sess1")
	if !ok {
		t.Fatal("Resolve(sess1) returned false")
	}
	if !ip1.Equal(ip2) {
		t.Errorf("Resolve = %s, want %s", ip2, ip1)
	}

	_, ok = pool.Resolve("nonexistent")
	if ok {
		t.Error("Resolve(nonexistent) returned true")
	}
}

func TestIPPoolSkipsGateway(t *testing.T) {
	pool, err := NewIPPool(PoolCfg{
		Subnet:     "10.10.0.0/24",
		Gateway:    "10.10.0.1",
		RangeStart: "10.10.0.1",
		RangeEnd:   "10.10.0.5",
	})
	if err != nil {
		t.Fatalf("NewIPPool: %v", err)
	}

	firstIP, err := pool.Allocate("sess1")
	if err != nil {
		t.Fatalf("Allocate(sess1): %v", err)
	}
	if firstIP.Equal(net.ParseIP("10.10.0.1")) {
		t.Errorf("allocated gateway IP %s", firstIP)
	}
}

func TestIPPoolExhaustion(t *testing.T) {
	pool, err := NewIPPool(PoolCfg{
		Subnet:     "10.10.0.0/29",
		Gateway:    "10.10.0.1",
		RangeStart: "10.10.0.1",
		RangeEnd:   "10.10.0.3",
	})
	if err != nil {
		t.Fatalf("NewIPPool: %v", err)
	}

	_, err = pool.Allocate("sess1")
	if err != nil {
		t.Fatalf("Allocate(sess1): %v", err)
	}
	_, err = pool.Allocate("sess2")
	if err != nil {
		t.Fatalf("Allocate(sess2): %v", err)
	}
	// /29 with 10.10.0.1-10.10.0.3: 3 IPs minus gateway (10.10.0.1) = 2 usable
	_, err = pool.Allocate("sess3")
	if err == nil {
		t.Error("expected pool exhaustion error")
	}
}

func TestSessionManagerCreateGetRemove(t *testing.T) {
	pool := testPool(t)
	sm := NewSessionManager(pool)

	sess, ip, err := sm.Create("session-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if sess.ID != "session-1" {
		t.Errorf("session ID = %s, want session-1", sess.ID)
	}
	if ip == nil {
		t.Fatal("assigned IP is nil")
	}

	got, ok := sm.Get("session-1")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.ID != "session-1" {
		t.Errorf("Get ID = %s", got.ID)
	}

	sm.Remove("session-1")
	_, ok = sm.Get("session-1")
	if ok {
		t.Error("Get after Remove returned true")
	}
}
