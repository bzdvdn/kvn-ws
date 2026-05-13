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
func NewServerTLSConfig(certFile, keyFile, clientCAFile, clientAuth string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}
	if clientCAFile != "" {
		caCert, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, fmt.Errorf("read client CA %s: %w", clientCAFile, err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("parse client CA %s: no certificates found", clientCAFile)
		}
		cfg.ClientCAs = caPool
		switch clientAuth {
		case "require":
			cfg.ClientAuth = tls.RequireAnyClientCert
		case "verify":
			cfg.ClientAuth = tls.RequireAndVerifyClientCert
		default:
			cfg.ClientAuth = tls.RequireAndVerifyClientCert
		}
	}
	return cfg, nil
}

func NewClientTLSConfig(skipVerify bool) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: skipVerify,
	}
}
