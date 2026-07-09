package client

import (
	"context"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/control"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
)

// @sk-task transport-factory#T2.2: dialStream uses TransportFactory (AC-004)
func dialStream(ctx context.Context, cfg *config.ClientConfig, logger *zap.Logger) (transport.StreamConn, error) {
	tlsCfg, err := clientTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	paddingEnabled := cfg.Obfuscation != nil && cfg.Obfuscation.Padding != nil && cfg.Obfuscation.Padding.Enabled
	paddingSize := 512
	if cfg.Obfuscation != nil && cfg.Obfuscation.Padding != nil && cfg.Obfuscation.Padding.Size > 0 {
		paddingSize = cfg.Obfuscation.Padding.Size
	}
	factoryCfg := &transport.FactoryConfig{
		TLS:               tlsCfg,
		Logger:            logger,
		MaxMessageSize:    cfg.MaxMessageSize,
		KeepaliveInterval: control.DefaultPingInterval,
		KeepaliveTimeout:  control.DefaultPongTimeout,
		Multiplex:         cfg.Multiplex,
		MTU:               cfg.MTU,
		UTLS:              cfg.Obfuscation != nil && cfg.Obfuscation.UTLS != nil && cfg.Obfuscation.UTLS.Enabled,
		UTLSFallback:      cfg.Obfuscation != nil && cfg.Obfuscation.UTLS != nil && cfg.Obfuscation.UTLS.Fallback,
		PaddingEnabled:    paddingEnabled,
		PaddingSize:       paddingSize,
		Obfuscation:       cfg.Obfuscation != nil && cfg.Obfuscation.Enabled,
	}

	factory := transport.NewFactory(cfg.Transport, factoryCfg)
	if cfg.Transport == "quic" {
		wsFactory := transport.NewFactory("ws", factoryCfg)
		factory = transport.NewFallbackFactory(factory, wsFactory, logger)
	}
	return factory.Dial(ctx, cfg.Server)
}
