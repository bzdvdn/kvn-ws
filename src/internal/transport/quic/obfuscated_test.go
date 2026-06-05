// @sk-test quic-obfuscation#T1.2: XOR roundtrip test (AC-003)
package quic

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

func TestObfuscatedRoundtrip(t *testing.T) {
	toServer := &memBuf{}
	toClient := &memBuf{}

	clientStream := &memStream{r: toClient, w: toServer}
	serverStream := &memStream{r: toServer, w: toClient}

	clientConn := NewQUICConn(nil, clientStream)
	serverConn := NewQUICConn(nil, serverStream)

	clientObf, err := NewObfuscatedQUICConn(clientConn, true)
	if err != nil {
		t.Fatalf("NewObfuscatedQUICConn(client): %v", err)
	}

	serverObf, err := NewObfuscatedQUICConn(serverConn, false)
	if err != nil {
		t.Fatalf("NewObfuscatedQUICConn(server): %v", err)
	}

	payload := []byte("hello obfuscated world")
	if err := clientObf.WriteMessage(payload); err != nil {
		t.Fatalf("client WriteMessage: %v", err)
	}

	got, err := serverObf.ReadMessage()
	if err != nil {
		t.Fatalf("server ReadMessage: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("roundtrip mismatch: got %q, want %q", got, payload)
	}

	data, err := serverObf.ReadMessage()
	if err == nil {
		t.Fatalf("expected EOF, got data len=%d", len(data))
	}
}

func TestObfuscatedNoCorruption(t *testing.T) {
	sizes := []int{0, 1, 64, 1024, 65535}
	for _, size := range sizes {
		t.Run(sizedObfName(size), func(t *testing.T) {
			toServer := &memBuf{}
			toClient := &memBuf{}

			clientStream := &memStream{r: toClient, w: toServer}
			serverStream := &memStream{r: toServer, w: toClient}

			clientConn := NewQUICConn(nil, clientStream)
			serverConn := NewQUICConn(nil, serverStream)

			clientObf, err := NewObfuscatedQUICConn(clientConn, true)
			if err != nil {
				t.Fatalf("NewObfuscatedQUICConn(client): %v", err)
			}

			serverObf, err := NewObfuscatedQUICConn(serverConn, false)
			if err != nil {
				t.Fatalf("NewObfuscatedQUICConn(server): %v", err)
			}

			payload := make([]byte, size)
			for i := range payload {
				payload[i] = byte(i % 251)
			}

			if err := clientObf.WriteMessage(payload); err != nil {
				t.Fatalf("client WriteMessage: %v", err)
			}

			got, err := serverObf.ReadMessage()
			if err != nil {
				t.Fatalf("server ReadMessage: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("roundtrip mismatch for size %d", size)
			}
		})
	}
}

type memBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *memBuf) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Read(p)
}

func (b *memBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

type memStream struct {
	r io.Reader
	w io.Writer
}

func (s *memStream) Read(b []byte) (int, error)  { return s.r.Read(b) }
func (s *memStream) Write(b []byte) (int, error) { return s.w.Write(b) }
func (s *memStream) Close() error                 { return nil }
func (s *memStream) SetReadDeadline(t time.Time) error  { return nil }
func (s *memStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *memStream) StreamID() quic.StreamID            { return 0 }
func (s *memStream) CancelRead(code quic.StreamErrorCode)  {}
func (s *memStream) CancelWrite(code quic.StreamErrorCode) {}
func (s *memStream) Context() context.Context              { return context.Background() }
func (s *memStream) SetDeadline(t time.Time) error         { return nil }

func sizedObfName(n int) string {
	switch {
	case n == 0:
		return "zero"
	case n < 1024:
		return "small"
	default:
		return "medium"
	}
}
