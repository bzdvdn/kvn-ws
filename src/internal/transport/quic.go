package transport

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
)

const (
	quicMsgAuth     byte = 1
	quicMsgAuthOK   byte = 2
	quicMsgData     byte = 3
	quicMsgReply    byte = 4
	quicMsgClose    byte = 5
	quicMsgFallback byte = 6

	// MaxQUICPayload is the inner UDP payload that fits a QUIC DATAGRAM frame
	// on a 1200-byte path after the KVN flow/target header and QUIC+AEAD
	// overhead. Larger datagrams are moved to the reliable HTTP/2 UDP stream
	// instead of being silently dropped by the QUIC packetizer.
	MaxQUICPayload = 1100
)

// ErrDatagramTooLarge reports that a UDP payload does not fit the QUIC
// datagram budget and should be carried by the HTTP/2 UDP stream instead.
var ErrDatagramTooLarge = errors.New("udp packet exceeds QUIC datagram budget")

func makeFallbackDatagram(id uint32) []byte {
	msg := make([]byte, 5)
	msg[0] = quicMsgFallback
	binary.BigEndian.PutUint32(msg[1:5], id)
	return msg
}

// QUICServer carries best-effort UDP flows over QUIC datagrams.
type QUICServer struct {
	listener *quic.Listener
	tokens   TokenLookup
	logger   *zap.Logger
}

// ListenQUIC starts a QUIC datagram listener with ALPN h3.
func ListenQUIC(addr string, tlsCfg *tls.Config, tokens TokenLookup, logger *zap.Logger) (*QUICServer, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	cfgTLS := tlsCfg.Clone()
	cfgTLS.NextProtos = []string{"h3"}
	ln, err := quic.ListenAddr(addr, cfgTLS, &quic.Config{
		EnableDatagrams:      true,
		MaxIdleTimeout:       2 * time.Minute,
		KeepAlivePeriod:      30 * time.Second,
		HandshakeIdleTimeout: 3 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return &QUICServer{listener: ln, tokens: tokens, logger: logger}, nil
}

// Addr returns the UDP listen address.
func (s *QUICServer) Addr() net.Addr { return s.listener.Addr() }

// Serve accepts QUIC datagram connections until ctx is canceled.
func (s *QUICServer) Serve(ctx context.Context) error {
	for {
		conn, err := s.listener.Accept(ctx)
		if err != nil {
			return err
		}
		go s.handle(ctx, conn)
	}
}

func (s *QUICServer) handle(ctx context.Context, conn quic.Connection) {
	var (
		sendMu     sync.Mutex
		flowsMu    sync.Mutex
		flows      = make(map[uint32]*serverUDPFlow)
		authorized bool
	)
	send := func(data []byte) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return conn.SendDatagram(data)
	}
	closeFlows := func() {
		flowsMu.Lock()
		for id, f := range flows {
			f.close()
			delete(flows, id)
		}
		flowsMu.Unlock()
	}
	defer closeFlows()
	for {
		data, err := conn.ReceiveDatagram(ctx)
		if err != nil {
			return
		}
		if len(data) == 0 {
			continue
		}
		switch data[0] {
		case quicMsgAuth:
			if len(data) < 3 {
				continue
			}
			n := int(binary.BigEndian.Uint16(data[1:3]))
			if len(data) < 3+n {
				continue
			}
			token := string(data[3 : 3+n])
			if _, ok := s.tokens(token); !ok {
				_ = conn.CloseWithError(0, "unauthorized")
				return
			}
			authorized = true
			_ = send([]byte{quicMsgAuthOK})
		case quicMsgData:
			if !authorized || len(data) < 8 {
				continue
			}
			id := binary.BigEndian.Uint32(data[1:5])
			targetLen := int(binary.BigEndian.Uint16(data[5:7]))
			if len(data) < 7+targetLen {
				continue
			}
			target := string(data[7 : 7+targetLen])
			payload := data[7+targetLen:]
			if len(payload) > MaxQUICPayload {
				for i := 0; i < 3; i++ {
					if serr := send(makeFallbackDatagram(id)); serr != nil {
						break
					}
				}
				flowsMu.Lock()
				if f := flows[id]; f != nil {
					f.close()
					delete(flows, id)
				}
				flowsMu.Unlock()
				continue
			}
			flowsMu.Lock()
			f := flows[id]
			if f == nil {
				var ferr error
				f, ferr = newServerUDPFlow(id, target, send, func() {
					flowsMu.Lock()
					delete(flows, id)
					flowsMu.Unlock()
				})
				if ferr != nil {
					flowsMu.Unlock()
					s.logger.Debug("quic udp target", zap.String("target", target), zap.Error(ferr))
					continue
				}
				flows[id] = f
			}
			flowsMu.Unlock()
			f.write(payload)
		case quicMsgClose:
			if len(data) < 5 {
				continue
			}
			id := binary.BigEndian.Uint32(data[1:5])
			flowsMu.Lock()
			if f := flows[id]; f != nil {
				f.close()
				delete(flows, id)
			}
			flowsMu.Unlock()
		}
	}
}

type serverUDPFlow struct {
	conn   *net.UDPConn
	send   func([]byte) error
	remove func()
	done   chan struct{}
	once   sync.Once
	id     uint32
}

func newServerUDPFlow(id uint32, target string, send func([]byte) error, remove func()) (*serverUDPFlow, error) {
	addr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}
	f := &serverUDPFlow{id: id, conn: conn, send: send, remove: remove, done: make(chan struct{})}
	go f.readLoop()
	return f, nil
}

