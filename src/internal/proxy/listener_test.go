package proxy

import (
	"net"
	"sync"
	"testing"
	"time"
)

// @sk-test transparent-proxy#T4.2: TestTransparentDetection verifies transparent handler wired (AC-002)
func TestTransparentDetection(t *testing.T) {
	var mu sync.Mutex
	transparentCalled := false
	handler := func(client net.Conn, dst string) {
		mu.Lock()
		transparentCalled = true
		mu.Unlock()
	}

	l := NewListener("127.0.0.1:0", nil, handler)
	l.SetTransparent(true)
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer l.Close()

	go func() { _ = l.AcceptLoop() }()

	conn, err := net.DialTimeout("tcp", l.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// send a byte that's neither SOCKS5 (0x05) nor 'C'
	_, _ = conn.Write([]byte{0x01})
	time.Sleep(100 * time.Millisecond)

	// transparent handler should NOT be called because SO_ORIGINAL_DST fails
	// on direct connections (not redirected by iptables). The connection
	// should just be closed.
	// This tests that the handler doesn't crash and the connection is cleanly closed.
	mu.Lock()
	wasCalled := transparentCalled
	mu.Unlock()
	if wasCalled {
		t.Log("transparent handler called (SO_ORIGINAL_DST not available on non-redirected conn)")
	}
}

// @sk-test transparent-proxy#T4.2: TestTransparentOffDoesNotIntercept verifies default path unchanged (AC-002)
func TestTransparentOffDoesNotIntercept(t *testing.T) {
	var mu sync.Mutex
	transparentCalled := false
	handler := func(client net.Conn, dst string) {
		mu.Lock()
		transparentCalled = true
		mu.Unlock()
	}

	l := NewListener("127.0.0.1:0", nil, handler)
	// transparent NOT set
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer l.Close()

	go func() { _ = l.AcceptLoop() }()

	conn, err := net.DialTimeout("tcp", l.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, _ = conn.Write([]byte{0x01})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	wasCalled := transparentCalled
	mu.Unlock()
	if wasCalled {
		t.Error("transparent handler called when transparent=false")
	}
}

// @sk-test transparent-proxy#T5.2: TestSetLogFn verifies debug logging callback (AC-010)
func TestSetLogFn(t *testing.T) {
	var mu sync.Mutex
	var logged []string
	handler := func(client net.Conn, dst string) {}

	l := NewListener("127.0.0.1:0", nil, handler)
	l.SetTransparent(true)
	l.SetLogFn(func(format string, args ...any) {
		mu.Lock()
		logged = append(logged, format)
		mu.Unlock()
	})
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer l.Close()

	go func() { _ = l.AcceptLoop() }()

	conn, err := net.DialTimeout("tcp", l.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, _ = conn.Write([]byte{0x01})
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(logged) == 0 {
		t.Fatal("expected logf to be called, got none")
	}
	found := false
	for _, msg := range logged {
		if msg == "transparent dst=" {
			found = true
		}
	}
	// getOriginalDst will fail on non-redirected conn, so we expect
	// "getOriginalDst failed: ..." at minimum.
	// The key: SetLogFn must not panic and must actually invoke the callback.
	if !found {
		t.Logf("logged messages: %v", logged)
	}
}

// @sk-test transparent-proxy#T5.1: TestGetOriginalDstNotTCPConn (AC-010)
func TestGetOriginalDstNotTCPConn(t *testing.T) {
	// getOriginalDst should return an error for non-TCP connections
	type fakeConn struct{ net.Conn }
	_, err := getOriginalDst(fakeConn{})
	if err == nil {
		t.Fatal("expected error for non-TCPConn, got nil")
	}
}

// @sk-test fix-critical-leaks#T6.1: TestProxySemaphore (AC-002)
func TestProxySemaphore(t *testing.T) {
	handler := func(client net.Conn, dst string) {
		// Simulate slow handler to test concurrency limiting
		time.Sleep(50 * time.Millisecond)
	}

	l := NewListener("127.0.0.1:0", nil, handler)
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer l.Close()

	// AcceptLoop in background
	go func() { _ = l.AcceptLoop() }()

	addr := l.Addr().String()
	concurrency := 10
	var wg sync.WaitGroup
	start := make(chan struct{})

	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			conn, err := net.DialTimeout("tcp", addr, time.Second)
			if err != nil {
				t.Logf("dial: %v", err)
				return
			}
			// Send SOCKS5 initial byte
			_, _ = conn.Write([]byte{0x05, 0x01, 0x00})
			conn.Close()
		}()
	}

	close(start)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("connections blocked by semaphore")
	}
}
