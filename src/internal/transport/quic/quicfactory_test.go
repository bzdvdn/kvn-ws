package quic

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/transport"
)

// @sk-test transport-factory#T3.1: TestQUICFactoryDial verifies QUICFactory.Dial returns a working StreamConn (AC-003)
func TestQUICFactoryDial(t *testing.T) {
	addr := startQUICEchoServer(t)
	logger := zap.NewNop()

	factoryCfg := &transport.FactoryConfig{
		TLS: &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"kvn-ws"},
		},
		Logger:      logger,
		Obfuscation: false,
	}
	factory := NewQUICFactory(factoryCfg)
	ctx := context.Background()

	conn, err := factory.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("QUICFactory.Dial failed: %v", err)
	}
	defer conn.Close()

	payload := []byte("hello quic factory")
	if err := conn.WriteMessage(payload); err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}
	resp, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if !bytes.Equal(resp, payload) {
		t.Fatalf("got %q, want %q", resp, payload)
	}
}

func startQUICEchoServer(t *testing.T) string {
	t.Helper()
	cert := generateTestCert(t)
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"kvn-ws"},
	}
	listener, err := quic.ListenAddr("127.0.0.1:0", tlsCfg, nil)
	if err != nil {
		t.Fatalf("quic.ListenAddr failed: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept(context.Background())
			if err != nil {
				return
			}
			go func() {
				stream, err := conn.AcceptStream(context.Background())
				if err != nil {
					return
				}
				var lenBuf [4]byte
				if _, err := stream.Read(lenBuf[:]); err != nil {
					return
				}
				msgLen := int(binary.BigEndian.Uint32(lenBuf[:]))
				msg := make([]byte, msgLen)
				if _, err := stream.Read(msg); err != nil {
					return
				}
				var out [4]byte
				binary.BigEndian.PutUint32(out[:], uint32(msgLen))
				stream.Write(out[:])
				stream.Write(msg)
				stream.Close()
			}()
		}
	}()

	return listener.Addr().String()
}

func generateTestCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  key,
	}
}
