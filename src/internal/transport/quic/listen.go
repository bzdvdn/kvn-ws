package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
)

// @sk-task quic-transport#T2.2: QUIC listener (AC-001)
type Listener struct {
	ln *quic.Listener
}

func Listen(addr string, tlsConf *tls.Config, quicConf *quic.Config) (*Listener, error) {
	ln, err := quic.ListenAddr(addr, tlsConf, quicConf)
	if err != nil {
		return nil, err
	}
	return &Listener{ln: ln}, nil
}

func (l *Listener) Accept(ctx context.Context) (*QUICConn, error) {
	conn, err := l.ln.Accept(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return NewQUICConn(conn, stream), nil
}

func (l *Listener) Close() error {
	return l.ln.Close()
}

func (l *Listener) Addr() string {
	return l.ln.Addr().String()
}

const (
	backoffBase = 10 * time.Millisecond
	backoffMax  = 5 * time.Second
)

// @sk-task arch-fix-critical-paths#T2.1: AcceptWithBackoff — exponential backoff for transient Accept errors (AC-001)
func AcceptWithBackoff(ctx context.Context, acceptFn func(ctx context.Context) (*QUICConn, error), logger *zap.Logger) (*QUICConn, error) {
	delay := backoffBase
	for {
		conn, err := acceptFn(ctx)
		if err == nil {
			return conn, nil
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		logger.Error("quic accept transient error, retrying with backoff", zap.Error(err))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
		if delay > backoffMax {
			delay = backoffMax
		}
	}
}
