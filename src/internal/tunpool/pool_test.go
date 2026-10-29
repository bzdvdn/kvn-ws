package tunpool

import (
	"errors"
	"testing"
	"time"
)

func testPool(t *testing.T) (*Pool, *time.Time) {
	t.Helper()
	p, err := New(&Config{
		Subnet: "10.66.0.0/24", RangeStart: "10.66.0.2", RangeEnd: "10.66.0.3",
		Gateway: "10.66.0.1", LeaseTTLSec: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	p.now = func() time.Time { return now }
	return p, &now
}

func TestUniqueAndRenew(t *testing.T) {
	p, _ := testPool(t)
	a, err := p.Lease("a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Lease("b")
	if err != nil {
		t.Fatal(err)
	}
	if a.Address == b.Address {
		t.Fatalf("same address %s", a.Address)
	}
	again, err := p.Lease("a")
	if err != nil || again.Address != a.Address {
		t.Fatalf("renew = %+v, %v", again, err)
	}
}

func TestExhaustedAndExpiry(t *testing.T) {
	p, now := testPool(t)
	if _, err := p.Lease("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Lease("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Lease("c"); !errors.Is(err, ErrExhausted) {
		t.Fatalf("want exhausted, got %v", err)
	}
	*now = now.Add(61 * time.Second)
	got, err := p.Lease("c")
	if err != nil {
		t.Fatal(err)
	}
	if got.Address == "" {
		t.Fatal("no address after expiry")
	}
}

func TestPersistenceAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/leases.json"
	cfg := Config{Subnet: "10.66.0.0/24", RangeStart: "10.66.0.2", RangeEnd: "10.66.0.3", Gateway: "10.66.0.1", LeaseTTLSec: 60, LeaseFile: path}
	p1, err := New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	a, err := p1.Lease("a")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := p2.Lease("a")
	if err != nil {
		t.Fatal(err)
	}
	if a.Address != b.Address {
		t.Fatalf("lease changed after restart: %s -> %s", a.Address, b.Address)
	}
	c, err := p2.Lease("b")
	if err != nil {
		t.Fatal(err)
	}
	if c.Address == a.Address {
		t.Fatal("restarted pool reused active address")
	}
}
