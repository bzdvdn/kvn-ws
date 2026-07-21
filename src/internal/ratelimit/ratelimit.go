package ratelimit

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// @sk-task production-hardening#T2.1: IP-based rate limiter (AC-003)
// @sk-task production-hardening#T2.2: per-session packet rate limiter (AC-004)
// @sk-task performance-scope-p2#T2.5: sync.Map вместо sync.Mutex (AC-006)
// IPRateLimiter limits requests per IP address.
type IPRateLimiter struct {
	limiters sync.Map
	burst    int
	perMin   int
}

func NewIPRateLimiter(burst, perMin int) *IPRateLimiter {
	return &IPRateLimiter{
		burst:  burst,
		perMin: perMin,
	}
}

func (rl *IPRateLimiter) Allow(addr string) bool {
	limIface, _ := rl.limiters.Load(addr)
	lim, ok := limIface.(*rate.Limiter)
	if !ok {
		lim = rate.NewLimiter(rate.Limit(rl.perMin)/60, rl.burst)
		actual, loaded := rl.limiters.LoadOrStore(addr, lim)
		if loaded {
			lim = actual.(*rate.Limiter)
		}
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
				rl.limiters.Range(func(k, v any) bool {
					if v.(*rate.Limiter).Tokens() >= float64(rl.burst) {
						rl.limiters.Delete(k)
					}
					return true
				})
			}
		}
	}()
}

// @sk-task performance-scope-p2#T2.5: sync.Map вместо sync.Mutex (AC-006)
// SessionPacketLimiter limits packet rate per session.
type SessionPacketLimiter struct {
	limiters sync.Map
	perSec   int
}

func NewSessionPacketLimiter(perSec int) *SessionPacketLimiter {
	return &SessionPacketLimiter{
		perSec: perSec,
	}
}

func (pl *SessionPacketLimiter) Allow(sessionID string) bool {
	limIface, _ := pl.limiters.Load(sessionID)
	lim, ok := limIface.(*rate.Limiter)
	if !ok {
		lim = rate.NewLimiter(rate.Limit(pl.perSec), pl.perSec)
		actual, loaded := pl.limiters.LoadOrStore(sessionID, lim)
		if loaded {
			lim = actual.(*rate.Limiter)
		}
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
				pl.limiters.Range(func(k, v any) bool {
					if v.(*rate.Limiter).Tokens() >= float64(pl.perSec) {
						pl.limiters.Delete(k)
					}
					return true
				})
			}
		}
	}()
}
