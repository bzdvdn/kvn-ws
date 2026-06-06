// @sk-task arch-refactoring#T1.2: unified StreamConn interface (AC-003)
package transport

import "time"

type StreamConn interface {
	ReadMessage() ([]byte, error)
	WriteMessage([]byte) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Close() error
}
