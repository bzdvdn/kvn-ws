package tunnel

import (
	"bytes"
	"context"
	"errors"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
)

// mockTun satisfies tun.TunDevice for testing purposes.
type mockTun struct{}

func (m *mockTun) Open() error                                                           { return nil }
func (m *mockTun) Close() error                                                          { return nil }
func (m *mockTun) Read(b []byte) (int, error)                                            { return 0, nil }
func (m *mockTun) Write(b []byte) (int, error)                                           { return len(b), nil }
func (m *mockTun) SetIP(ip net.IP, mask *net.IPNet) error                                { return nil }
func (m *mockTun) SetMTU(mtu int) error                                                  { return nil }
func (m *mockTun) SetGateway(gateway net.IP) error                                       { return nil }
func (m *mockTun) RemoveGateway(gateway net.IP) error                                    { return nil }
func (m *mockTun) AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error { return nil }
func (m *mockTun) RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	return nil
}
func (m *mockTun) CleanupExcludeRoutes() {}

func (m *mockTun) SetDNS(dnsServers []string) error { return nil }

func (m *mockTun) DisableGSO() error { return nil }

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

func (m *mockStreamConn) WriteMessage(data []byte) error     { return nil }
func (m *mockStreamConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockStreamConn) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockStreamConn) Close() error                       { return nil }

// mockTunWrite tracks writes for verification.
type mockTunWrite struct {
	mu      sync.Mutex
	written [][]byte
}

func (m *mockTunWrite) Open() error                { return nil }
func (m *mockTunWrite) Close() error               { return nil }
func (m *mockTunWrite) Read(b []byte) (int, error) { return 0, nil }
func (m *mockTunWrite) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	buf := make([]byte, len(b))
	copy(buf, b)
	m.written = append(m.written, buf)
	return len(b), nil
}
func (m *mockTunWrite) SetIP(ip net.IP, mask *net.IPNet) error { return nil }
func (m *mockTunWrite) SetMTU(mtu int) error                   { return nil }
func (m *mockTunWrite) SetGateway(gateway net.IP) error        { return nil }
func (m *mockTunWrite) RemoveGateway(gateway net.IP) error     { return nil }
func (m *mockTunWrite) AddExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	return nil
}
func (m *mockTunWrite) RemoveExcludeRoute(cidr string, phyGateway net.IP, phyIface string) error {
	return nil
}
func (m *mockTunWrite) CleanupExcludeRoutes() {}

func (m *mockTunWrite) SetDNS(dnsServers []string) error { return nil }

func (m *mockTunWrite) DisableGSO() error { return nil }

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
	if !bytes.Equal(tunW.written[0], payload) {
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
	if !errors.Is(err, context.Canceled) {
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

// @sk-test dns-upstreams-list#T4.1: TestServerDNSForwardUsesConfig (AC-006)
func TestServerDNSForwardUsesConfig(t *testing.T) {
	// Start a mock UDP upstream
	upstreamConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer upstreamConn.Close()
	upstreamAddr := upstreamConn.LocalAddr().String()

	// Build a DNS query and wrap it in a DNS frame
	query := []byte{
		0x00, 0x01, // TXID
		0x01, 0x00, // flags: standard query
		0x00, 0x01, // questions: 1
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // rest
	}
	// Add label "test.example.com"
	query = append(query, 4, 't', 'e', 's', 't', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0x00, 0x00, 0x01, 0x00, 0x01) // QTYPE A, QCLASS IN

	// Construct payload: [4-byte streamID][query]
	payload := make([]byte, 4+len(query))
	payload[3] = 1 // streamID = 1 (big-endian)
	copy(payload[4:], query)

	frameData := encodeFrame(t, &framing.Frame{
		Type:    framing.FrameTypeDNS,
		Payload: payload,
	})

	stream := &mockStreamConn{messages: [][]byte{frameData}}
	s := &Session{
		tunDev:        &mockTun{},
		stream:        stream,
		logger:        zap.NewNop(),
		tunnelTimeout: time.Second,
		dnsUpstreams:  []string{upstreamAddr},
	}

	// Read from upstream to verify query arrives
	done := make(chan struct{})
	var gotQuery []byte
	go func() {
		buf := make([]byte, 1500)
		n, clientAddr, err := upstreamConn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		gotQuery = make([]byte, n)
		copy(gotQuery, buf[:n])

		// Send minimal response to unblock forwardDNS
		resp := make([]byte, 16)
		copy(resp, buf[:2]) // copy TXID
		resp[2] = 0x81      // response flags
		resp[3] = 0x80
		_, _ = upstreamConn.WriteToUDP(resp, clientAddr)
		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.wsToTun(ctx)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: upstream did not receive DNS query")
	}

	if len(gotQuery) < 12 {
		t.Fatal("upstream received incomplete DNS query")
	}
}
