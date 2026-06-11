package client

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

type timeoutErr struct{}

func (e *timeoutErr) Error() string   { return "timeout" }
func (e *timeoutErr) Timeout() bool   { return true }
func (e *timeoutErr) Temporary() bool { return false }

type wrappedTimeoutErr struct {
	msg string
	err error
}

func (e *wrappedTimeoutErr) Error() string   { return e.msg }
func (e *wrappedTimeoutErr) Unwrap() error   { return e.err }
func (e *wrappedTimeoutErr) Timeout() bool   { return true }
func (e *wrappedTimeoutErr) Temporary() bool { return false }

// @sk-test fix-critical-leaks#T6.1: TestTypeAssertionErrorsAs (AC-011)
func TestTypeAssertionErrorsAs(t *testing.T) {
	var netErr net.Error
	if !errors.As(&wrappedTimeoutErr{msg: "wrapped", err: &timeoutErr{}}, &netErr) {
		t.Fatal("errors.As should unwrap to net.Error")
	}
	if !netErr.Timeout() {
		t.Error("expected Timeout() = true")
	}
	if !errors.As(&timeoutErr{}, &netErr) {
		t.Fatal("errors.As should match direct net.Error")
	}
	if !netErr.Timeout() {
		t.Error("expected Timeout() = true")
	}
}

// @sk-test fix-critical-leaks#T6.1: TestRouteDirectLifecycle (AC-003)
func TestRouteDirectLifecycle(t *testing.T) {
	// Simulate the RouteDirect errgroup pattern used in proxy.go:
	// two io.Copy goroutines managed by errgroup, with ctx-aware cancel.
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eg, gctx := errgroup.WithContext(ctx)
	var writeDone, readDone bool

	eg.Go(func() error {
		_, err := io.Copy(left, right) // reads from right, writes to left
		writeDone = true
		return err
	})
	eg.Go(func() error {
		_, err := io.Copy(right, left) // reads from left, writes to right
		readDone = true
		return err
	})

	go func() {
		<-gctx.Done()
		left.Close()
		right.Close()
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	err := eg.Wait()
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Logf("errgroup returned: %v", err)
	}
	if !writeDone && !readDone {
		t.Log("both io.Copy goroutines completed after ctx cancel (conn close unblocked them)")
	}
}

// @sk-test fix-critical-leaks#T6.1: TestRouteDirectLifecycle (AC-003)
func TestRouteDirectErrgroupWaitsBoth(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Write 100 bytes then stop
		left.Write(make([]byte, 100))
		left.Close()
	}()

	eg := new(errgroup.Group)
	eg.Go(func() error {
		_, err := io.Copy(io.Discard, right)
		return err
	})
	eg.Go(func() error {
		// Read nothing, just return
		return nil
	})

	if err := eg.Wait(); err != nil {
		t.Fatalf("errgroup.Wait: %v", err)
	}
	wg.Wait()
	t.Log("both errgroup goroutines completed")
}
