// @sk-task quic-transport#T1.1: StreamConn interface for transport abstraction (AC-001, AC-004)
package tunnel

import "time"

type StreamConn interface {
	ReadMessage() ([]byte, error)
	WriteMessage([]byte) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Close() error
}
