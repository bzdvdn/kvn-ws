package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	stdtls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePEMFile(t *testing.T, dir, name, blockType string, der []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	block := &pem.Block{Type: blockType, Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func writeServerCertFiles(t *testing.T, dir string) (certFile, keyFile, caFile string) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(CA): %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "kvn-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(CA): %v", err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(server): %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(server): %v", err)
	}

	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey(server): %v", err)
	}

	return writePEMFile(t, dir, "server.pem", "CERTIFICATE", serverDER),
		writePEMFile(t, dir, "server-key.pem", "EC PRIVATE KEY", serverKeyDER),
		writePEMFile(t, dir, "ca.pem", "CERTIFICATE", caDER)
}

// @sk-test production-gap#T1.1: TestNewClientTLSConfigFromSettingsDefaultsToVerify (AC-001)
func TestNewClientTLSConfigFromSettingsDefaultsToVerify(t *testing.T) {
	cfg, err := NewClientTLSConfigFromSettings(ClientTLSSettings{})
	if err != nil {
		t.Fatalf("NewClientTLSConfigFromSettings: %v", err)
	}
	if cfg.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify = true, want false")
	}
	if cfg.MinVersion != stdtls.VersionTLS13 {
		t.Fatalf("MinVersion = %d, want TLS 1.3", cfg.MinVersion)
	}
}

// @sk-test production-gap#T1.1: TestNewClientTLSConfigFromSettingsLoadsCAAndServerName (AC-001)
func TestNewClientTLSConfigFromSettingsLoadsCAAndServerName(t *testing.T) {
	dir := t.TempDir()
	_, _, caFile := writeServerCertFiles(t, dir)

	cfg, err := NewClientTLSConfigFromSettings(ClientTLSSettings{
		CAFile:     caFile,
		ServerName: "localhost",
		VerifyMode: "verify",
	})
	if err != nil {
		t.Fatalf("NewClientTLSConfigFromSettings: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Fatal("RootCAs = nil, want loaded CA pool")
	}
	if cfg.ServerName != "localhost" {
		t.Fatalf("ServerName = %q, want localhost", cfg.ServerName)
	}
}

// @sk-test production-gap#T1.2: TestNewServerTLSConfigRejectsRequireWithoutCA (AC-002)
func TestNewServerTLSConfigRejectsRequireWithoutCA(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile, _ := writeServerCertFiles(t, dir)

	_, err := NewServerTLSConfig(certFile, keyFile, "", "require")
	if err == nil {
		t.Fatal("NewServerTLSConfig require without CA = nil error, want error")
	}
}

// @sk-test production-gap#T1.2: TestNewServerTLSConfigMapsClientAuthModes (AC-002)
func TestNewServerTLSConfigMapsClientAuthModes(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile, caFile := writeServerCertFiles(t, dir)

	tests := []struct {
		name       string
		clientAuth string
		want       stdtls.ClientAuthType
	}{
		{name: "request", clientAuth: "request", want: stdtls.RequestClientCert},
		{name: "require", clientAuth: "require", want: stdtls.RequireAndVerifyClientCert},
		{name: "verify", clientAuth: "verify", want: stdtls.VerifyClientCertIfGiven},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewServerTLSConfig(certFile, keyFile, caFile, tt.clientAuth)
			if err != nil {
				t.Fatalf("NewServerTLSConfig(%s): %v", tt.clientAuth, err)
			}
			if cfg.ClientAuth != tt.want {
				t.Fatalf("ClientAuth = %d, want %d", cfg.ClientAuth, tt.want)
			}
		})
	}
}
