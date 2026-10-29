package transport

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/tunpool"
)

func echoTarget(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

// @sk-test kvn-v2-transport#T5.1: HTTP/2 stream reaches a TCP target and echoes (AC-001)
func TestStreamRoundTrip(t *testing.T) {
	target, closeTarget := echoTarget(t)
	defer closeTarget()

	srv := NewServer(func(token string) (string, bool) {
		return "test", token == "secret"
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }), 64, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tlsCfg := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}
	stream, err := Dial(ctx, ts.URL+Path, "secret", target, tlsCfg)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = stream.Close() }()

	payload := []byte("hello-v2")
	if _, err := stream.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(stream, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(buf, payload) {
		t.Fatalf("echo = %q, want %q", buf, payload)
	}
}

// @sk-test v2-standard#T4.7: ICMP stream accepts application/icmp response (AC-007)
func TestDialICMPStream(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/icmp")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		body, _ := io.ReadAll(r.Body)
		reply := []byte{0, 0, 0, 0, 0, 1, 0, 1}
		if len(body) >= 8 {
			reply = append(reply, body[8:]...)
		}
		_, _ = w.Write(reply)
	})
	ts := httptest.NewUnstartedServer(mux)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tlsCfg := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}
	opts := ClientOptions{TLS: tlsCfg}
	client, err := NewClientWithOptions(ts.URL, "secret", &opts)
	if err != nil {
		t.Fatalf("NewClientWithOptions: %v", err)
	}
	body, err := client.DialICMP(ctx, "127.0.0.1", []byte{8, 0, 0, 0, 0, 1, 0, 1})
	if err != nil {
		t.Fatalf("DialICMP: %v", err)
	}
	defer func() { _ = body.Close() }()
	reply, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(reply) < 8 || reply[0] != 0 {
		t.Fatalf("bad ICMP reply: %x", reply)
	}
}

// @sk-test kvn-v2-transport#T5.1: invalid Bearer gets fallback, not a tunnel (AC-003)
func TestFallbackWithoutBearer(t *testing.T) {
	srv := NewServer(func(token string) (string, bool) { return "", false },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html>site</html>"))
		}), 8, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := Dial(ctx, ts.URL+Path, "bad", "127.0.0.1:1", &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}})
	if err == nil {
		t.Fatal("expected status error for fallback")
	}
}

// @sk-test kvn-v2-transport#T5.1: best-effort UDP flow roundtrip over HTTP/2 (AC-008)
func TestUDPStreamRoundTrip(t *testing.T) {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, rerr := udpConn.ReadFromUDP(buf)
			if rerr != nil {
				return
			}
			_, _ = udpConn.WriteToUDP(buf[:n], addr)
		}
	}()

	srv := NewServer(func(token string) (string, bool) { return "t", token == "secret" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }), 64, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := DialUDP(ctx, ts.URL+UDPPath, "secret", udpConn.LocalAddr().String(),
		&tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}})
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer func() { _ = stream.Close() }()
	payload := []byte("udp-v2")
	if err := stream.WritePacket(payload); err != nil {
		t.Fatalf("write packet: %v", err)
	}
	got, err := stream.ReadPacket()
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("udp echo = %q, want %q", got, payload)
	}
}

