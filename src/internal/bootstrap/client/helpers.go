package client

import (
	"crypto/tls"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	tlstp "github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
)

// @sk-task fix-critical-leaks#T4.2: shared TLS config helper (AC-012)
func clientTLSConfig(cfg *config.ClientConfig) (*tls.Config, error) {
	tlsCfg, err := tlstp.NewClientTLSConfigFromSettings(tlstp.ClientTLSSettings{
		CAFile:     cfg.TLS.CAFile,
		ServerName: cfg.TLS.ServerName,
		VerifyMode: cfg.TLS.VerifyMode,
	})
	if err != nil {
		return nil, err
	}
	if sni := tlstp.SelectSNI(cfg.TLS.SNI); sni != "" {
		tlsCfg.ServerName = sni
	}
	return tlsCfg, nil
}

// @sk-task fix-critical-leaks#T4.2: shared backoff parsing helper (AC-012)
func parseBackoff(cfg *config.ReconnectCfg) (min, max time.Duration) {
	min = 1 * time.Second
	max = 30 * time.Second
	if cfg == nil {
		return
	}
	if cfg.MinBackoffSec > 0 {
		min = time.Duration(cfg.MinBackoffSec) * time.Second
	}
	if cfg.MaxBackoffSec > 0 {
		max = time.Duration(cfg.MaxBackoffSec) * time.Second
	}
	return
}
