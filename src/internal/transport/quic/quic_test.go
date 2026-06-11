package quic

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

// controlledBuf is a thread-safe byte buffer that implements io.ReadWriteCloser.
type controlledBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *controlledBuf) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Read(p)
}

func (b *controlledBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *controlledBuf) Close() error { return nil }

// controlledStream implements quic.Stream over in-memory buffers.
type controlledStream struct {
	r io.Reader
	w io.Writer
}

func (s *controlledStream) Read(b []byte) (int, error)            { return s.r.Read(b) }
func (s *controlledStream) Write(b []byte) (int, error)           { return s.w.Write(b) }
func (s *controlledStream) Close() error                          { return nil }
func (s *controlledStream) SetReadDeadline(t time.Time) error     { return nil }
func (s *controlledStream) SetWriteDeadline(t time.Time) error    { return nil }
func (s *controlledStream) StreamID() quic.StreamID               { return 0 }
func (s *controlledStream) CancelRead(code quic.StreamErrorCode)  {}
func (s *controlledStream) CancelWrite(code quic.StreamErrorCode) {}
func (s *controlledStream) Context() context.Context              { return context.Background() }
func (s *controlledStream) SetDeadline(t time.Time) error         { return nil }

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

// @sk-test arch-refactoring#T4.1: MaxMessageSize limit — msgLen = 0 → ok (AC-001)
func TestQUICConnReadMessageZeroLen(t *testing.T) {
	buf := &controlledBuf{}
	writeLen(buf, 0)
	s := &controlledStream{r: buf, w: buf}
	conn := NewQUICConn(nil, s)
	conn.SetMaxMessageSize(1024)

	data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage for empty msg: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty data, got len=%d", len(data))
	}
}

// @sk-test arch-refactoring#T4.1: MaxMessageSize limit — msgLen = MaxMessageSize → ok (AC-001)
func TestQUICConnReadMessageMaxSize(t *testing.T) {
	buf := &controlledBuf{}
	payload := make([]byte, 1024)
	writeLen(buf, 1024)
	buf.Write(payload)
	s := &controlledStream{r: buf, w: buf}
	conn := NewQUICConn(nil, s)
	conn.SetMaxMessageSize(1024)

	data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage at limit: %v", err)
	}
	if len(data) != 1024 {
		t.Fatalf("expected 1024 bytes, got %d", len(data))
	}
}

// @sk-test arch-refactoring#T4.1: MaxMessageSize limit — msgLen = MaxMessageSize+1 → ErrMessageTooLarge (AC-001)
func TestQUICConnReadMessageOversize(t *testing.T) {
	buf := &controlledBuf{}
	writeLen(buf, 1025)
	payload := make([]byte, 1025)
	buf.Write(payload)
	s := &controlledStream{r: buf, w: buf}
	conn := NewQUICConn(nil, s)
	conn.SetMaxMessageSize(1024)

	_, err := conn.ReadMessage()
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("expected ErrMessageTooLarge, got %v", err)
	}
}

func writeLen(w io.Writer, n int) {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(n))
	_, _ = w.Write(lenBuf[:])
}

// @sk-test fix-critical-leaks#T6.1: TestQUICDialContextCancel (AC-004)
func TestQUICDialContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Dial(ctx, "127.0.0.1:19999", &tls.Config{
		InsecureSkipVerify: true,
	}, nil)
	if err == nil {
		t.Fatal("expected dial to fail with cancelled context")
	}
	t.Logf("dial error with cancelled ctx: %v", err)
}

func TestQUICDialTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Dial(ctx, "127.0.0.1:19999", &tls.Config{
		InsecureSkipVerify: true,
	}, nil)
	if err == nil {
		t.Fatal("expected dial to fail on non-listening port")
	}
	t.Logf("expected dial error: %v", err)
}

type mockStream struct{}

func (m *mockStream) Read(b []byte) (int, error)                    { return len(b), nil }
func (m *mockStream) Write(b []byte) (int, error)                   { return len(b), nil }
func (m *mockStream) Close() error                                  { return nil }
func (m *mockStream) SetReadDeadline(t time.Time) error             { return nil }
func (m *mockStream) SetWriteDeadline(t time.Time) error            { return nil }
func (m *mockStream) StreamID() quic.StreamID                       { return 0 }
func (m *mockStream) CancelRead(code quic.StreamErrorCode)          {}
func (m *mockStream) CancelWrite(code quic.StreamErrorCode)         {}
func (m *mockStream) Context() context.Context                      { return context.Background() }
func (m *mockStream) SetDeadline(t time.Time) error                 { return nil }
func (m *mockStream) ReadAtLeast(p []byte, minLen int) (int, error) { return len(p), nil }

var _ quic.Stream = (*mockStream)(nil)
