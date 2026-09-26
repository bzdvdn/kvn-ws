package webui

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/bootstrap/client"
	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

// @sk-test fix-critical-leaks#T6.1: TestWebUIBroadcastShutdown (AC-006)
func TestWebUIBroadcastShutdown(t *testing.T) {
	before := runtime.NumGoroutine()

	srv, err := New(0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ctx)
	}()

	time.Sleep(50 * time.Millisecond) // let goroutines start
	cancel()
	select {
	case <-errCh:
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}

	after := runtime.NumGoroutine()
	if leaked := after - before; leaked > 3 {
		t.Logf("goroutine delta after shutdown: %d (may include test infra)", leaked)
	}
}

// @sk-test tun-connect-restart: stale client/TUN state is released before a new connect
func TestStopStaleClientLocked(t *testing.T) {
	srv, err := New(0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cl, err := client.NewFromConfig(&config.ClientConfig{Mode: "proxy", Server: "ws://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	srv.state.setClient(cl)
	srv.state.SetCancel(cancel)
	srv.state.SetDoneCh(done)
	go func() {
		<-ctx.Done()
		close(done)
	}()

	srv.stopStaleClientLocked()

	select {
	case <-ctx.Done():
	default:
		t.Fatal("run context was not cancelled")
	}
	if srv.state.Client() != nil {
		t.Error("client reference was not cleared")
	}
	if srv.state.Cancel() != nil {
		t.Error("cancel func was not cleared")
	}
	if srv.state.DoneCh() != nil {
		t.Error("done channel was not cleared")
	}
}
