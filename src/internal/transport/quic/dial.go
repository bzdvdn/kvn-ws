package quic

import (
	"context"
	"crypto/tls"
	"net/url"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
)

// @sk-task quic-transport#T3.1: strip URL to host:port for QUIC dial
func dialAddr(endpoint string) string {
	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		if err == nil && u.Host != "" {
			return u.Host
		}
	}
	return endpoint
}

const DefaultDialTimeout = 10 * time.Second

// @sk-task quic-transport#T3.1: QUIC dial function (AC-001)
// @sk-task fix-critical-leaks#T2.1: QUIC Dial ctx param (AC-004)
func Dial(ctx context.Context, addr string, tlsConf *tls.Config, quicConf *quic.Config) (*QUICConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, DefaultDialTimeout)
	defer cancel()

	conn, err := quic.DialAddr(dialCtx, dialAddr(addr), tlsConf, quicConf)
	if err != nil {
		return nil, err
	}
	stream, err := conn.OpenStreamSync(dialCtx)
	if err != nil {
		return nil, err
	}
	return NewQUICConn(conn, stream), nil
}
