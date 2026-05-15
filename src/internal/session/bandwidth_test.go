// @sk-test security-acl#T6: bandwidth limiter tests (AC-003)
package session

import (
	"testing"
)

func TestBandwidthLimiterUnlimited(t *testing.T) {
	mgr := NewTokenBandwidthManager(map[string]int{"user1": 0})
	if !mgr.Allow("user1", 1000000) {
		t.Error("expected unlimited to allow any size")
	}
}

func TestBandwidthLimiterAllowsSmall(t *testing.T) {
	cfg := map[string]int{"test": 102400}
	mgr := NewTokenBandwidthManager(cfg)
	if !mgr.Allow("test", 100) {
		t.Error("expected 100 bytes to be allowed")
	}
}

func TestBandwidthLimiterUnknownToken(t *testing.T) {
	mgr := NewTokenBandwidthManager(map[string]int{})
	if !mgr.Allow("unknown", 1000) {
		t.Error("expected unknown token to be allowed (no bw cfg)")
	}
}

// @sk-test post-hardening#T4.3: TestBandwidthManagerRace (AC-008)
func TestBandwidthManagerRace(t *testing.T) {
	mgr := NewTokenBandwidthManager(map[string]int{"limited": 100000})
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				mgr.Allow("limited", 1000)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
