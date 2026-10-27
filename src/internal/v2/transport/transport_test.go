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
