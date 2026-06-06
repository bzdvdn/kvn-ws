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
