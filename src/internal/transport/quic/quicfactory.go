package quic

import (
	"context"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/bzdvdn/kvn-ws/src/internal/transport"
)

// @sk-task transport-factory#T1.3: QUICFactory implements TransportFactory for QUIC (AC-003)
type QUICFactory struct {
	tlsCfg      *transport.FactoryConfig
	maxMsgSize  int
	obfuscation bool
}

// @sk-task transport-factory#T1.3: NewQUICFactory creates a QUICFactory (AC-003)
func NewQUICFactory(cfg *transport.FactoryConfig) *QUICFactory {
	return &QUICFactory{
		tlsCfg:      cfg,
		maxMsgSize:  cfg.MaxMessageSize,
		obfuscation: cfg.Obfuscation,
	}
}

func (f *QUICFactory) Dial(ctx context.Context, endpoint string) (transport.StreamConn, error) {
	quicCfg := &quic.Config{
		KeepAlivePeriod: 7 * time.Second,
	}
	conn, err := Dial(ctx, endpoint, f.tlsCfg.TLS, quicCfg)
	if err != nil {
		return nil, err
	}
	if f.maxMsgSize > 0 {
		conn.SetMaxMessageSize(f.maxMsgSize)
	}
	if f.obfuscation {
		obfConn, obfErr := NewObfuscatedQUICConn(conn)
		if obfErr != nil {
			return nil, obfErr
		}
		return obfConn, nil
	}
	return conn, nil
}

func (f *QUICFactory) Listen(ctx context.Context, addr string) (transport.TransportListener, error) {
	return nil, nil
}

func init() {
	transport.Register("quic", func(cfg *transport.FactoryConfig) transport.TransportFactory {
		return NewQUICFactory(cfg)
	})
}
