// @sk-task quic-transport#T3.1: QUIC dial function (AC-001)
package quic

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/quic-go/quic-go"
)

const DefaultDialTimeout = 10 * time.Second

func Dial(addr string, tlsConf *tls.Config, quicConf *quic.Config) (*QUICConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultDialTimeout)
	defer cancel()

	conn, err := quic.DialAddr(ctx, addr, tlsConf, quicConf)
	if err != nil {
		return nil, err
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return NewQUICConn(stream), nil
}
