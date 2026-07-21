package proxy

import (
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
)

const streamWriteChanSize = 64

// @sk-task local-proxy-mode#T2.2: ProxyStream and stream management (AC-001)
// @sk-task post-hardening#T3.4: sessionProxyStreams extracted (AC-012)
// @sk-task quic-proxy-mode#T2.1: local StreamConn for cycle avoidance (AC-001, AC-003)
// @sk-task arch-refactoring#T2.2: type alias to transport.StreamConn (AC-003)
// StreamConn is the minimal interface needed by Manager/ForwardToStream.
// Implemented by websocket.WSConn and quic.QUICConn.
type StreamConn = transport.StreamConn

// @sk-task post-hardening#T3.4: per-session proxy stream container (AC-012)
// @sk-task lock-optimization#T3.5: Load → RLock (AC-007)
type SessionStreams struct {
	mu sync.RWMutex
	m  map[uint32]net.Conn
}

func NewSessionStreams() *SessionStreams {
	return &SessionStreams{m: make(map[uint32]net.Conn)}
}

func (s *SessionStreams) Load(key uint32) (net.Conn, bool) {
	s.mu.RLock()
	v, ok := s.m[key]
	s.mu.RUnlock()
	return v, ok
}

func (s *SessionStreams) Store(key uint32, val net.Conn) {
	s.mu.Lock()
	s.m[key] = val
	s.mu.Unlock()
}

func (s *SessionStreams) Delete(key uint32) {
	s.mu.Lock()
	delete(s.m, key)
	s.mu.Unlock()
}

func (s *SessionStreams) CloseAll() {
	s.mu.Lock()
	for _, conn := range s.m {
		_ = conn.Close()
	}
	s.m = make(map[uint32]net.Conn)
	s.mu.Unlock()
}

var nextStreamID uint32

func NewStreamID() uint32 {
	return atomic.AddUint32(&nextStreamID, 1)
}

type Stream struct {
	ID      uint32
	Dst     string
	Local   net.Conn
	writeCh chan []byte
	stopWr  sync.Once
}

// startWriter launches a dedicated goroutine that drains writeCh and writes
// to Local sequentially.  HandleIncomingFrame enqueues instead of blocking
// the read-loop.
func (s *Stream) startWriter() {
	s.writeCh = make(chan []byte, streamWriteChanSize)
	go func() {
		for data := range s.writeCh {
			if _, err := s.Local.Write(data); err != nil {
				return
			}
		}
	}()
}

func (s *Stream) stopWriter() {
	s.stopWr.Do(func() {
		if s.writeCh != nil {
			close(s.writeCh)
		}
	})
}

// @sk-task quic-proxy-mode#T2.1: ForwardToWS → ForwardToStream (AC-001, AC-003)
// @sk-task fix-client-block#T1.1: write deadline to prevent mutex deadlock (AC-001)
func (s *Stream) ForwardToStream(stream StreamConn) {
	defer func() { _ = s.Local.Close() }()
	buf := make([]byte, 4096)
	for {
		n, err := s.Local.Read(buf)
		if err != nil {
			return
		}
		payload := make([]byte, 4+2+len(s.Dst)+n)
		binary.BigEndian.PutUint32(payload[0:4], s.ID)
		binary.BigEndian.PutUint16(payload[4:6], uint16(len(s.Dst))) // #nosec G115 — bounded by protocol
		copy(payload[6:], s.Dst)
		copy(payload[6+len(s.Dst):], buf[:n])

		f := framing.Frame{
			Type:    framing.FrameTypeProxy,
			Flags:   framing.FrameFlagNone,
			Payload: payload,
		}
		encoded, err := f.Encode()
		if err != nil {
			return
		}
		_ = stream.SetWriteDeadline(time.Now().Add(120 * time.Second))
		if err := stream.WriteMessage(encoded); err != nil {
			framing.ReturnBuffer(encoded)
			return
		}
		framing.ReturnBuffer(encoded)
	}
}

// @sk-task quic-proxy-mode#T2.1: Manager.wsConn → Manager.stream (AC-001, AC-003)
// @sk-task lock-optimization#T3.5: Get → RLock (AC-008)
type Manager struct {
	mu      sync.RWMutex
	streams map[uint32]*Stream
	stream  StreamConn
	logf    func(format string, args ...any)
}

// @sk-task quic-proxy-mode#T2.1: NewManager takes StreamConn (AC-001, AC-003)
func NewManager(stream StreamConn, logfn ...func(string, ...any)) *Manager {
	var lf func(string, ...any)
	if len(logfn) > 0 {
		lf = logfn[0]
	}
	return &Manager{
		streams: make(map[uint32]*Stream),
		stream:  stream,
		logf:    lf,
	}
}

func (m *Manager) Add(s *Stream) {
	s.startWriter()
	m.mu.Lock()
	m.streams[s.ID] = s
	m.mu.Unlock()
}

func (m *Manager) Get(id uint32) *Stream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.streams[id]
}

func (m *Manager) Remove(id uint32) {
	m.mu.Lock()
	s, ok := m.streams[id]
	delete(m.streams, id)
	m.mu.Unlock()
	if ok && s != nil {
		s.stopWriter()
	}
}

// @sk-task local-proxy-mode#T2.2: route incoming proxy frame to local TCP conn (AC-001)
// @sk-task win-proxy-perf#T2.2: async per-stream writer, read-loop never blocks (AC-001)
func (m *Manager) HandleIncomingFrame(f *framing.Frame) {
	payload := f.Payload
	if len(payload) < 6 {
		return
	}
	streamID := binary.BigEndian.Uint32(payload[0:4])
	dstLen := binary.BigEndian.Uint16(payload[4:6])
	if int(6+dstLen) > len(payload) {
		return
	}
	data := payload[6+dstLen:]

	s := m.Get(streamID)
	if s == nil {
		return
	}

	if len(data) == 0 {
		s.stopWriter()
		_ = s.Local.Close()
		m.Remove(streamID)
		return
	}

	// Enqueue to per-stream writer goroutine.
	// Blocking send is deliberate — it applies backpressure when the stream
	// cannot keep up, without closing the connection.
	tmp := make([]byte, len(data))
	copy(tmp, data)
	s.writeCh <- tmp
}
