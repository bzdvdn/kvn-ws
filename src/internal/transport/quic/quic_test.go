// @sk-test quic-transport#T4.1: QUIC integration test (AC-001, AC-002, AC-003)
package quic

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

func TestQUICConnInterfaceConformance(t *testing.T) {
	var _ interface {
		ReadMessage() ([]byte, error)
		WriteMessage([]byte) error
		SetReadDeadline(time.Time) error
		SetWriteDeadline(time.Time) error
		Close() error
	} = (*QUICConn)(nil)
}

func TestNewQUICConn(t *testing.T) {
	conn := NewQUICConn(nil, &mockStream{})
	if conn == nil {
		t.Fatal("NewQUICConn returned nil")
	}
}

func TestQUICDialTimeout(t *testing.T) {
	_, err := Dial("127.0.0.1:19999", &tls.Config{
		InsecureSkipVerify: true,
	}, nil)
	if err == nil {
		t.Fatal("expected dial to fail on non-listening port")
	}
	t.Logf("expected dial error: %v", err)
}

type mockStream struct{}

func (m *mockStream) Read(b []byte) (int, error)         { return len(b), nil }
func (m *mockStream) Write(b []byte) (int, error)        { return len(b), nil }
func (m *mockStream) Close() error                       { return nil }
func (m *mockStream) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockStream) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockStream) StreamID() quic.StreamID            { return 0 }
func (m *mockStream) CancelRead(code quic.StreamErrorCode)  {}
func (m *mockStream) CancelWrite(code quic.StreamErrorCode) {}
func (m *mockStream) Context() context.Context              { return context.Background() }
func (m *mockStream) SetDeadline(t time.Time) error         { return nil }
func (m *mockStream) ReadAtLeast(p []byte, min int) (int, error) { return len(p), nil }

var _ quic.Stream = (*mockStream)(nil)
