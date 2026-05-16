// @sk-test tun-data-path#T1.1: MockTunDevice for unit tests (AC-004)

package tun

import (
	"bytes"
	"net"
	"sync"
	"testing"
)

type MockTunDevice struct {
	mu         sync.Mutex
	readQueue  [][]byte
	writeQueue [][]byte
	name       string
	closed     bool
	ip         net.IP
	mask       *net.IPNet
	mtu        int
}

func NewMockTunDevice() *MockTunDevice {
	return &MockTunDevice{
		readQueue:  make([][]byte, 0),
		writeQueue: make([][]byte, 0),
		name:       "mocktun",
	}
}

func (m *MockTunDevice) Open() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = false
	return nil
}

func (m *MockTunDevice) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *MockTunDevice) Read(buf []byte) (int, error) {
	m.mu.Lock()
	if len(m.readQueue) == 0 {
		m.mu.Unlock()
		return 0, nil
	}
	pkt := m.readQueue[0]
	m.readQueue = m.readQueue[1:]
	m.mu.Unlock()
	n := copy(buf, pkt)
	return n, nil
}

func (m *MockTunDevice) Write(buf []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pkt := make([]byte, len(buf))
	copy(pkt, buf)
	m.writeQueue = append(m.writeQueue, pkt)
	return len(buf), nil
}

func (m *MockTunDevice) SetIP(ip net.IP, mask *net.IPNet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ip = ip
	m.mask = mask
	return nil
}

func (m *MockTunDevice) SetMTU(mtu int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mtu = mtu
	return nil
}

func (m *MockTunDevice) Inject(pkt []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(pkt))
	copy(cp, pkt)
	m.readQueue = append(m.readQueue, cp)
}

func (m *MockTunDevice) WrittenPackets() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([][]byte, len(m.writeQueue))
	for i, p := range m.writeQueue {
		cp := make([]byte, len(p))
		copy(cp, p)
		result[i] = cp
	}
	return result
}

func (m *MockTunDevice) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readQueue = nil
	m.writeQueue = nil
}

var _ TunDevice = (*MockTunDevice)(nil)

func TestMockTunInjectAndRead(t *testing.T) {
	m := NewMockTunDevice()
	if err := m.Open(); err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = m.Close() }()

	m.Inject([]byte("injected-packet"))
	buf := make([]byte, 1600)
	n, err := m.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != "injected-packet" {
		t.Errorf("read data = %q, want %q", string(buf[:n]), "injected-packet")
	}
}

func TestMockTunWriteAndCollect(t *testing.T) {
	m := NewMockTunDevice()
	if err := m.Open(); err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = m.Close() }()

	payload := []byte("test-packet-data")
	n, err := m.Write(payload)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len(payload) {
		t.Errorf("write n = %d, want %d", n, len(payload))
	}

	pkts := m.WrittenPackets()
	if len(pkts) != 1 {
		t.Fatalf("written count = %d, want 1", len(pkts))
	}
	if !bytes.Equal(pkts[0], payload) {
		t.Errorf("written data = %q, want %q", pkts[0], payload)
	}
}

func TestMockTunQueueOrder(t *testing.T) {
	m := NewMockTunDevice()
	_ = m.Open()
	defer func() { _ = m.Close() }()

	m.Inject([]byte("packet1"))
	m.Inject([]byte("packet2"))

	buf := make([]byte, 1600)
	n, _ := m.Read(buf)
	if string(buf[:n]) != "packet1" {
		t.Errorf("first read = %q, want packet1", string(buf[:n]))
	}
	n, _ = m.Read(buf)
	if string(buf[:n]) != "packet2" {
		t.Errorf("second read = %q, want packet2", string(buf[:n]))
	}
}

func TestMockTunMultipleWrites(t *testing.T) {
	m := NewMockTunDevice()
	_ = m.Open()
	defer func() { _ = m.Close() }()

	_, _ = m.Write([]byte("pkt1"))
	_, _ = m.Write([]byte("pkt2"))
	_, _ = m.Write([]byte("pkt3"))

	pkts := m.WrittenPackets()
	if len(pkts) != 3 {
		t.Fatalf("written count = %d, want 3", len(pkts))
	}
	if string(pkts[0]) != "pkt1" {
		t.Errorf("packet[0] = %q, want pkt1", string(pkts[0]))
	}
}

func TestMockTunEmptyRead(t *testing.T) {
	m := NewMockTunDevice()
	_ = m.Open()
	defer func() { _ = m.Close() }()

	buf := make([]byte, 1600)
	n, err := m.Read(buf)
	if err != nil {
		t.Fatalf("read on empty queue: %v", err)
	}
	if n != 0 {
		t.Errorf("read on empty queue returned n=%d, want 0", n)
	}
}

func TestMockTunSetIP(t *testing.T) {
	m := NewMockTunDevice()
	_ = m.Open()
	defer func() { _ = m.Close() }()

	ip := net.ParseIP("10.0.0.1")
	mask := &net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(24, 32),
	}
	if err := m.SetIP(ip, mask); err != nil {
		t.Fatalf("set ip: %v", err)
	}
}

func TestMockTunClose(t *testing.T) {
	m := NewMockTunDevice()
	if err := m.Open(); err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
