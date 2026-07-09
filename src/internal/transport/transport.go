package transport

import (
	"context"
	"crypto/tls"
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
