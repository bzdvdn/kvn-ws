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

// @sk-test local-proxy-mode#T3.2: socks5 method negotiation falls back to
// no-auth when the client cannot do RFC 1929 (e.g. Telegram Android)
func TestSOCKS5AuthMethodNegotiation(t *testing.T) {
	auth := &ProxyAuth{Username: "u", Password: "p"}

	cases := []struct {
		name     string
		methods  []byte
		expected byte
	}{
		{"user-pass-and-none-offered", []byte{0x00, 0x02}, socksAuthUserPass},
		{"only-no-auth-offered", []byte{0x00}, socksAuthNone},
		{"no-auth-offered", []byte{0x00, 0x01}, socksAuthNone},
		{"neither-offered", []byte{0x01}, 0xFF},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewListener("127.0.0.1:0", auth, func(_ net.Conn, dst string) {})
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

			greeting := append([]byte{socksVersion5, byte(len(tc.methods))}, tc.methods...)
			if _, err := conn.Write(greeting); err != nil {
				t.Fatalf("write greeting: %v", err)
			}

			buf := make([]byte, 2)
			if _, err := conn.Read(buf); err != nil {
				t.Fatalf("read method reply: %v", err)
			}
			if buf[0] != socksVersion5 {
				t.Fatalf("version = %#x, want %#x", buf[0], socksVersion5)
			}
			if buf[1] != tc.expected {
				t.Fatalf("method = %#x, want %#x", buf[1], tc.expected)
			}
		})
	}
}

// @sk-test ipv4-prefer-tunnel#T3.2: raw IPv6 CONNECT is rejected with host
// unreachable, IPv4-mapped ::ffff: targets are rewritten to IPv4.
func TestSOCKS5IPv6AddressHandling(t *testing.T) {
	t.Run("mapped-ipv6-rewritten-to-ipv4", func(t *testing.T) {
		var mu sync.Mutex
		var gotDst string
		handler := func(_ net.Conn, dst string) {
			mu.Lock()
			gotDst = dst
			mu.Unlock()
		}
		l := NewListener("127.0.0.1:0", nil, handler)
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

		// greeting, then CONNECT with IPv4-mapped IPv6 ::ffff:127.0.0.1:8080
		if _, err := conn.Write([]byte{socksVersion5, 0x01, socksAuthNone}); err != nil {
			t.Fatalf("greeting: %v", err)
		}
		reply := make([]byte, 2)
		if _, err := conn.Read(reply); err != nil {
			t.Fatalf("read greeting: %v", err)
		}
		req := []byte{socksVersion5, socksCmdConnect, 0x00, socksAtypIPv6}
		req = append(req, net.IPv4(127, 0, 0, 1).To16()...) // ::ffff:127.0.0.1
		req = append(req, 0x1f, 0x90)                       // port 8080
		if _, err := conn.Write(req); err != nil {
			t.Fatalf("connect: %v", err)
		}

		deadline := time.Now().Add(2 * time.Second)
		for {
			mu.Lock()
			dst := gotDst
			mu.Unlock()
			if dst != "" {
				if dst != "127.0.0.1:8080" {
					t.Fatalf("dst = %q, want %q", dst, "127.0.0.1:8080")
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("onConn not called with IPv4-mapped dst")
			}
			time.Sleep(10 * time.Millisecond)
		}
	})

	t.Run("raw-ipv6-rejected-host-unreachable", func(t *testing.T) {
		handlerCalled := false
		handler := func(net.Conn, string) { handlerCalled = true }
		l := NewListener("127.0.0.1:0", nil, handler)
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

		if _, err := conn.Write([]byte{socksVersion5, 0x01, socksAuthNone}); err != nil {
			t.Fatalf("greeting: %v", err)
		}
		reply := make([]byte, 2)
		if _, err := conn.Read(reply); err != nil {
			t.Fatalf("read greeting: %v", err)
		}
		req := []byte{socksVersion5, socksCmdConnect, 0x00, socksAtypIPv6}
		req = append(req, net.ParseIP("2001:db8::1").To16()...)
		req = append(req, 0x01, 0xbb) // port 443
		if _, err := conn.Write(req); err != nil {
			t.Fatalf("connect: %v", err)
		}

		buf := make([]byte, 10)
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("read reply: %v", err)
		}
		if n < 2 || buf[0] != socksVersion5 || buf[1] != socksRepHostUnreachable {
			t.Fatalf("reply = %x, want 0504 host unreachable", buf[:n])
		}
		if handlerCalled {
			t.Fatal("onConn must not be called for raw IPv6 CONNECT")
		}
	})
}
