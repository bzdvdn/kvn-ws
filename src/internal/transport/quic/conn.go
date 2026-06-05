// @sk-task quic-transport#T2.1: QUICConn wrapper implementing StreamConn (AC-001)
package quic

import (
	"encoding/binary"
	"io"
	"math"
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
	var lenBuf [4]byte
	if _, err := io.ReadFull(c.stream, lenBuf[:]); err != nil {
		return nil, err
	}
	msgLen := binary.BigEndian.Uint32(lenBuf[:])
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(c.stream, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (c *QUICConn) WriteMessage(data []byte) error {
	if len(data) > math.MaxUint32 {
		return io.ErrShortWrite
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := c.stream.Write(lenBuf[:]); err != nil {
		return err
	}
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
