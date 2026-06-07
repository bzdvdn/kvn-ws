package client

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/systemproxy"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

type Client struct {
	cfg        *config.ClientConfig
	logger     *zap.Logger
	masterKey  []byte
	tunDev     tun.TunDevice
}

func (c *Client) SetLogger(l *zap.Logger) {
	c.logger = l
}

// @sk-task kvn-web#T2.2: NewFromConfig creates Client from existing config (AC-003)
func NewFromConfig(cfg *config.ClientConfig) (*Client, error) {
	logger, _, err := logger.New(cfg.Log.Level) //nolint:forbidigo
	if err != nil {
		return nil, err
	}

	logger.Info("starting client", zap.String("server", cfg.Server))

	var masterKey []byte
	if cfg.Crypto.Enabled {
		masterKey, err = crypto.ParseMasterKey(cfg.Crypto.Key)
		if err != nil {
			return nil, err
		}
		logger.Info("app-layer encryption enabled")
	} else {
		logger.Info("app-layer encryption disabled")
	}

	return &Client{
		cfg:       cfg,
		logger:    logger,
		masterKey: masterKey,
	}, nil
}

func New() (*Client, error) {
	cfgPath := pflag.String("config", "configs/client.yaml", "path to config file")
	pflag.Parse()

	cfg, err := config.LoadClientConfig(*cfgPath)
	if err != nil {
		return nil, err
	}

	logger, _, err := logger.New(cfg.Log.Level) //nolint:forbidigo
	if err != nil {
		return nil, err
	}

	logger.Info("starting client", zap.String("server", cfg.Server))

	var masterKey []byte
	if cfg.Crypto.Enabled {
		masterKey, err = crypto.ParseMasterKey(cfg.Crypto.Key)
		if err != nil {
			return nil, err
		}
		logger.Info("app-layer encryption enabled")
	} else {
		logger.Info("app-layer encryption disabled")
	}

	return &Client{
		cfg:       cfg,
		logger:    logger,
		masterKey: masterKey,
	}, nil
}

// @sk-task whitelist-obfuscation#T3.2: padding size default helper (AC-005)
func paddingSizeOrDefault(oc *config.ObfuscationCfg) int {
	if oc != nil && oc.Padding != nil && oc.Padding.Size > 0 {
		return oc.Padding.Size
	}
	return 512
}

// @sk-task system-proxy#T2.1: integrate system proxy with client lifecycle (AC-001, AC-002, AC-003, AC-007)
func (c *Client) Run(ctx context.Context) error {
	defer c.logger.Info("client stopped")

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if c.cfg.Mode == "proxy" {
		if c.cfg.SystemProxy != nil && *c.cfg.SystemProxy {
			sysProxy := systemproxy.New(c.cfg)
			addr := c.cfg.ProxyListen
			if addr == "" {
				addr = "127.0.0.1:2310"
			}
			noProxy := systemproxy.NOProxyBuilder(c.cfg.Routing)
			if err := sysProxy.Set(ctx, c.logger, addr, noProxy); err != nil {
				c.logger.Warn("system proxy set failed", zap.Error(err))
			} else {
				defer func() {
					_ = sysProxy.Restore(ctx, c.logger)
				}()
			}
		}
		c.runProxyMode(ctx)
		return nil
	}

	tunDev := tun.NewTunDevice()
	if err := tunDev.Open(); err != nil {
		log.Fatalf("open tun: %v", err)
	}
	defer func() { _ = tunDev.Close() }()

	c.reconnectLoop(ctx, tunDev)
	return nil
}
