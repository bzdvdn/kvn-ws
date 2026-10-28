// Package mobile exposes the v2 data plane to Android via gomobile.
package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/client"
	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

// Resolve fetches the server-assigned TUN JSON for a config without opening TUN.
func Resolve(configJSON string) (string, error) {
	var cfg config.V2ClientConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return "", fmt.Errorf("parse v2 config: %w", err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	session, err := client.New(&cfg, zap.NewNop()).ResolveTUN(context.Background())
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Client is one running mobile v2 session.
type Client struct {
	cancel context.CancelFunc
	dev    *fdDevice
	done   chan struct{}
	mu     sync.Mutex
	err    error
}

// Start parses JSON V2 config and pumps the Android VpnService TUN fd.
func Start(configJSON string, fd int) (*Client, error) {
	if fd < 0 {
		return nil, fmt.Errorf("invalid tun fd %d", fd)
	}
	var cfg config.V2ClientConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, fmt.Errorf("parse v2 config: %w", err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	dev := newFDDevice(fd)
	c := &Client{dev: dev, done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go func() {
		defer close(c.done)
		err := client.New(&cfg, zap.NewNop()).RunTUNDevice(ctx, dev)
		_ = dev.Close()
		c.mu.Lock()
		c.err = err
		c.mu.Unlock()
	}()
	// Give immediate session/config failures a chance to surface.
	select {
	case <-c.done:
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		if err == nil {
			err = fmt.Errorf("mobile session stopped")
		}
		return nil, err
	case <-time.After(250 * time.Millisecond):
	}
	return c, nil
}

// RxBytes returns the number of bytes read from TUN.
func (c *Client) RxBytes() int64 { return c.dev.rx.Load() }

// TxBytes returns the number of bytes written to TUN.
func (c *Client) TxBytes() int64 { return c.dev.tx.Load() }

// Stop cancels the session and closes the TUN descriptor.
func (c *Client) Stop() {
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	// Closing the TUN fd unblocks RunTUNDevice's blocking Read. Without this
	// the service would keep the Android VPN interface alive forever.
	_ = c.dev.Close()
	select {
	case <-c.done:
	case <-time.After(3 * time.Second):
	}
}

// Error returns the terminal session error, if any.
func (c *Client) Error() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		return ""
	}
	return c.err.Error()
}

type fdDevice struct {
	fd int
	rx atomic.Int64
	tx atomic.Int64
}

func newFDDevice(fd int) *fdDevice {
	return &fdDevice{fd: fd}
}

func (d *fdDevice) Open() error { return nil }

// Close is intentionally a no-op: Kotlin VpnService owns the fd.
func (d *fdDevice) Close() error { return nil }
func (d *fdDevice) Read(p []byte) (int, error) {
	for {
		n, err := syscall.Read(d.fd, p)
		if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if n > 0 {
			d.rx.Add(int64(n))
		}
		return n, err
	}
}

func (d *fdDevice) Write(p []byte) (int, error) {
	for {
		n, err := syscall.Write(d.fd, p)
		if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if n > 0 {
			d.tx.Add(int64(n))
		}
		return n, err
	}
}
func (d *fdDevice) SetIP(net.IP, *net.IPNet) error                  { return nil }
func (d *fdDevice) SetMTU(int) error                                { return nil }
func (d *fdDevice) SetGateway(net.IP) error                         { return nil }
func (d *fdDevice) RemoveGateway(net.IP) error                      { return nil }
func (d *fdDevice) AddExcludeRoute(string, net.IP, string) error    { return nil }
func (d *fdDevice) RemoveExcludeRoute(string, net.IP, string) error { return nil }
func (d *fdDevice) SetDNS([]string) error                           { return nil }
func (d *fdDevice) CleanupExcludeRoutes()                           {}
func (d *fdDevice) DisableGSO() error                               { return nil }

var _ tun.TunDevice = (*fdDevice)(nil)
