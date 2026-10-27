package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	wst "github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
)

// @sk-test fallback-auth-stealth#T4.2: stealth path failure falls back to legacy (AC-006)
func TestDialStealthFallsBackToLegacy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/legacy", func(w http.ResponseWriter, r *http.Request) {
		conn, err := wst.Accept(w, r, zap.NewNop())
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		time.Sleep(50 * time.Millisecond)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := &config.ClientConfig{
		Server: "ws" + srv.URL[len("http"):] + "/legacy",
		TLS:    config.ClientTLSCfg{VerifyMode: "insecure"},
		Auth:   config.AuthCfg{Token: "secret"},
		Stealth: &config.StealthCfg{
			Enabled:    true,
			Path:       "/stealth",
			LegacyPath: "/legacy",
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dialStreamWith(ctx, cfg, zap.NewNop(), false)
	if err != nil {
		t.Fatalf("dialStreamWith: %v", err)
	}
	_ = conn.Close()
}

// @sk-test fallback-auth-stealth#T4.2: stealth dial sends standard Bearer (AC-001, AC-005)
func TestDialStealthSendsBearer(t *testing.T) {
	authCh := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/stealth", func(w http.ResponseWriter, r *http.Request) {
		authCh <- r.Header.Get("Authorization")
		conn, err := wst.Accept(w, r, zap.NewNop())
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		time.Sleep(50 * time.Millisecond)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := &config.ClientConfig{
		Server:  "ws" + srv.URL[len("http"):] + "/legacy",
		TLS:     config.ClientTLSCfg{VerifyMode: "insecure"},
		Auth:    config.AuthCfg{Token: "secret"},
		Stealth: &config.StealthCfg{Enabled: true, Path: "/stealth"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := dialStreamWith(ctx, cfg, zap.NewNop(), false)
	if err != nil {
		t.Fatalf("dialStreamWith: %v", err)
	}
	_ = conn.Close()
	select {
	case auth := <-authCh:
		if auth != "Bearer secret" {
			t.Fatalf("Authorization = %q, want Bearer secret", auth)
		}
	case <-time.After(time.Second):
		t.Fatal("stealth path not dialed")
	}
}
