// @sk-task security-acl#T6: Per-token bandwidth limiter
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
	m.mu.Lock()
	lim, ok := m.limiters[tokenName]
	if !ok {
		bps, exists := m.tokenCfg[tokenName]
		if !exists || bps <= 0 {
			m.mu.Unlock()
			return true
		}
		burst := bps
		if burst < 1 {
			burst = 1
		}
		lim = rate.NewLimiter(rate.Limit(bps), burst)
		m.limiters[tokenName] = lim
	}
	m.mu.Unlock()
	return lim.AllowN(time.Now(), bytes)
}
