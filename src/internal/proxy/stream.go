// @sk-task local-proxy-mode#T2.2: ProxyStream and stream management (AC-001)
// @sk-task post-hardening#T3.4: sessionProxyStreams extracted (AC-012)
package proxy

import (
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"

	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
)

// @sk-task post-hardening#T3.4: per-session proxy stream container (AC-012)
type SessionStreams struct {
	mu sync.Mutex
	M  map[uint32]net.Conn
}

func (s *SessionStreams) Load(key uint32) (net.Conn, bool) {
	s.mu.Lock()
	v, ok := s.M[key]
	s.mu.Unlock()
	return v, ok
}

func (s *SessionStreams) Store(key uint32, val net.Conn) {
	s.mu.Lock()
	s.M[key] = val
	s.mu.Unlock()
}

func (s *SessionStreams) Delete(key uint32) {
	s.mu.Lock()
	delete(s.M, key)
	s.mu.Unlock()
}

func (s *SessionStreams) CloseAll() {
	s.mu.Lock()
	for _, conn := range s.M {
		_ = conn.Close()
	}
	s.M = make(map[uint32]net.Conn)
	s.mu.Unlock()
}

var nextStreamID uint32

func NewStreamID() uint32 {
	return atomic.AddUint32(&nextStreamID, 1)
}

type Stream struct {
	ID    uint32
	Dst   string
	Local net.Conn
}

// @sk-task local-proxy-mode#T2.2: forward local TCP data to WS proxy frames (AC-001)
func (s *Stream) ForwardToWS(ws *websocket.WSConn) {
	defer func() { _ = s.Local.Close() }()
	buf := make([]byte, 4096)
	for {
		n, err := s.Local.Read(buf)
		if err != nil {
			return
		}
		payload := make([]byte, 4+2+len(s.Dst)+n)
		binary.BigEndian.PutUint32(payload[0:4], s.ID)
		binary.BigEndian.PutUint16(payload[4:6], uint16(len(s.Dst)))
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
		if err := ws.WriteMessage(encoded); err != nil {
			framing.ReturnBuffer(encoded)
			return
		}
		framing.ReturnBuffer(encoded)
	}
}

type Manager struct {
	mu      sync.Mutex
	streams map[uint32]*Stream
	wsConn  *websocket.WSConn
}

func NewManager(ws *websocket.WSConn) *Manager {
	return &Manager{
		streams: make(map[uint32]*Stream),
		wsConn:  ws,
	}
}

func (m *Manager) Add(s *Stream) {
	m.mu.Lock()
	m.streams[s.ID] = s
	m.mu.Unlock()
}

func (m *Manager) Get(id uint32) *Stream {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streams[id]
}

func (m *Manager) Remove(id uint32) {
	m.mu.Lock()
	delete(m.streams, id)
	m.mu.Unlock()
}

// @sk-task local-proxy-mode#T2.2: route incoming proxy frame to local TCP conn (AC-001)
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
	_, _ = s.Local.Write(data)
}
