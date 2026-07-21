package quic

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
)

var ErrMessageTooLarge = errors.New("message too large")

var readBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 1500)
		return &b
	},
}

func getReadBuf(size int) []byte {
	ptr := readBufPool.Get().(*[]byte)
	buf := *ptr
	if cap(buf) < size {
		buf = make([]byte, size)
	}
	return buf[:size]
}

func putReadBuf(buf []byte) {
	readBufPool.Put(&buf)
}

// @sk-task quic-transport#T2.1: QUICConn wrapper implementing StreamConn (AC-001)
// @sk-task arch-refactoring#T2.1: add MaxMessageSize limit (AC-001)
// @sk-task performance-scope-p2#T1.1: ReadMessage sync.Pool (AC-001)
// @sk-task performance-scope-p2#T2.1: WriteMessage без mu, deadlineMu (AC-007)
// @sk-task performance-scope-p2#T2.3: maxMessageSize atomic.Int32 (AC-007)
type QUICConn struct {
	deadlineMu     sync.Mutex
	conn           quic.Connection
	stream         quic.Stream
	maxMessageSize atomic.Int32
}

func NewQUICConn(conn quic.Connection, stream quic.Stream) *QUICConn {
	c := &QUICConn{
		conn:   conn,
		stream: stream,
	}
	c.maxMessageSize.Store(10 * 1024 * 1024)
	return c
}

func (c *QUICConn) SetMaxMessageSize(size int) {
	if size > 0 {
		c.maxMessageSize.Store(int32(size))
	}
}

// @sk-task performance-scope-p2#T1.1: sync.Pool for read buffer (AC-001)
func (c *QUICConn) ReadMessage() ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(c.stream, lenBuf[:]); err != nil {
		return nil, err
	}
	msgLen := binary.BigEndian.Uint32(lenBuf[:])
	if mms := c.maxMessageSize.Load(); mms >= 0 && msgLen > uint32(mms) { // #nosec G115
		return nil, ErrMessageTooLarge
	}
	buf := getReadBuf(int(msgLen))
	if _, err := io.ReadFull(c.stream, buf); err != nil {
		putReadBuf(buf)
		return nil, err
	}
	return buf, nil
}

// @sk-task performance-scope-p2#T2.1: WriteMessage without mu (AC-007)
func (c *QUICConn) WriteMessage(data []byte) error {
	if len(data) > math.MaxUint32 {
		return io.ErrShortWrite
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data))) // #nosec G115 — checked above
	if _, err := c.stream.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := c.stream.Write(data)
	return err
}

func (c *QUICConn) SetReadDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	return c.stream.SetReadDeadline(t)
}

func (c *QUICConn) SetWriteDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
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
