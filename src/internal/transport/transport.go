package transport

import "time"

// @sk-task arch-refactoring#T1.2: unified StreamConn interface (AC-003)
type StreamConn interface {
	ReadMessage() ([]byte, error)
	WriteMessage([]byte) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Close() error
}

// @sk-task kvn-web-redesign#T4.1: CountingStreamConn wraps StreamConn to count bytes (AC-013)
type CountingStreamConn struct {
	StreamConn
	AddTX func(n int64)
	AddRX func(n int64)
}

func (c *CountingStreamConn) ReadMessage() ([]byte, error) {
	data, err := c.StreamConn.ReadMessage()
	if err == nil && c.AddRX != nil {
		c.AddRX(int64(len(data)))
	}
	return data, err
}

func (c *CountingStreamConn) WriteMessage(data []byte) error {
	if c.AddTX != nil {
		c.AddTX(int64(len(data)))
	}
	return c.StreamConn.WriteMessage(data)
}
