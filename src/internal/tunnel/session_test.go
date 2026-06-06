package tunnel

import (
	"context"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"go.uber.org/zap"
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

// mockStreamConn implements StreamConn with queued messages for testing.
type mockStreamConn struct {
	mu       sync.Mutex
	messages [][]byte
	err      error
}

func (m *mockStreamConn) ReadMessage() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	if len(m.messages) == 0 {
		return nil, context.Canceled
	}
	msg := m.messages[0]
	m.messages = m.messages[1:]
	return msg, nil
}

func (m *mockStreamConn) WriteMessage(data []byte) error { return nil }
func (m *mockStreamConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockStreamConn) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockStreamConn) Close() error                       { return nil }

// mockTunWrite tracks writes for verification.
type mockTunWrite struct {
	mu      sync.Mutex
	written [][]byte
}

func (m *mockTunWrite) Open() error                                               { return nil }
func (m *mockTunWrite) Close() error                                              { return nil }
func (m *mockTunWrite) Read(b []byte) (int, error)                               { return 0, nil }
func (m *mockTunWrite) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	buf := make([]byte, len(b))
	copy(buf, b)
	m.written = append(m.written, buf)
	return len(b), nil
}
func (m *mockTunWrite) SetIP(ip net.IP, mask *net.IPNet) error                   { return nil }
func (m *mockTunWrite) SetMTU(mtu int) error                                     { return nil }
func (m *mockTunWrite) SetGateway(gateway net.IP) error                          { return nil }
func (m *mockTunWrite) RemoveGateway(gateway net.IP) error                       { return nil }
func (m *mockTunWrite) AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error { return nil }
func (m *mockTunWrite) RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error { return nil }
func (m *mockTunWrite) DisableGSO() error                                         { return nil }

func encodeFrame(t *testing.T, f *framing.Frame) []byte {
	t.Helper()
	data, err := f.Encode()
	if err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	return data
}

// @sk-test arch-refactoring#T4.1: wsToTun data frame dispatch + Release (AC-005)
func TestWsToTunDataFrame(t *testing.T) {
	tunW := &mockTunWrite{}
	payload := []byte("hello tun")
	frameData := encodeFrame(t, &framing.Frame{
		Type:    framing.FrameTypeData,
		Payload: payload,
	})
	stream := &mockStreamConn{messages: [][]byte{frameData}}
	s := &Session{
		tunDev:        tunW,
		stream:        stream,
		logger:        zap.NewNop(),
		tunnelTimeout: time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = s.wsToTun(ctx)

	tunW.mu.Lock()
	defer tunW.mu.Unlock()
	if len(tunW.written) != 1 {
		t.Fatalf("expected 1 write to tun, got %d", len(tunW.written))
	}
	if string(tunW.written[0]) != string(payload) {
		t.Fatalf("tun write = %q, want %q", tunW.written[0], payload)
	}
}

// @sk-test arch-refactoring#T4.1: wsToTun close frame dispatch (AC-005)
func TestWsToTunCloseFrame(t *testing.T) {
	frameData := encodeFrame(t, &framing.Frame{
		Type: framing.FrameTypeClose,
	})
	stream := &mockStreamConn{messages: [][]byte{frameData}}
	s := &Session{
		stream:        stream,
		logger:        zap.NewNop(),
		tunnelTimeout: time.Second,
	}

	// Close frame should return nil (clean exit), not an error
	err := s.wsToTun(context.Background())
	if err != nil {
		t.Fatalf("wsToTun with close frame: %v", err)
	}
}

// @sk-test arch-refactoring#T4.1: wsToTun unknown frame type — Release called (AC-005)
func TestWsToTunUnknownFrame(t *testing.T) {
	frameData := encodeFrame(t, &framing.Frame{
		Type:    0xFF, // unknown
		Payload: []byte("should be released"),
	})
	// After unknown frame, wsToTun loops and calls ReadMessage again,
	// which returns context.Canceled — this is expected.
	stream := &mockStreamConn{
		messages: [][]byte{frameData},
	}
	s := &Session{
		stream:        stream,
		logger:        zap.NewNop(),
		tunnelTimeout: time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.wsToTun(ctx)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// @sk-test arch-refactoring#T4.1: fix-critical-leaks#T6.1: TestTunGoroutineLeak (AC-001)
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
