package client

import (
	"context"
	"sync"
	"testing"
	"time"
)

// @sk-test fix-critical-leaks#T6.1: TestSleepWithContextTimerLeak (AC-008)
func TestSleepWithContextTimerLeak(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		sleepWithContext(ctx, time.Hour)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sleepWithContext blocked after context cancellation")
	}
}

// @sk-test fix-critical-leaks#T6.1: TestSleepWithContextTimerLeak (AC-008)
func TestSleepWithContextTimerStops(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()
			sleepWithContext(ctx, time.Minute)
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timers not stopped properly")
	}
}

// @sk-test stealth-surface-hardening#T3.2: reconnect jitter stays within bounds (AC-005)
func TestJitterDelayBounds(t *testing.T) {
	base := 10 * time.Second
	pct := 20
	delta := base / 5
	for i := 0; i < 1000; i++ {
		got := jitterDelay(base, pct)
		if got < base-delta || got > base+delta {
			t.Fatalf("jitter = %v, want [%v, %v]", got, base-delta, base+delta)
		}
	}
	if got := jitterDelay(base, 0); got != base {
		t.Fatalf("zero jitter = %v, want %v", got, base)
	}
}
