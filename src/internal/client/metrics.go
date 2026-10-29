package client

import (
	"context"
	"net"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

// Metrics is a snapshot of the v2 data plane.
type Metrics struct {
	RxBytes    int64
	TxBytes    int64
	LatencyMS  float64
	UptimeS    int64
	Reconnects int64
}

type countingTun struct {
	tun.TunDevice
	rx *atomic.Int64
	tx *atomic.Int64
}

// Read reads packets written by the kernel into TUN: these are outbound
// (upload) packets, so they belong to TxBytes.
func (d *countingTun) Read(p []byte) (int, error) {
	n, err := d.TunDevice.Read(p)
	if n > 0 {
		d.tx.Add(int64(n))
	}
	return n, err
}

// Write injects packets from the tunnel into the kernel: these are inbound
// (download) packets, so they belong to RxBytes.
func (d *countingTun) Write(p []byte) (int, error) {
	n, err := d.TunDevice.Write(p)
	if n > 0 {
		d.rx.Add(int64(n))
	}
	return n, err
}

func (c *Client) Metrics() Metrics {
	uptime := int64(0)
	if !c.startedAt.IsZero() {
		uptime = int64(time.Since(c.startedAt).Seconds())
	}
	return Metrics{
		RxBytes:    c.rxBytes.Load(),
		TxBytes:    c.txBytes.Load(),
		LatencyMS:  float64(c.latencyMS.Load()),
		UptimeS:    uptime,
		Reconnects: c.reconnects.Load(),
	}
}

func (c *Client) counted(dev tun.TunDevice) tun.TunDevice {
	return &countingTun{TunDevice: dev, rx: &c.rxBytes, tx: &c.txBytes}
}

func (c *Client) latencyLoop(ctx context.Context, endpoint string) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "443")
	}
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		start := time.Now()
		conn, derr := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", host)
		if derr == nil {
			_ = conn.Close()
			c.latencyMS.Store(uint64(time.Since(start).Milliseconds())) // #nosec G115 -- elapsed milliseconds are non-negative
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