func (f *serverUDPFlow) write(payload []byte) {
	if len(payload) > MaxQUICPayload {
		return
	}
	_, _ = f.conn.Write(payload)
}

func (f *serverUDPFlow) readLoop() {
	defer f.close()
	buf := make([]byte, 65535)
	for {
		_ = f.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := f.conn.Read(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				select {
				case <-f.done:
					return
				default:
					continue
				}
			}
			return
		}
		if n > MaxQUICPayload {
			// The reply does not fit a QUIC datagram on this path; ask the
			// client to carry this flow over the HTTP/2 UDP stream.
			for i := 0; i < 3; i++ {
				if serr := f.send(makeFallbackDatagram(f.id)); serr != nil {
					break
				}
			}
			if f.remove != nil {
				f.remove()
			}
			return
		}
		msg := make([]byte, 5+n)
		msg[0] = quicMsgReply
		binary.BigEndian.PutUint32(msg[1:5], f.id)
		copy(msg[5:], buf[:n])
		if err := f.send(msg); err != nil {
			return
		}
	}
}

func (f *serverUDPFlow) close() {
	f.once.Do(func() {
		close(f.done)
		_ = f.conn.Close()
	})
}

// QUICClient is one shared QUIC datagram connection for all UDP flows.
type QUICClient struct {
	conn   quic.Connection
	logger *zap.Logger
	sendMu sync.Mutex

	nextID     atomic.Uint32
	mu         sync.Mutex
	flows      map[uint32]*QUICFlow
	done       chan struct{}
	once       sync.Once
	authOK     chan struct{}
	loopCancel context.CancelFunc
	dead       atomic.Bool
	onFallback func(target string)
}

