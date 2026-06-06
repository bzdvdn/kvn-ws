package tunnel

import (
	"context"
	"net"
	"runtime"
	"testing"
	"time"
)

// mockTun satisfies tun.TunDevice for testing purposes.
type mockTun struct{}

func (m *mockTun) Open() error                                               { return nil }
func (m *mockTun) Close() error                                              { return nil }
func (m *mockTun) Read(b []byte) (int, error)                               { return 0, nil }
func (m *mockTun) Write(b []byte) (int, error)                              { return len(b), nil }
func (m *mockTun) SetIP(ip net.IP, mask *net.IPNet) error                   { return nil }
func (m *mockTun) SetMTU(mtu int) error                                     { return nil }
func (m *mockTun) SetGateway(gateway net.IP) error                          { return nil }
func (m *mockTun) RemoveGateway(gateway net.IP) error                       { return nil }
func (m *mockTun) AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error { return nil }
func (m *mockTun) RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error { return nil }
func (m *mockTun) DisableGSO() error                                         { return nil }

// @sk-test fix-critical-leaks#T6.1: TestTunGoroutineLeak (AC-001)
func TestTunGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	for range 10 {
		ctx, cancel := context.WithCancel(context.Background())
		s := &Session{
			tunDev: &mockTun{},
		}
		s.startTunReader(ctx)
		time.Sleep(time.Millisecond)
		cancel()
		time.Sleep(time.Millisecond)
	}
	after := runtime.NumGoroutine()
	if leaked := after - before; leaked > 5 {
		t.Errorf("goroutine leak: %d goroutines after 10 iterations", leaked)
	}
}
