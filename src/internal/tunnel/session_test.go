package tunnel

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	"github.com/bzdvdn/kvn-ws/src/internal/proxy"
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

func proxyPayload(sid uint32, dst string, data []byte) []byte {
	p := make([]byte, 4+2+len(dst)+len(data))
	binary.BigEndian.PutUint32(p[0:4], sid)
	binary.BigEndian.PutUint16(p[4:6], uint16(len(dst))) // #nosec G115 — bounded by protocol
	copy(p[6:], dst)
	copy(p[6+len(dst):], data)
	return p
}

// recStream records frames written back to the transport stream (e.g. close frames).
type recStream struct {
	*mockStreamConn
	mu  sync.Mutex
	out [][]byte
}

func (r *recStream) WriteMessage(data []byte) error {
	r.mu.Lock()
	buf := make([]byte, len(data))
	copy(buf, data)
	r.out = append(r.out, buf)
	r.mu.Unlock()
	return nil
}

// @sk-test proxy-slow-dial: wsToTun read-loop must not block on a proxy target
// dial. Stream 1 uses a non-routable destination (dial may hang), stream 2 must
// still reach its listener while that dial is pending.
func TestProxyDialAsyncDoesNotBlockLoop(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	echoAddr := ln.Addr().String()
	received := make(chan string, 1)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 4096)
			n, _ := c.Read(buf)
			received <- string(buf[:n])
			_ = c.Close()
		}
	}()

	f1 := encodeFrame(t, &framing.Frame{
		Type:    framing.FrameTypeProxy,
		Payload: proxyPayload(1, "203.0.113.9:81", []byte("ping1")),
	})
	f2 := encodeFrame(t, &framing.Frame{
		Type:    framing.FrameTypeProxy,
		Payload: proxyPayload(2, echoAddr, []byte("ping2")),
	})

	stream := &recStream{mockStreamConn: &mockStreamConn{messages: [][]byte{f1, f2}}}
	s := &Session{
		stream:        stream,
		logger:        zap.NewNop(),
		proxyStreams:  proxy.NewSessionStreams(),
		proxySem:      make(chan struct{}, 256),
		tunnelTimeout: 2 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = s.wsToTun(ctx) }()

	select {
	case got := <-received:
		if got != "ping2" {
			t.Fatalf("echo = %q, want %q", got, "ping2")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("read-loop was blocked: stream 2 not relayed while stream 1 dial pending")
	}
}

// @sk-test proxy-slow-dial: on a failed target dial the server writes a close
// frame with an empty destination back to the client.
func TestProxyDialFailureSendsCloseFrame(t *testing.T) {
	f := encodeFrame(t, &framing.Frame{
		Type:    framing.FrameTypeProxy,
		Payload: proxyPayload(42, "127.0.0.1:9", []byte("ping")),
	})

	stream := &recStream{mockStreamConn: &mockStreamConn{messages: [][]byte{f}}}
	s := &Session{
		stream:        stream,
		logger:        zap.NewNop(),
		proxyStreams:  proxy.NewSessionStreams(),
		proxySem:      make(chan struct{}, 256),
		tunnelTimeout: 2 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = s.wsToTun(ctx) }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stream.mu.Lock()
		for _, msg := range stream.out {
			var fr framing.Frame
			if err := fr.Decode(msg); err != nil {
				continue
			}
			if fr.Type == framing.FrameTypeProxy && len(fr.Payload) >= 6 {
				sid := binary.BigEndian.Uint32(fr.Payload[0:4])
				dstLen := binary.BigEndian.Uint16(fr.Payload[4:6])
				if sid == 42 && dstLen == 0 {
					stream.mu.Unlock()
					return
				}
			}
		}
		stream.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no close frame delivered for a failed proxy dial")
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

// @sk-test dual-ws-channel#T4.2: parseIPProto classification (AC-002)
func TestParseIPProto(t *testing.T) {
	v4udp := make([]byte, 20)
	v4udp[0] = 0x45
	v4udp[9] = 17
	v4tcp := make([]byte, 20)
	v4tcp[0] = 0x45
	v4tcp[9] = 6
	v6udp := make([]byte, 40)
	v6udp[0] = 0x60
	v6udp[6] = 17
	v6tcp := make([]byte, 40)
	v6tcp[0] = 0x60
	v6tcp[6] = 6
	v6ext := make([]byte, 40)
	v6ext[0] = 0x60
	v6ext[6] = 0x2b // hop-by-hop ext header → fallback primary

	tests := []struct {
		name string
		pkt  []byte
		want bool
	}{
		{"ipv4 udp", v4udp, true},
		{"ipv4 tcp", v4tcp, false},
		{"ipv6 udp", v6udp, true},
		{"ipv6 tcp", v6tcp, false},
		{"ipv6 ext header", v6ext, false},
		{"short v4", []byte{0x45, 0x00}, false},
		{"short v6", []byte{0x60}, false},
		{"empty", nil, false},
		{"garbage", []byte{0xaa, 0xbb, 0xcc}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseIPProto(tc.pkt); got != tc.want {
				t.Errorf("parseIPProto(%q) = %v, want %v", tc.pkt, got, tc.want)
			}
		})
	}
}

// queuedStreamConn lets tests push messages in discrete steps.
type queuedStreamConn struct {
	mu       sync.Mutex
	cond     *sync.Cond
	messages [][]byte
	closed   bool
}