// NewQUICClient dials the QUIC datagram endpoint and authenticates once.
func NewQUICClient(ctx context.Context, endpoint, token string, tlsCfg *tls.Config, logger *zap.Logger) (*QUICClient, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	cfgTLS := tlsCfg.Clone()
	cfgTLS.NextProtos = []string{"h3"}
	conn, err := quic.DialAddr(ctx, endpoint, cfgTLS, &quic.Config{
		EnableDatagrams:      true,
		MaxIdleTimeout:       2 * time.Minute,
		KeepAlivePeriod:      30 * time.Second,
		HandshakeIdleTimeout: 3 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	c := &QUICClient{conn: conn, logger: logger, flows: make(map[uint32]*QUICFlow), done: make(chan struct{}), authOK: make(chan struct{}, 1)}
	// The read loop must outlive the dial context: callers bound startup with
	// a short timeout and cancel it once the handshake finishes.
	loopCtx, loopCancel := context.WithCancel(context.Background())
	c.loopCancel = loopCancel
	go c.readLoop(loopCtx)
	auth := makeAuthDatagram(token)
	for attempt := 0; attempt < 8; attempt++ {
		if err := c.send(auth); err != nil {
			_ = c.Close()
			return nil, err
		}
		select {
		case <-c.authOK:
			return c, nil
		case <-ctx.Done():
			_ = c.Close()
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	_ = c.Close()
	return nil, errors.New("quic auth timeout")
}

func makeAuthDatagram(token string) []byte {
	msg := make([]byte, 3+len(token))
	msg[0] = quicMsgAuth
	binary.BigEndian.PutUint16(msg[1:3], uint16(len(token))) // #nosec G115 -- token length is config-bounded
	copy(msg[3:], token)
	return msg
}

func (c *QUICClient) send(data []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.conn.SendDatagram(data)
}

func (c *QUICClient) readLoop(ctx context.Context) {
	defer func() {
		c.dead.Store(true)
		c.closeAll()
	}()
	for {
		data, err := c.conn.ReceiveDatagram(ctx)
		if err != nil {
			return
		}
		if len(data) == 0 {
			continue
		}
		switch data[0] {
		case quicMsgAuthOK:
			select {
			case c.authOK <- struct{}{}:
			default:
			}
		case quicMsgReply:
			if len(data) < 5 {
				continue
			}
			id := binary.BigEndian.Uint32(data[1:5])
			c.mu.Lock()
			f := c.flows[id]
			c.mu.Unlock()
			if f != nil {
				f.deliver(data[5:])
			}
		case quicMsgFallback:
			if len(data) < 5 {
				continue
			}
			id := binary.BigEndian.Uint32(data[1:5])
			c.mu.Lock()
			f := c.flows[id]
			handler := c.onFallback
			delete(c.flows, id)
			c.mu.Unlock()
			if f != nil {
				_ = f.Close()
				if handler != nil {
					handler(f.target)
				}
			}
		}
	}
}

// SetFallbackHandler registers a callback invoked when the peer asks to move a
// flow from unreliable QUIC datagrams to the reliable HTTP/2 UDP stream.
func (c *QUICClient) SetFallbackHandler(fn func(target string)) {
	c.mu.Lock()
	c.onFallback = fn
	c.mu.Unlock()
}

// DialFlow creates a logical UDP flow inside the shared QUIC connection.
// It fails once the QUIC connection is dead so the caller can fall back to the
// HTTP/2 UDP stream instead of blackholing datagrams.
func (c *QUICClient) DialFlow(_ context.Context, target string) (*QUICFlow, error) {
	if c.dead.Load() || c.conn.Context().Err() != nil {
		return nil, errors.New("quic connection closed")
	}
	id := c.nextID.Add(1)
	f := &QUICFlow{client: c, id: id, target: target, in: make(chan []byte, 1024), done: make(chan struct{})}
	c.mu.Lock()
	c.flows[id] = f
	c.mu.Unlock()
	return f, nil
}

// Close closes the QUIC connection.
func (c *QUICClient) Close() error {
	var err error
	c.once.Do(func() {
		c.dead.Store(true)
		if c.loopCancel != nil {
			c.loopCancel()
		}
		close(c.done)
		err = c.conn.CloseWithError(0, "client close")
	})
	return err
}

func (c *QUICClient) closeAll() {
	c.mu.Lock()
	for id, f := range c.flows {
		f.closeLocal()
		delete(c.flows, id)
	}
	c.mu.Unlock()
}

func (c *QUICClient) removeFlow(id uint32) {
	c.mu.Lock()
	delete(c.flows, id)
	c.mu.Unlock()
}

// QUICFlow is one UDP flow over QUIC datagrams.
type QUICFlow struct {
	client *QUICClient
	id     uint32
	target string
	in     chan []byte
	done   chan struct{}
	once   sync.Once
}

func (f *QUICFlow) WritePacket(payload []byte) error {
	if f.client.dead.Load() {
		return errors.New("quic connection closed")
	}
	if len(payload) > MaxQUICPayload {
		return fmt.Errorf("%w: payload %d budget %d", ErrDatagramTooLarge, len(payload), MaxQUICPayload)
	}
	msg := make([]byte, 7+len(f.target)+len(payload))
	msg[0] = quicMsgData
	binary.BigEndian.PutUint32(msg[1:5], f.id)
	binary.BigEndian.PutUint16(msg[5:7], uint16(len(f.target))) // #nosec G115 -- target length is validated by config
	copy(msg[7:], f.target)
	copy(msg[7+len(f.target):], payload)
	err := f.client.send(msg)
	if err != nil {
		var tooLarge *quic.DatagramTooLargeError
		if errors.As(err, &tooLarge) {
			return fmt.Errorf("%w: %w", ErrDatagramTooLarge, err)
		}
	}
	return err
}

func (f *QUICFlow) ReadPacket() ([]byte, error) {
	select {
	case payload := <-f.in:
		return payload, nil
	case <-f.done:
		return nil, errors.New("quic flow closed")
	}
}

func (f *QUICFlow) Close() error {
	f.closeLocal()
	f.client.removeFlow(f.id)
	msg := make([]byte, 5)
	msg[0] = quicMsgClose
	binary.BigEndian.PutUint32(msg[1:5], f.id)
	_ = f.client.send(msg)
	return nil
}

func (f *QUICFlow) deliver(payload []byte) {
	cp := make([]byte, len(payload))
	copy(cp, payload)
	select {
	case f.in <- cp:
	default:
	}
}

func (f *QUICFlow) closeLocal() {
	f.once.Do(func() { close(f.done) })
}
