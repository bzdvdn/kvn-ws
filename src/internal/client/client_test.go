package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
)

func echoTCP(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(c)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

// @sk-test kvn-v2-transport#T2.0: v2 proxy client tunnels TCP over HTTP/2 (AC-001)
func TestProxyClientEndToEnd(t *testing.T) {
	target, closeTarget := echoTCP(t)
	defer closeTarget()

	srv := transport.NewServer(func(token string) (string, bool) { return "t", token == "secret" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }), 64, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxyAddr := ln.Addr().String()
	_ = ln.Close()

	cfg := &config.V2ClientConfig{
		ActiveServer: "test",
		Servers:      []config.V2ServerEntry{{Name: "test", Endpoint: ts.URL, Token: "secret"}},
		Mode:         "proxy",
		ProxyListen:  proxyAddr,
		TLS:          config.ClientTLSCfg{VerifyMode: "insecure"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = New(cfg, zap.NewNop()).Run(ctx) }()

	var conn net.Conn
	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, err = net.DialTimeout("tcp", proxyAddr, 200*time.Millisecond)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("proxy did not start: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil || resp[1] != 0 {
		t.Fatalf("socks greeting: %v %v", resp, err)
	}
	host, portStr, _ := net.SplitHostPort(target)
	ip := net.ParseIP(host).To4()
	port := 0
	_, _ = fmtSscanf(portStr, &port)
	req := []byte{0x05, 0x01, 0x00, 0x01}
	req = append(req, ip...)
	var pb [2]byte
	binary.BigEndian.PutUint16(pb[:], uint16(port))
	req = append(req, pb[:]...)
	if _, err := conn.Write(req); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply); err != nil || reply[1] != 0 {
		t.Fatalf("socks connect: %v %v", reply, err)
	}
	payload := []byte("v2-proxy-echo")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("echo = %q, want %q", got, payload)
	}
}

func fmtSscanf(s string, port *int) (int, error) {
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		*port = *port*10 + int(c-'0')
	}
	return 1, nil
}
