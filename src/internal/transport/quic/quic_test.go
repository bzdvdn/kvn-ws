package quic

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
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

// duplexStream implements quic.Stream over two independent buffers (one per direction).
type duplexStream struct {
	r *controlledBuf
	w *controlledBuf
}

func (s *duplexStream) Read(b []byte) (int, error)            { return s.r.Read(b) }
func (s *duplexStream) Write(b []byte) (int, error)           { return s.w.Write(b) }
func (s *duplexStream) Close() error                          { return nil }
func (s *duplexStream) SetReadDeadline(t time.Time) error     { return nil }
func (s *duplexStream) SetWriteDeadline(t time.Time) error    { return nil }
func (s *duplexStream) StreamID() quic.StreamID               { return 0 }
func (s *duplexStream) CancelRead(code quic.StreamErrorCode)  {}
func (s *duplexStream) CancelWrite(code quic.StreamErrorCode) {}
func (s *duplexStream) Context() context.Context              { return context.Background() }
func (s *duplexStream) SetDeadline(t time.Time) error         { return nil }

// @sk-test quic-relay-mode#T4: QUICConn round-trip — WriteMessage on one end, ReadMessage on the other,
// verify no length prefix contamination in payload (AC-001).
func TestQUICConnRoundTrip(t *testing.T) {
	clientToServer := &controlledBuf{}
	serverToClient := &controlledBuf{}

	clientStream := &duplexStream{r: serverToClient, w: clientToServer}
	serverStream := &duplexStream{r: clientToServer, w: serverToClient}

	client := NewQUICConn(nil, clientStream)
	server := NewQUICConn(nil, serverStream)

	payload := []byte{0x02, 0x00, 0x00, 0x11, 0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d}
	if err := client.WriteMessage(payload); err != nil {
		t.Fatalf("client WriteMessage: %v", err)
	}

	got, err := server.ReadMessage()
	if err != nil {
		t.Fatalf("server ReadMessage: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("len mismatch: want %d, got %d", len(payload), len(got))
	}
	for i := range payload {
		if got[i] != payload[i] {
			t.Fatalf("byte %d: want 0x%02x, got 0x%02x", i, payload[i], got[i])
		}
	}

	// server → client reverse direction
	reply := []byte{0x02, 0x00, 0x00, 0x05, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}
	if err := server.WriteMessage(reply); err != nil {
		t.Fatalf("server WriteMessage: %v", err)
	}
	gotReply, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("client ReadMessage: %v", err)
	}
	if len(gotReply) != len(reply) {
		t.Fatalf("reply len mismatch: want %d, got %d", len(reply), len(gotReply))
	}
	for i := range reply {
		if gotReply[i] != reply[i] {
			t.Fatalf("reply byte %d: want 0x%02x, got 0x%02x", i, reply[i], gotReply[i])
		}
	}
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

// @sk-test quic-datagrams#T1.3: datagram-capable client/server pair over loopback
func startDatagramPair(t *testing.T) (client, server *QUICConn) {
	t.Helper()
	cert := generateTestCert(t)
	serverTLS := &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"kvn-ws"}}
	ln, err := Listen("127.0.0.1:0", serverTLS, &quic.Config{})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	srvCh := make(chan *QUICConn, 1)
	go func() {
		conn, err := ln.Accept(context.Background())
		if err != nil {
			return
		}
		srvCh <- conn
	}()

	clientTLS := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"kvn-ws"}}
	client, err = Dial(context.Background(), ln.Addr(), clientTLS, &quic.Config{})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// The server's AcceptStream returns only once the client writes stream data.
	if err := client.WriteMessage([]byte("stream-open")); err != nil {
		t.Fatalf("client WriteMessage: %v", err)
	}

	select {
	case srv := <-srvCh:
		t.Cleanup(func() { _ = srv.Close() })
		return client, srv
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server conn")
		return nil, nil
	}
}

