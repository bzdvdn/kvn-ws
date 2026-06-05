// @sk-task quic-transport#T2.2: QUIC listener (AC-001)
package quic

import (
	"context"
	"crypto/tls"

	"github.com/quic-go/quic-go"
)

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
