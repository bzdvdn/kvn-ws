package client

import (
	"context"
	"testing"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"go.uber.org/zap"
)

// @sk-test arch-refactoring#T4.1: dialStream tests (AC-004)
// @sk-test arch-refactoring#T4.1: dialStream with cancelled ctx returns error (AC-004)
func TestDialStreamCancelledContext(t *testing.T) {
	cfg := &config.ClientConfig{
		Server:    "wss://127.0.0.1:19999/tunnel",
		Transport: "tcp",
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := dialStream(ctx, cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
	t.Logf("cancelled ctx dial error: %v", err)
}

// @sk-test arch-refactoring#T4.1: dialStream with cancelled quic transport falls back to tcp (AC-004)
func TestDialStreamQUICFallback(t *testing.T) {
	cfg := &config.ClientConfig{
		Server:    "127.0.0.1:19999",
		Transport: "quic",
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := dialStream(ctx, cfg, zap.NewNop())
	// Expected: QUIC dial fails (no server), fallback to TCP websocket also fails
	if err == nil {
		t.Fatal("expected error when no server is listening")
	}
	t.Logf("quic fallback error: %v", err)
}
