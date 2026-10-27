package client

import (
	"context"
	"math/rand"
	"net"
	"time"
)

func nextBackoff(current, minBackoff, maxBackoff time.Duration) time.Duration {
	next := current * 2
	jitter := time.Duration(rand.Int63n(int64(time.Second))) - time.Second/2 // #nosec G404
	next += jitter
	if next < minBackoff {
		return minBackoff
	}
	if next > maxBackoff {
		return maxBackoff
	}
	return next
}

// @sk-task stealth-surface-hardening#T2.2: bounded jitter for reconnect cadence (AC-005)
func jitterDelay(base time.Duration, pct int) time.Duration {
	if base <= 0 {
		return base
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 50 {
		pct = 50
	}
	if pct == 0 {
		return base
	}
	delta := time.Duration(int64(base) * int64(pct) / 100)
	if delta <= 0 {
		return base
	}
	return base - delta + time.Duration(rand.Int63n(int64(2*delta)+1)) // #nosec G404 — timing jitter
}

// @sk-task fix-critical-leaks#T1.2: fix time.After leak (AC-008)
func sleepWithContext(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func computeGateway(ip net.IP, mask net.IPMask) net.IP {
	network := ip.Mask(mask)
	gw := make(net.IP, len(network))
	copy(gw, network)
	gw[len(gw)-1] = 1
	return gw
}