// @sk-test kvn-v2-transport#T5.1: slow TCP flow does not block another flow (AC-002)
func TestSlowFlowDoesNotBlockAnother(t *testing.T) {
	slowLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer slowLn.Close()
	go func() {
		for {
			c, err := slowLn.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				time.Sleep(5 * time.Second)
				_ = conn.Close()
			}(c) // hold the connection open without reading
		}
	}()
	fastTarget, closeFast := echoTarget(t)
	defer closeFast()

	srv := NewServer(func(token string) (string, bool) { return "t", token == "secret" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }), 64, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	tlsCfg := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}
	client, err := NewClient(ts.URL+Path, "secret", tlsCfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	slow, err := client.DialTCP(ctx, slowLn.Addr().String())
	if err != nil {
		t.Fatalf("slow dial: %v", err)
	}
	defer func() { _ = slow.Close() }()
	if _, err := slow.Write([]byte("hold")); err != nil {
		t.Fatalf("slow write: %v", err)
	}

	fast, err := client.DialTCP(ctx, fastTarget)
	if err != nil {
		t.Fatalf("fast dial while slow flow open: %v", err)
	}
	defer func() { _ = fast.Close() }()
	payload := []byte("fast")
	if _, err := fast.Write(payload); err != nil {
		t.Fatalf("fast write: %v", err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(fast, got); err != nil {
		t.Fatalf("fast read: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("fast echo = %q", got)
	}
}

// @sk-test kvn-v2-transport#T4.3: UDP flow roundtrips over one QUIC datagram connection (AC-008)
func TestQUICDatagramUDPFlow(t *testing.T) {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, rerr := udpConn.ReadFromUDP(buf)
			if rerr != nil {
				return
			}
			_, _ = udpConn.WriteToUDP(buf[:n], addr)
		}
	}()

	cert := selfSignedCert(t)
	qs, err := ListenQUIC("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h3"}},
		func(token string) (string, bool) { return "t", token == "secret" }, zap.NewNop())
	if err != nil {
		t.Fatalf("ListenQUIC: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = qs.Serve(ctx) }()

	client, err := NewQUICClient(ctx, qs.Addr().String(), "secret", &tls.Config{InsecureSkipVerify: true}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewQUICClient: %v", err)
	}
	defer func() { _ = client.Close() }()
	flow, err := client.DialFlow(ctx, udpConn.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = flow.Close() }()
	payload := []byte("quic-udp")
	if err := flow.WritePacket(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := flow.ReadPacket()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("quic echo = %q", got)
	}
}

func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// @sk-test v2-standard#T2.3: padded TCP stream roundtrip on the browser-like path (AC-011)
func TestPaddedTCPStreamRoundTrip(t *testing.T) {
	target, closeTarget := echoTarget(t)
	defer closeTarget()

	srv := NewServer(func(token string) (string, bool) { return "test", token == "secret" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }), 64, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	tlsCfg := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}
	opts := DefaultClientOptions(tlsCfg)
	opts.UTLSProfile = ""
	opts.Padding = true
	opts.PaddingSize = 64
	client, err := NewClientWithOptions(ts.URL, "secret", &opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.DialTCP(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	payload := []byte("padded-hello")
	if _, err := stream.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(stream, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("padded echo = %q, want %q", got, payload)
	}
}

// @sk-test v2-standard#T2.3: padded UDP flow keeps packet boundaries and padding (AC-011)
func TestPaddedUDPStreamRoundTrip(t *testing.T) {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, rerr := udpConn.ReadFromUDP(buf)
			if rerr != nil {
				return
			}
			_, _ = udpConn.WriteToUDP(buf[:n], addr)
		}
	}()

	srv := NewServer(func(token string) (string, bool) { return "test", token == "secret" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }), 64, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	tlsCfg := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}
	opts := DefaultClientOptions(tlsCfg)
	opts.UTLSProfile = ""
	opts.Padding = true
	opts.PaddingSize = 64
	client, err := NewClientWithOptions(ts.URL, "secret", &opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.DialUDP(ctx, udpConn.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	payload := []byte("padded-udp")
	if err := stream.WritePacket(payload); err != nil {
		t.Fatal(err)
	}
	got, err := stream.ReadPacket()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("padded udp = %q, want %q", got, payload)
	}
}

// @sk-test v2-standard#T2.4: unknown paths return the fallback site, not a tunnel (AC-011)
func TestUnknownPathFallback(t *testing.T) {
	srv := NewServer(func(token string) (string, bool) { return "t", token == "secret" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html>site</html>"))
		}), 8, zap.NewNop())
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	client := ts.Client()
	resp, err := client.Get(ts.URL + "/definitely-not-a-tunnel")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "<html>site</html>" {
		t.Fatalf("fallback status=%d body=%q", resp.StatusCode, body)
	}
	if resp.Header.Get("X-KVN-Target") != "" {
		t.Fatal("fallback leaked KVN header")
	}
}

// @sk-test v2-tun-assignment#T1.3: authenticated session returns a pooled TUN lease (AC-002)
func TestSessionEndpoint(t *testing.T) {
	pool, err := tunpool.New(&tunpool.Config{
		Subnet: "10.66.0.0/24", RangeStart: "10.66.0.2", RangeEnd: "10.66.0.3",
		Gateway: "10.66.0.1", LeaseTTLSec: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(func(token string) (string, bool) { return "t", token == "secret" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html>site</html>"))
		}), 8, zap.NewNop(), WithTUNPool(pool))
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", http.NoBody)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("X-Request-ID", "client-a")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("session status=%d type=%q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	var session TUNSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.TUNAddress != "10.66.0.2/24" || session.TUNGateway != "10.66.0.1" || session.LeaseTTLSec != 60 {
		t.Fatalf("session = %+v", session)
	}
}

// @sk-test v2-tun-assignment#T1.3: unauthorized session request gets fallback (AC-009)
func TestSessionUnauthorizedFallback(t *testing.T) {
	pool, _ := tunpool.New(&tunpool.Config{
		Subnet: "10.66.0.0/24", RangeStart: "10.66.0.2", RangeEnd: "10.66.0.3",
		Gateway: "10.66.0.1", LeaseTTLSec: 60,
	})
	srv := NewServer(func(token string) (string, bool) { return "t", token == "secret" },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html>site</html>"))
		}), 8, zap.NewNop(), WithTUNPool(pool))
	ts := httptest.NewUnstartedServer(srv)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", http.NoBody)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", "client-a")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "<html>site</html>" {
		t.Fatalf("fallback status=%d body=%q", resp.StatusCode, body)
	}
}
