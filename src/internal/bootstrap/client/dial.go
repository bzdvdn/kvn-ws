package client

import (
	"context"
	"net/url"
	"time"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/control"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	quictp "github.com/bzdvdn/kvn-ws/src/internal/transport/quic"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
)

// @sk-task arch-refactoring#T3.1: common dialStream for tun and proxy modes (AC-004)
// @sk-task arch-refactoring#T3.1: single dialStream for QUIC and WebSocket (AC-004)
func dialStream(ctx context.Context, cfg *config.ClientConfig, logger *zap.Logger) (transport.StreamConn, error) {
	tlsCfg, err := clientTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	transportType := cfg.Transport
	if transportType == "" {
		transportType = "tcp"
	}

	if transportType == "quic" {
		quicAddr := cfg.Server
		if u, parseErr := url.Parse(quicAddr); parseErr == nil && u.Host != "" {
			quicAddr = u.Host
		}
		quicCfg := &quic.Config{
			KeepAlivePeriod: 7 * time.Second,
		}
		quicConn, err := quictp.Dial(ctx, quicAddr, tlsCfg, quicCfg)
		if err != nil {
			logger.Warn("QUIC dial failed, falling back to TCP", zap.Error(err))
		} else {
			if cfg.MaxMessageSize > 0 {
				quicConn.SetMaxMessageSize(cfg.MaxMessageSize)
			}
			if cfg.Obfuscation != nil && cfg.Obfuscation.Enabled {
				logger.Info("QUIC obfuscation enabled")
				obfConn, obfErr := quictp.NewObfuscatedQUICConn(quicConn)
				if obfErr != nil {
					logger.Warn("QUIC obfuscation init failed, falling back to TCP", zap.Error(obfErr))
				} else {
					return obfConn, nil
				}
			} else {
				return quicConn, nil
			}
		}
	}

	paddingEnabled := cfg.Obfuscation != nil && cfg.Obfuscation.Padding != nil && cfg.Obfuscation.Padding.Enabled
	wsCfg := websocket.WSConfig{
		Multiplex:      cfg.Multiplex,
		MTU:            cfg.MTU,
		UTLS:           cfg.Obfuscation != nil && cfg.Obfuscation.UTLS != nil && cfg.Obfuscation.UTLS.Enabled,
		UTLSFallback:   cfg.Obfuscation != nil && cfg.Obfuscation.UTLS != nil && cfg.Obfuscation.UTLS.Fallback,
		PaddingEnabled: paddingEnabled,
		PaddingSize:    paddingSizeOrDefault(cfg.Obfuscation),
	}
	wsConn, err := websocket.Dial(cfg.Server, tlsCfg, logger, wsCfg)
	if err != nil {
		return nil, err
	}
	wsConn.SetKeepalive(control.DefaultPingInterval, control.DefaultPongTimeout)
	return wsConn, nil
}