// @sk-test quic-datagrams#T1.3: unreliable datagram round-trip (AC-001)
func TestQUICDatagramRoundTrip(t *testing.T) {
	client, server := startDatagramPair(t)
	if !client.SupportsDatagrams() {
		t.Fatal("client: datagrams not supported after negotiation")
	}
	if !server.SupportsDatagrams() {
		t.Fatal("server: datagrams not supported after negotiation")
	}

	payload := []byte("datagram-payload")
	if err := client.SendDatagram(payload); err != nil {
		t.Fatalf("client SendDatagram: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := server.ReceiveDatagram(ctx)
	if err != nil {
		t.Fatalf("server ReceiveDatagram: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("server got %q, want %q", got, payload)
	}

	if err := server.SendDatagram(got); err != nil {
		t.Fatalf("server echo: %v", err)
	}
	echo, err := client.ReceiveDatagram(ctx)
	if err != nil {
		t.Fatalf("client ReceiveDatagram: %v", err)
	}
	if !bytes.Equal(echo, payload) {
		t.Errorf("client got %q, want %q", echo, payload)
	}
}

// @sk-test quic-datagrams#T1.3: oversized datagram reports a max-payload limit (AC-001)
func TestQUICDatagramTooLarge(t *testing.T) {
	client, _ := startDatagramPair(t)
	err := client.SendDatagram(make([]byte, 8000))
	if err == nil {
		t.Fatal("SendDatagram(8000) = nil, want too-large error")
	}
	maxPayload, ok := DatagramTooLarge(err)
	if !ok {
		t.Fatalf("error %v is not DatagramTooLarge", err)
	}
	if maxPayload <= 0 {
		t.Errorf("max datagram payload = %d, want > 0", maxPayload)
	}
}

// @sk-test quic-datagrams#T2.3: transport marker selects datagram capability (AC-003)
func TestDatagramCapableMarker(t *testing.T) {
	if !DatagramCapable(DatagramTransport) {
		t.Errorf("DatagramCapable(%q) = false, want true", DatagramTransport)
	}
	if DatagramCapable("quic") || DatagramCapable("") || DatagramCapable("ws") {
		t.Error("non-marker transport reported datagram-capable")
	}
}

// @sk-test quic-datagrams#T2.3: marker survives ClientHello/ServerHello additively; old peer stays non-capable (AC-003)
func TestDatagramTransportHandshakeRoundTrip(t *testing.T) {
	chFrame, err := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Token:        "tok",
		Transport:    DatagramTransport,
	})
	if err != nil {
		t.Fatalf("EncodeClientHello: %v", err)
	}
	ch, err := handshake.DecodeClientHello(chFrame)
	if err != nil {
		t.Fatalf("DecodeClientHello: %v", err)
	}
	if !DatagramCapable(ch.Transport) {
		t.Errorf("decoded ClientHello transport = %q, want datagram-capable", ch.Transport)
	}

	shFrame, err := handshake.EncodeServerHello(&handshake.ServerHello{
		SessionId:  "aa11bb22cc33dd44ee55ff6600112233",
		AssignedIp: net.ParseIP("10.10.0.2").To4(),
		Mtu:        1400,
		Transport:  DatagramTransport,
	})
	if err != nil {
		t.Fatalf("EncodeServerHello: %v", err)
	}
	sh, err := handshake.DecodeServerHello(shFrame)
	if err != nil {
		t.Fatalf("DecodeServerHello: %v", err)
	}
	if !DatagramCapable(sh.Transport) {
		t.Errorf("decoded ServerHello transport = %q, want datagram-capable", sh.Transport)
	}

	// Legacy peer: no transport marker -> not datagram-capable.
	oldFrame, err := handshake.EncodeClientHello(&handshake.ClientHello{ProtoVersion: handshake.ProtoVersion, Token: "tok"})
	if err != nil {
		t.Fatalf("EncodeClientHello(old): %v", err)
	}
	oldHello, err := handshake.DecodeClientHello(oldFrame)
	if err != nil {
		t.Fatalf("DecodeClientHello(old): %v", err)
	}
	if DatagramCapable(oldHello.Transport) {
		t.Errorf("legacy ClientHello transport = %q, want non-capable", oldHello.Transport)
	}
}

// @sk-test quic-datagrams#T3.3: obfuscated datagram round-trip uses the same nonce envelope (AC-001)
func TestObfuscatedDatagramRoundTrip(t *testing.T) {
	client, server := startDatagramPair(t)
	nonce := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	ocClient, err := NewObfuscatedQUICConn(client)
	if err != nil {
		t.Fatalf("client obfuscation: %v", err)
	}
	ocClient.SetNonce(nonce)
	ocServer, err := NewObfuscatedQUICConn(server)
	if err != nil {
		t.Fatalf("server obfuscation: %v", err)
	}
	ocServer.SetNonce(nonce)

	payload := []byte("obfuscated-datagram")
	if err := ocClient.SendDatagram(payload); err != nil {
		t.Fatalf("SendDatagram: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := ocServer.ReceiveDatagram(ctx)
	if err != nil {
		t.Fatalf("ReceiveDatagram: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("obfuscated datagram round-trip = %q, want %q", got, payload)
	}
}
