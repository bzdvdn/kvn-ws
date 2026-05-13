package websocket

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// @sk-test core-tunnel-mvp#T5.2: TestWSIntegrationEcho (AC-002)
func TestWSDialAndEcho(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	var serverConn *websocket.Conn
	done := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		var err error
		serverConn, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server upgrade: %v", err)
			return
		}
		defer serverConn.Close()
		_, msg, err := serverConn.ReadMessage()
		if err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		if err := serverConn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
			t.Errorf("server write: %v", err)
		}
		close(done)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):] + "/echo"

	conn, err := Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	payload := []byte("hello ws echo")
	if err := conn.WriteMessage(payload); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for echo")
	}

	msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	if string(msg) != string(payload) {
		t.Errorf("echo = %s, want %s", msg, payload)
	}
}

// @sk-test core-tunnel-mvp#T5.2: TestWSTLSIntegration (AC-003)
func TestWSTLSIntegration(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		conn.Close()
	}))
	server.TLS = tlsConfig
	server.StartTLS()
	defer server.Close()

	wsURL := "wss" + server.URL[len("https"):] + "/tunnel"

	dialTLS := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	}

	conn, err := Dial(wsURL, dialTLS)
	if err != nil {
		t.Fatalf("Dial WSS: %v", err)
	}
	defer conn.Close()

	if conn.Underlying() == nil {
		t.Fatal("underlying conn is nil")
	}
}
