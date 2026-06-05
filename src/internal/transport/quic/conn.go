// @sk-task quic-transport#T2.1: QUICConn wrapper implementing StreamConn (AC-001)
package quic

import (
	"time"

	"github.com/quic-go/quic-go"
)

type QUICConn struct {
	stream quic.Stream
}

func NewQUICConn(stream quic.Stream) *QUICConn {
	return &QUICConn{stream: stream}
}

func (c *QUICConn) ReadMessage() ([]byte, error) {
	buf := make([]byte, 4096)
	n, err := c.stream.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (c *QUICConn) WriteMessage(data []byte) error {
	_, err := c.stream.Write(data)
	return err
}

func (c *QUICConn) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

func (c *QUICConn) SetWriteDeadline(t time.Time) error {
	return c.stream.SetWriteDeadline(t)
}

func (c *QUICConn) Close() error {
	return c.stream.Close()
}

func (c *QUICConn) StreamID() quic.StreamID {
	return c.stream.StreamID()
}
