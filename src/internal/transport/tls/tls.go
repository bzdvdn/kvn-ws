// @sk-task foundation#T1.3: internal stubs (AC-002)
// @sk-task security-acl#T11: mTLS support

package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// @sk-task core-tunnel-mvp#T2.2: TLS 1.3 config (AC-003)
// @sk-task security-acl#T11: mTLS with optional client CA
// @sk-task production-gap#T1.2: explicit request/require/verify semantics (AC-002)
// @sk-task production-gap#T2.1: trust-enforcing server TLS behavior (AC-002)
func NewServerTLSConfig(certFile, keyFile, clientCAFile, clientAuth string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return newServerTLSConfig(cert, clientCAFile, nil, clientAuth)
}

// @sk-task production-gap#T2.1: reusable server TLS builder for trust-enforcing tests (AC-002)
func NewServerTLSConfigFromMaterial(cert tls.Certificate, clientCAPEM []byte, clientAuth string) (*tls.Config, error) {
	return newServerTLSConfig(cert, "", clientCAPEM, clientAuth)
}

func newServerTLSConfig(cert tls.Certificate, clientCAFile string, clientCAPEM []byte, clientAuth string) (*tls.Config, error) {
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}
	switch {
	case clientCAFile != "":
		caCert, err := os.ReadFile(clientCAFile) // #nosec G304 — path from trusted config
		if err != nil {
			return nil, fmt.Errorf("read client CA %s: %w", clientCAFile, err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("parse client CA %s: no certificates found", clientCAFile)
		}
		cfg.ClientCAs = caPool
	case len(clientCAPEM) > 0:
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(clientCAPEM) {
			return nil, fmt.Errorf("parse client CA material: no certificates found")
		}
		cfg.ClientCAs = caPool
	}

	switch clientAuth {
	case "":
		cfg.ClientAuth = tls.NoClientCert
	case "request":
		cfg.ClientAuth = tls.RequestClientCert
	case "require":
		if cfg.ClientCAs == nil {
			return nil, fmt.Errorf("client_ca_file is required for client_auth=require")
		}
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	case "verify":
		if cfg.ClientCAs == nil {
			return nil, fmt.Errorf("client_ca_file is required for client_auth=verify")
		}
		cfg.ClientAuth = tls.VerifyClientCertIfGiven
	default:
		return nil, fmt.Errorf("unsupported client_auth %q", clientAuth)
	}
	return cfg, nil
}

// @sk-task production-gap#T1.1: trusted client TLS builder (AC-001)
type ClientTLSSettings struct {
	CAFile     string
	ServerName string
	VerifyMode string
}

func NewClientTLSConfig(skipVerify bool) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: skipVerify, // #nosec G402 — user configurable via verify_mode
	}
}

// @sk-task production-gap#T1.1: explicit client TLS trust semantics (AC-001)
// @sk-task production-gap#T2.1: trust-enforcing client TLS behavior (AC-001)
func NewClientTLSConfigFromSettings(settings ClientTLSSettings) (*tls.Config, error) {
	verifyMode := settings.VerifyMode
	if verifyMode == "" {
		verifyMode = "verify"
	}

	cfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
		ServerName: settings.ServerName,
	}

	switch verifyMode {
	case "verify":
	case "insecure":
		cfg.InsecureSkipVerify = true
	default:
		return nil, fmt.Errorf("unsupported verify_mode %q", verifyMode)
	}

	if settings.CAFile != "" {
		caCert, err := os.ReadFile(settings.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file %s: %w", settings.CAFile, err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("parse CA file %s: no certificates found", settings.CAFile)
		}
		cfg.RootCAs = caPool
	}

	return cfg, nil
}
