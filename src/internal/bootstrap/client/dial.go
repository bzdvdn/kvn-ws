package client

import (
	"context"
	"fmt"
	"net/url"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/control"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
)

// @sk-task transport-factory#T2.2: dialStream uses TransportFactory (AC-004)
func dialStream(ctx context.Context, cfg *config.ClientConfig, logger *zap.Logger) (transport.StreamConn, error) {
	return dialStreamWith(ctx, cfg, logger, false)
}

// @sk-task game-latency#T3.3: request unpadded framing for the secondary channel (AC-001)
func realtimeUnpadded(cfg *config.ClientConfig) bool {
	return cfg.Obfuscation != nil && cfg.Obfuscation.Padding != nil &&
		cfg.Obfuscation.Padding.Enabled && cfg.Obfuscation.Padding.RealtimeUnpaddedEnabled()
}

// @sk-task fallback-auth-stealth#T2.2: try stealth path with Bearer, then fall back to legacy (AC-001, AC-006)
func dialStreamWith(ctx context.Context, cfg *config.ClientConfig, logger *zap.Logger, requestNoPad bool) (transport.StreamConn, error) {
	if cfg.Stealth != nil && cfg.Stealth.Enabled && cfg.Stealth.Path != "" && cfg.Transport != "quic" {
		stealthURL, serr := replaceURLPath(cfg.Server, cfg.Stealth.Path)
		if serr != nil {
			return nil, fmt.Errorf("stealth url: %w", serr)
		}
		conn, derr := dialStreamURL(ctx, cfg, logger, requestNoPad, stealthURL, "Bearer "+cfg.Auth.Token)
		if derr == nil {
			return conn, nil
		}
		legacyURL := cfg.Server
		if cfg.Stealth.LegacyPath != "" {
			if u, lerr := replaceURLPath(cfg.Server, cfg.Stealth.LegacyPath); lerr == nil {
				legacyURL = u
			}
		}
		logger.Warn("stealth path unavailable, falling back to legacy",
			zap.String("stealth_path", cfg.Stealth.Path),
			zap.String("legacy_url", legacyURL),
			zap.Error(derr),
		)
		return dialStreamURL(ctx, cfg, logger, requestNoPad, legacyURL, "")
	}
	return dialStreamURL(ctx, cfg, logger, requestNoPad, cfg.Server, "")
}

// @sk-task fallback-auth-stealth#T2.2: build a URL with the same host/scheme and a new path (AC-006)
func replaceURLPath(rawURL, path string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("server URL has no host")
	}
	u.Path = path
	u.RawQuery = ""
	return u.String(), nil
}

func dialStreamURL(ctx context.Context, cfg *config.ClientConfig, logger *zap.Logger, requestNoPad bool, endpoint, authorization string) (transport.StreamConn, error) {
	tlsCfg, err := clientTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	paddingEnabled := cfg.Obfuscation != nil && cfg.Obfuscation.Padding != nil && cfg.Obfuscation.Padding.Enabled
	paddingSize := 512
	paddingMode := "fixed"
	noPadHeaders := []string{"X-KVN-NoPad"}
	if cfg.Obfuscation != nil && cfg.Obfuscation.Padding != nil {
		if cfg.Obfuscation.Padding.Size > 0 {
			paddingSize = cfg.Obfuscation.Padding.Size
		}
		paddingMode = cfg.Obfuscation.Padding.ModeOrDefault()
		noPadHeaders = cfg.Obfuscation.Padding.NoPadHeadersOrDefault()
	}
	factoryCfg := &transport.FactoryConfig{
		TLS:                   tlsCfg,
		Logger:                logger,
		MaxMessageSize:        cfg.MaxMessageSize,
		KeepaliveInterval:     control.DefaultPingInterval,
		KeepaliveTimeout:      control.DefaultPongTimeout,
		Multiplex:             cfg.Multiplex,
		MTU:                   cfg.MTU,
		UTLS:                  cfg.Obfuscation != nil && cfg.Obfuscation.UTLS != nil && cfg.Obfuscation.UTLS.Enabled,
		UTLSFallback:          cfg.Obfuscation != nil && cfg.Obfuscation.UTLS != nil && cfg.Obfuscation.UTLS.Fallback,
		PaddingEnabled:        paddingEnabled,
		PaddingSize:           paddingSize,
		PaddingMode:           paddingMode,
		NoPadHeaders:          noPadHeaders,
		MultiplexSubprotocols: cfg.MultiplexSubprotocolsOrDefault(),
		HeadersProfile:        cfg.HeadersProfileOrDefault(),
		KeepaliveJitterPct:    cfg.KeepaliveJitterPct,
		RequestNoPad:          requestNoPad,
		Authorization:         authorization,
		Obfuscation:           cfg.Obfuscation != nil && cfg.Obfuscation.Enabled,
	}

	factory := transport.NewFactory(cfg.Transport, factoryCfg)
	if cfg.Transport == "quic" {
		wsFactory := transport.NewFactory("ws", factoryCfg)
		factory = transport.NewFallbackFactory(factory, wsFactory, logger)
	}
	return factory.Dial(ctx, endpoint)
}
