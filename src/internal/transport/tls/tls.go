// @sk-task foundation#T1.3: internal stubs (AC-002)

package tls

import (
	"crypto/tls"
)

// @sk-task core-tunnel-mvp#T2.2: TLS 1.3 config (AC-003)
func NewServerTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

func NewClientTLSConfig(skipVerify bool) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: skipVerify,
	}
}