func newQueuedStreamConn() *queuedStreamConn {
	q := &queuedStreamConn{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *queuedStreamConn) push(msg []byte) {
	q.mu.Lock()
	q.messages = append(q.messages, msg)
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (q *queuedStreamConn) ReadMessage() ([]byte, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.messages) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.messages) == 0 {
		return nil, net.ErrClosed
	}
	msg := q.messages[0]
	q.messages = q.messages[1:]
	return msg, nil
}

func (q *queuedStreamConn) WriteMessage(data []byte) error     { return nil }
func (q *queuedStreamConn) SetReadDeadline(t time.Time) error  { return nil }
func (q *queuedStreamConn) SetWriteDeadline(t time.Time) error { return nil }
func (q *queuedStreamConn) Close() error {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
	return nil
}

func waitWritten(t *testing.T, tunW *mockTunWrite, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		tunW.mu.Lock()
		n := len(tunW.written)
		tunW.mu.Unlock()
		if n == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	tunW.mu.Lock()
	defer tunW.mu.Unlock()
	t.Fatalf("expected %d writes to tun, got %d", want, len(tunW.written))
}

// @sk-test dual-ws-channel#T4.2: buffer accumulates until primaryReady, flushes after (AC-005)
func TestSecondaryBufferAccumulatesThenFlushes(t *testing.T) {
	tunW := &mockTunWrite{}
	secondary := newQueuedStreamConn()
	s := &Session{
		tunDev:        tunW,
		logger:        zap.NewNop(),
		tunnelTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.setRunCtx(ctx)
	s.SetSecondary(secondary)

	frameMsg := func(payload []byte) []byte {
		return encodeFrame(t, &framing.Frame{Type: framing.FrameTypeData, Payload: payload})
	}

	// primary not ready yet: three frames must be buffered, not written.
	secondary.push(frameMsg([]byte("p1")))
	secondary.push(frameMsg([]byte("p2")))
	secondary.push(frameMsg([]byte("p3")))
	time.Sleep(50 * time.Millisecond)
	tunW.mu.Lock()
	nBefore := len(tunW.written)
	tunW.mu.Unlock()
	if nBefore != 0 {
		t.Fatalf("primary not ready: expected 0 tun writes, got %d", nBefore)
	}

	// Primary becomes ready; next frame flushes buffered + current.
	s.primaryReady.Store(true)
	secondary.push(frameMsg([]byte("p4")))
	waitWritten(t, tunW, 4)

	tunW.mu.Lock()
	defer tunW.mu.Unlock()
	want := [][]byte{[]byte("p1"), []byte("p2"), []byte("p3"), []byte("p4")}
	for i, p := range want {
		if !bytes.Equal(tunW.written[i], p) {
			t.Errorf("write %d = %q, want %q", i, tunW.written[i], p)
		}
	}
}

// @sk-test dual-ws-channel#T4.2: buffer drops incoming once full (1080 pushed, only 1024 buffered) (AC-005)
func TestSecondaryBufferDropsOnOverflow(t *testing.T) {
	tunW := &mockTunWrite{}
	secondary := newQueuedStreamConn()
	s := &Session{
		tunDev:        tunW,
		logger:        zap.NewNop(),
		tunnelTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.setRunCtx(ctx)
	s.SetSecondary(secondary)

	frameMsg := func(payload []byte) []byte {
		return encodeFrame(t, &framing.Frame{Type: framing.FrameTypeData, Payload: payload})
	}

	// Push far more than the 1024-packet cap while primary not ready.
	for i := 0; i < 1500; i++ {
		secondary.push(frameMsg([]byte{byte(i)}))
	}
	time.Sleep(100 * time.Millisecond)

	// Ready + one more frame → flush the 1024 buffered packets + the final one.
	s.primaryReady.Store(true)
	secondary.push(frameMsg([]byte("end")))
	waitWritten(t, tunW, 1024+1)

	tunW.mu.Lock()
	defer tunW.mu.Unlock()
	// Buffered must be the first 1024 (byte(0..1023)); excess was dropped.
	for i := 0; i < 1024; i++ {
		if !bytes.Equal(tunW.written[i], []byte{byte(i)}) {
			t.Fatalf("buffered write %d = %q, want byte(%d)", i, tunW.written[i], i)
		}
	}
	if !bytes.Equal(tunW.written[1024], []byte("end")) {
		t.Errorf("final write = %q, want %q", tunW.written[1024], "end")
	}
}

// @sk-test dual-ws-channel#T4.2: secondary read decrypts with shared session cipher (AC-006)
func TestSecondaryCryptoCrossChannel(t *testing.T) {
	masterKey := bytes.Repeat([]byte{0x42}, 32)
	salt := bytes.Repeat([]byte{0xaa}, 32)
	sessionID := "abcdef0123456789abcdef0123456789"
	cipher, err := crypto.NewSessionCipher(masterKey, salt, sessionID)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}

	tunW := &mockTunWrite{}
	secondary := newQueuedStreamConn()
	plaintext := []byte("encrypted-on-secondary-channel")
	enc, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	secondary.push(encodeFrame(t, &framing.Frame{Type: framing.FrameTypeData, Payload: enc}))

	s := &Session{
		tunDev:        tunW,
		logger:        zap.NewNop(),
		cipher:        cipher,
		tunnelTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.setRunCtx(ctx)
	s.primaryReady.Store(true)
	s.SetSecondary(secondary)

	waitWritten(t, tunW, 1)
	tunW.mu.Lock()
	defer tunW.mu.Unlock()
	if !bytes.Equal(tunW.written[0], plaintext) {
		t.Errorf("secondary decrypt = %q, want %q", tunW.written[0], plaintext)
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
