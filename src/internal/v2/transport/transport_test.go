package transport

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func echoTarget(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

// @sk-test kvn-v2-transport#T5.1: HTTP/2 stream reaches a TCP target and echoes (AC-001)
func TestStreamRoundTrip(t *testing.T) {
	target, closeTarget := echoTarget(t)
	defer closeTarget()

	srv := NewServer(func(token string) (string, bool) {
		return "test", token == "secret"
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }), 64, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tlsCfg := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}
	stream, err := Dial(ctx, ts.URL+Path, "secret", target, tlsCfg)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = stream.Close() }()

	payload := []byte("hello-v2")
	if _, err := stream.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != string(payload) {
		t.Fatalf("echo = %q, want %q", buf, payload)
	}
}

// @sk-test kvn-v2-transport#T5.1: invalid Bearer gets fallback, not a tunnel (AC-003)
func TestFallbackWithoutBearer(t *testing.T) {
	srv := NewServer(func(token string) (string, bool) { return "", false },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html>site</html>"))
		}), 8, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := Dial(ctx, ts.URL+Path, "bad", "127.0.0.1:1", &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}})
	if err == nil {
		t.Fatal("expected status error for fallback")
	}
}

// @sk-test kvn-v2-transport#T5.1: best-effort UDP flow roundtrip over HTTP/2 (AC-008)
func TestUDPStreamRoundTrip(t *testing.T) {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, rerr := udpConn.ReadFromUDP(buf)
			if rerr != nil {
				return
			}
			_, _ = udpConn.WriteToUDP(buf[:n], addr)
		}
	}()

	srv := NewServer(func(token string) (string, bool) { return "t", token == "secret" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }), 64, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := DialUDP(ctx, ts.URL+UDPPath, "secret", udpConn.LocalAddr().String(),
		&tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}})
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer func() { _ = stream.Close() }()
	payload := []byte("udp-v2")
	if err := stream.WritePacket(payload); err != nil {
		t.Fatalf("write packet: %v", err)
	}
	got, err := stream.ReadPacket()
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("udp echo = %q, want %q", got, payload)
	}
}

// @sk-test kvn-v2-transport#T5.1: slow TCP flow does not block another flow (AC-002)
func TestSlowFlowDoesNotBlockAnother(t *testing.T) {
	slowLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer slowLn.Close()
	go func() {
		for {
			c, err := slowLn.Accept()
			if err != nil {
				return
			}
			defer c.Close() // hold the connection open without reading
		}
	}()
	fastTarget, closeFast := echoTarget(t)
	defer closeFast()

	srv := NewServer(func(token string) (string, bool) { return "t", token == "secret" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }), 64, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	tlsCfg := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}
	client, err := NewClient(ts.URL+Path, "secret", tlsCfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	slow, err := client.DialTCP(ctx, slowLn.Addr().String())
	if err != nil {
		t.Fatalf("slow dial: %v", err)
	}
	defer func() { _ = slow.Close() }()
	if _, err := slow.Write([]byte("hold")); err != nil {
		t.Fatalf("slow write: %v", err)
	}

	fast, err := client.DialTCP(ctx, fastTarget)
	if err != nil {
		t.Fatalf("fast dial while slow flow open: %v", err)
	}
	defer func() { _ = fast.Close() }()
	payload := []byte("fast")
	if _, err := fast.Write(payload); err != nil {
		t.Fatalf("fast write: %v", err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(fast, got); err != nil {
		t.Fatalf("fast read: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("fast echo = %q", got)
	}
}
