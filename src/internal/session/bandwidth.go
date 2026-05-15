// @sk-task security-acl#T6: Per-token bandwidth limiter
// @sk-task post-hardening#T3.1: keep lock during AllowN (AC-008)
package session

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type TokenBandwidthManager struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	tokenCfg map[string]int
}

func NewTokenBandwidthManager(tokenCfgs map[string]int) *TokenBandwidthManager {
	return &TokenBandwidthManager{
		limiters: make(map[string]*rate.Limiter),
		tokenCfg: tokenCfgs,
	}
}

func (m *TokenBandwidthManager) Allow(tokenName string, bytes int) bool {
	_, ok := m.Reserve(tokenName, bytes)
	return ok
}

func (m *TokenBandwidthManager) Reserve(tokenName string, bytes int) (time.Duration, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lim, ok := m.limiters[tokenName]
	if !ok {
		bps, exists := m.tokenCfg[tokenName]
		if !exists || bps <= 0 {
			return 0, true
		}
		burst := bps
		if burst < 1 {
			burst = 1
		}
		lim = rate.NewLimiter(rate.Limit(bps), burst)
		m.limiters[tokenName] = lim
	}
	r := lim.ReserveN(time.Now(), bytes)
	if !r.OK() {
		return 0, false
	}
	return r.Delay(), true
}
