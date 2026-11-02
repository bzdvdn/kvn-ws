package transport

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

type coverRecorder struct {
	called bool
}

func (c *coverRecorder) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	c.called = true
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<html>cover</html>"))
}

func newRoutingServer(t *testing.T) (*httptest.Server, *coverRecorder) {
	t.Helper()
	cover := &coverRecorder{}
	srv := NewServer(func(token string) (string, bool) {
		return "t", token == "secret"
	}, cover, 64, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return ts, cover
}

func TestRoutingHTTP1ServesCover(t *testing.T) {
	ts, cover := newRoutingServer(t)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/", strings.NewReader("x"))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Host = "127.0.0.1:9"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("h1 request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if !cover.called {
		t.Fatal("HTTP/1.1 v2-looking request did not reach the cover site")
	}
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "application/octet-stream") {
		t.Fatal("HTTP/1.1 request was served as a v2 stream")
	}
}

func TestRoutingInvalidTokenServesCover(t *testing.T) {
	ts, cover := newRoutingServer(t)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/", strings.NewReader("x"))
	req.Header.Set("Authorization", "Bearer invalid")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Host = "127.0.0.1:9"
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("h2 request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if !cover.called {
		t.Fatal("invalid token did not fall back to the cover site")
	}
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "application/octet-stream") {
		t.Fatal("invalid token was accepted as a v2 stream")
	}
}

func TestRoutingValidTokenServesV2(t *testing.T) {
	target, closeTarget := echoTarget(t)
	defer closeTarget()
	ts, cover := newRoutingServer(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/", strings.NewReader("hello"))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Host = target
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("h2 request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if cover.called {
		t.Fatal("valid v2 request was routed to the cover site")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/octet-stream") {
		t.Fatalf("content-type = %q, want octet-stream", ct)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
}

func TestRoutingUnknownPathServesCover(t *testing.T) {
	ts, cover := newRoutingServer(t)
	resp, err := ts.Client().Get(ts.URL + "/some/missing/page")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if !cover.called {
		t.Fatal("unknown path did not reach the cover site")
	}
}

// TestClientWithHTTP2OnlyALPNConnects covers old clients that offer only h2
// against the new server (and a new client against an h2-only server).
func TestClientWithHTTP2OnlyALPNConnects(t *testing.T) {
	target, closeTarget := echoTarget(t)
	defer closeTarget()
	srv := NewServer(func(token string) (string, bool) { return "t", token == "secret" },
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }), 64, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	tlsCfg := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}
	opts := DefaultClientOptions(tlsCfg)
	opts.UTLSProfile = ""
	c, err := NewClientWithOptions(ts.URL, "secret", &opts)
	if err != nil {
		t.Fatalf("NewClientWithOptions: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := c.DialTCP(ctx, target)
	if err != nil {
		t.Fatalf("DialTCP: %v", err)
	}
	defer func() { _ = stream.Close() }()
	if _, err := stream.Write([]byte("legacy")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 6)
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "legacy" {
		t.Fatalf("echo = %q", buf)
	}
}
