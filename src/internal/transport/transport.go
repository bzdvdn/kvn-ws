package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

// @sk-task arch-refactoring#T1.2: unified StreamConn interface (AC-003)
type StreamConn interface {
	ReadMessage() ([]byte, error)
	WriteMessage([]byte) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Close() error
}

// @sk-task quic-datagrams#T1.1: optional unreliable-datagram capability on a StreamConn (AC-001)
type DatagramConn interface {
	SupportsDatagrams() bool
	SendDatagram(payload []byte) error
	ReceiveDatagram(ctx context.Context) ([]byte, error)
}

// @sk-task ws-media-queue-backpressure#T3.3: optional deadline-safe write path
// avoids control frames clobbering a data write deadline (AC-008)
type DeadlineWriter interface {
	WriteMessageWithDeadline(data []byte, deadline time.Time) error
}

// WriteMessageWithDeadline writes one message using a single deadline-critical
// section when the transport supports it, falling back to SetWriteDeadline plus
// WriteMessage otherwise.
func WriteMessageWithDeadline(c StreamConn, data []byte, deadline time.Time) error {
	if dw, ok := c.(DeadlineWriter); ok {
		return dw.WriteMessageWithDeadline(data, deadline)
	}
	if err := c.SetWriteDeadline(deadline); err != nil {
		return err
	}
	return c.WriteMessage(data)
}

// @sk-task kvn-web-redesign#T4.1: CountingStreamConn wraps StreamConn to count bytes (AC-013)
type CountingStreamConn struct {
	StreamConn
	AddTX func(n int64)
	AddRX func(n int64)
}

func (c *CountingStreamConn) ReadMessage() ([]byte, error) {
	data, err := c.StreamConn.ReadMessage()
	if err == nil && c.AddRX != nil {
		c.AddRX(int64(len(data)))
	}
	return data, err
}

func (c *CountingStreamConn) WriteMessage(data []byte) error {
	if c.AddTX != nil {
		c.AddTX(int64(len(data)))
	}
	return c.StreamConn.WriteMessage(data)
}

// @sk-task ws-media-queue-backpressure#T3.3: forward deadline-safe writes (AC-008)
func (c *CountingStreamConn) WriteMessageWithDeadline(data []byte, deadline time.Time) error {
	if c.AddTX != nil {
		c.AddTX(int64(len(data)))
	}
	return WriteMessageWithDeadline(c.StreamConn, data, deadline)
}

// @sk-task quic-datagrams#T3.1: forward datagram capability through the counting wrapper (AC-001)
func (c *CountingStreamConn) SupportsDatagrams() bool {
	if d, ok := c.StreamConn.(DatagramConn); ok {
		return d.SupportsDatagrams()
	}
	return false
}

func (c *CountingStreamConn) SendDatagram(payload []byte) error {
	d, ok := c.StreamConn.(DatagramConn)
	if !ok {
		return errors.New("datagrams unsupported")
	}
	return d.SendDatagram(payload)
}

func (c *CountingStreamConn) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	d, ok := c.StreamConn.(DatagramConn)
	if !ok {
		return nil, errors.New("datagrams unsupported")
	}
	return d.ReceiveDatagram(ctx)
}

// @sk-task transport-factory#T1.1: TransportFactory interface (AC-001)
type TransportFactory interface {
	Dial(ctx context.Context, endpoint string) (StreamConn, error)
	Listen(ctx context.Context, addr string) (TransportListener, error)
}

// @sk-task transport-factory#T1.1: TransportListener interface (AC-001)
type TransportListener interface {
	Accept(ctx context.Context) (StreamConn, error)
	Close() error
}

// @sk-task transport-factory#T1.1: FactoryConfig aggregates transport configuration (AC-001)
type FactoryConfig struct {
	TLS            *tls.Config
	Logger         *zap.Logger
	MaxMessageSize int

	KeepaliveInterval time.Duration
	KeepaliveTimeout  time.Duration

	Multiplex      bool
	MTU            int
	UTLS           bool
	UTLSFallback   bool
	PaddingEnabled bool
	PaddingSize    int
	// @sk-task stealth-surface-hardening#T1.1: fixed|random padding policy (AC-001)
	PaddingMode string
	// @sk-task stealth-surface-hardening#T1.3: configurable no-pad/subprotocol aliases (AC-009, AC-010)
	NoPadHeaders          []string
	MultiplexSubprotocols []string
	HeadersProfile        string
	KeepaliveJitterPct    int
	// @sk-task game-latency#T3.2: request unpadded secondary framing (AC-001)
	RequestNoPad bool

	Obfuscation bool
}

var (
	factoriesMu sync.Mutex
	factories   = map[string]func(cfg *FactoryConfig) TransportFactory{}
)

// @sk-task transport-factory#T1.1: Register adds a transport factory constructor (AC-001)
func Register(name string, fn func(cfg *FactoryConfig) TransportFactory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[name] = fn
}

// @sk-task transport-factory#T1.1: NewFactory creates a TransportFactory by type (AC-001)
// typeStr: "quic" or "ws" (empty defaults to "ws").
func NewFactory(typeStr string, cfg *FactoryConfig) TransportFactory {
	if typeStr == "" || typeStr == "tcp" {
		typeStr = "ws"
	}
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	fn, ok := factories[typeStr]
	if !ok {
		fn = factories["ws"]
	}
	if fn == nil {
		return nil
	}
	return fn(cfg)
}

// @sk-task transport-factory#T2.1: FallbackFactory wraps primary+secondary with automatic fallback (AC-007)
type FallbackFactory struct {
	primary   TransportFactory
	secondary TransportFactory
	logger    *zap.Logger
}

func NewFallbackFactory(primary, secondary TransportFactory, logger *zap.Logger) *FallbackFactory {
	return &FallbackFactory{
		primary:   primary,
		secondary: secondary,
		logger:    logger,
	}
}

func (f *FallbackFactory) Dial(ctx context.Context, endpoint string) (StreamConn, error) {
	conn, err := f.primary.Dial(ctx, endpoint)
	if err == nil {
		return conn, nil
	}
	f.logger.Warn("primary transport dial failed, falling back", zap.Error(err))
	return f.secondary.Dial(ctx, endpoint)
}

func (f *FallbackFactory) Listen(ctx context.Context, addr string) (TransportListener, error) {
	return f.primary.Listen(ctx, addr)
}
