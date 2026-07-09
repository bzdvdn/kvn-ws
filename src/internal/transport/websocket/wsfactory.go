package websocket

import (
	"context"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/transport"
)

// @sk-task transport-factory#T1.2: WSFactory implements TransportFactory for WebSocket (AC-002)
type WSFactory struct {
	tlsCfg           *transport.FactoryConfig
	keepaliveInt     time.Duration
	keepaliveTimeout time.Duration
}

// @sk-task transport-factory#T1.2: NewWSFactory creates a WSFactory (AC-002)
func NewWSFactory(cfg *transport.FactoryConfig) *WSFactory {
	return &WSFactory{
		tlsCfg:           cfg,
		keepaliveInt:     cfg.KeepaliveInterval,
		keepaliveTimeout: cfg.KeepaliveTimeout,
	}
}

func (f *WSFactory) Dial(ctx context.Context, endpoint string) (transport.StreamConn, error) {
	wsCfg := WSConfig{
		Multiplex:      f.tlsCfg.Multiplex,
		MTU:            f.tlsCfg.MTU,
		UTLS:           f.tlsCfg.UTLS,
		UTLSFallback:   f.tlsCfg.UTLSFallback,
		PaddingEnabled: f.tlsCfg.PaddingEnabled,
		PaddingSize:    f.tlsCfg.PaddingSize,
	}
	conn, err := Dial(endpoint, f.tlsCfg.TLS, f.tlsCfg.Logger, wsCfg)
	if err != nil {
		return nil, err
	}
	if f.keepaliveInt > 0 && f.keepaliveTimeout > 0 {
		conn.SetKeepalive(f.keepaliveInt, f.keepaliveTimeout)
	}
	return conn, nil
}

func (f *WSFactory) Listen(ctx context.Context, addr string) (transport.TransportListener, error) {
	return nil, nil
}

func init() {
	transport.Register("ws", func(cfg *transport.FactoryConfig) transport.TransportFactory {
		return NewWSFactory(cfg)
	})
}
