package quic

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

var ErrMessageTooLarge = errors.New("message too large")

// @sk-task quic-transport#T2.1: QUICConn wrapper implementing StreamConn (AC-001)
// @sk-task arch-refactoring#T2.1: add MaxMessageSize limit (AC-001)
type QUICConn struct {
	mu             sync.Mutex
	conn           quic.Connection
	stream         quic.Stream
	maxMessageSize int
}

func NewQUICConn(conn quic.Connection, stream quic.Stream) *QUICConn {
	return &QUICConn{
		conn:           conn,
		stream:         stream,
		maxMessageSize: 10 * 1024 * 1024,
	}
}

func (c *QUICConn) SetMaxMessageSize(size int) {
	if size > 0 {
		c.maxMessageSize = size
	}
}

func (c *QUICConn) ReadMessage() ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(c.stream, lenBuf[:]); err != nil {
		return nil, err
	}
	msgLen := binary.BigEndian.Uint32(lenBuf[:])
	if msgLen > uint32(c.maxMessageSize) {
		return nil, ErrMessageTooLarge
	}
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
	c.mu.Lock()
	defer c.mu.Unlock()
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := c.stream.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := c.stream.Write(data)
	return err
}

func (c *QUICConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stream.SetReadDeadline(t)
}

func (c *QUICConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stream.SetWriteDeadline(t)
}

func (c *QUICConn) Close() error {
	streamErr := c.stream.Close()
	if c.conn != nil {
		_ = c.conn.CloseWithError(0, "close")
	}
	return streamErr
}

func (c *QUICConn) StreamID() quic.StreamID {
	return c.stream.StreamID()
}
