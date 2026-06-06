package proxy

import (
	"net"
	"sync"
	"testing"
	"time"
)

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
