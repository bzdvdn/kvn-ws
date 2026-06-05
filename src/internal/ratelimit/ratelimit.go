// @sk-task production-hardening#T2.1: IP-based rate limiter (AC-003)
// @sk-task production-hardening#T2.2: per-session packet rate limiter (AC-004)
package ratelimit

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IPRateLimiter limits requests per IP address.
type IPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	burst    int
	perMin   int
}

func NewIPRateLimiter(burst, perMin int) *IPRateLimiter {
	return &IPRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		burst:    burst,
		perMin:   perMin,
	}
}

func (rl *IPRateLimiter) Allow(addr string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	lim, ok := rl.limiters[addr]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(rl.perMin)/60, rl.burst)
		rl.limiters[addr] = lim
	}
	return lim.Allow()
}

func (rl *IPRateLimiter) StartCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rl.mu.Lock()
				for k, lim := range rl.limiters {
					if lim.Tokens() >= float64(rl.burst) {
						delete(rl.limiters, k)
					}
				}
				rl.mu.Unlock()
			}
		}
	}()
}

// SessionPacketLimiter limits packet rate per session.
type SessionPacketLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	perSec   int
}

func NewSessionPacketLimiter(perSec int) *SessionPacketLimiter {
	return &SessionPacketLimiter{
		limiters: make(map[string]*rate.Limiter),
		perSec:   perSec,
	}
}

func (pl *SessionPacketLimiter) Allow(sessionID string) bool {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	lim, ok := pl.limiters[sessionID]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(pl.perSec), pl.perSec)
		pl.limiters[sessionID] = lim
	}
	return lim.Allow()
}

func (pl *SessionPacketLimiter) StartCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pl.mu.Lock()
				for k, lim := range pl.limiters {
					if lim.Tokens() >= float64(pl.perSec) {
						delete(pl.limiters, k)
					}
				}
				pl.mu.Unlock()
			}
		}
	}()
}
