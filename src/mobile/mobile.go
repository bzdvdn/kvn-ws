// Package mobile exposes the v2 data plane to Android via gomobile.
package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

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
	inner  *client.Client
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
	inner := client.New(&cfg, zap.NewNop())
	c := &Client{dev: dev, inner: inner, done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go func() {
		defer close(c.done)
		err := inner.RunTUNDevice(ctx, dev)
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

// RxBytes returns download bytes written into TUN.
func (c *Client) RxBytes() int64 { return c.dev.rx.Load() }

// TxBytes returns upload bytes read from TUN.
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

// Health checks the running session transport with a bounded timeout. It
// reuses the live HTTP/2 connection, so it reflects the actual data plane.
func (c *Client) Health() error {
	if c.inner == nil {
		return fmt.Errorf("mobile session not started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.inner.Health(ctx)
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

const tunPollTimeout = 100 * time.Millisecond

type fdDevice struct {
	fd   int
	rx   atomic.Int64
	tx   atomic.Int64
	stop chan struct{}
	once sync.Once
}

func newFDDevice(fd int) *fdDevice {
	// The VpnService descriptor is blocking by default. Non-blocking mode lets
	// reads be interrupted instead of stranding a goroutine in read(2).
	_ = syscall.SetNonblock(fd, true)
	return &fdDevice{fd: fd, stop: make(chan struct{})}
}

func (d *fdDevice) Open() error { return nil }

// Close signals the packet pump to stop. Kotlin VpnService owns the fd, so Go
// never closes it; the owner closes it after RunTUNDevice returns.
func (d *fdDevice) Close() error {
	d.once.Do(func() { close(d.stop) })
	return nil
}

func (d *fdDevice) stopped() bool {
	select {
	case <-d.stop:
		return true
	default:
		return false
	}
}

// waitReady blocks until the fd is ready for events or the device is stopped.
func (d *fdDevice) waitReady(events int16) bool {
	fds := []unix.PollFd{{Fd: int32(d.fd), Events: events}} // #nosec G115 -- Linux fd fits in int32
	for {
		if d.stopped() {
			return false
		}
		n, err := unix.Poll(fds, int(tunPollTimeout/time.Millisecond))
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return false
		}
		if n == 0 {
			continue
		}
		if fds[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return false
		}
		return true
	}
}

// Read reads packets from the Android TUN fd: outbound (upload) traffic.
func (d *fdDevice) Read(p []byte) (int, error) {
	for {
		if d.stopped() {
			return 0, io.EOF
		}
		n, err := syscall.Read(d.fd, p)
		if err == nil {
			if n > 0 {
				d.tx.Add(int64(n))
			}
			return n, nil
		}
		if err != syscall.EAGAIN && err != syscall.EWOULDBLOCK {
			return n, err
		}
		if !d.waitReady(unix.POLLIN) {
			return 0, io.EOF
		}
	}
}

// Write writes packets into the Android TUN fd: inbound (download) traffic.
func (d *fdDevice) Write(p []byte) (int, error) {
	for {
		if d.stopped() {
			return 0, io.EOF
		}
		n, err := syscall.Write(d.fd, p)
		if err == nil {
			if n > 0 {
				d.rx.Add(int64(n))
			}
			return n, nil
		}
		if err != syscall.EAGAIN && err != syscall.EWOULDBLOCK {
			return n, err
		}
		if !d.waitReady(unix.POLLOUT) {
			return 0, io.EOF
		}
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
