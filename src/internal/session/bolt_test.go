package session

import (
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

// @sk-test fix-critical-leaks#T6.1: TestBoltDBTimeout (AC-007)
func TestBoltDBTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	store, err := NewBoltStore(path, zap.NewNop())
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	defer store.Close()

	// Force another open while first is still open — should timeout fast, not hang
	start := time.Now()
	_, err = NewBoltStore(path, zap.NewNop())
	if err == nil {
		t.Log("second open succeeded (may be OS-dependent)")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("BoltDB open blocked for %v, expected ~1s timeout with bbolt.DefaultOptions.Timeout", d)
	}
}

// @sk-test fix-critical-leaks#T6.1: TestBoltDBTimeout (AC-007)
func TestBoltDBTimeout6(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test6.db")

	store, err := NewBoltStore6(path, zap.NewNop())
	if err != nil {
		t.Fatalf("NewBoltStore6: %v", err)
	}
	defer store.Close()

	start := time.Now()
	_, err = NewBoltStore6(path, zap.NewNop())
	if err == nil {
		t.Log("second open succeeded (may be OS-dependent)")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("BoltDB6 open blocked for %v, expected ~1s timeout", d)
	}
}
