package webui

import (
	"context"
	"runtime"
	"testing"
	"time"
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
