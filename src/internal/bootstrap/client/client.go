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

func (c *Client) Run(ctx context.Context) error {
	defer c.logger.Info("client stopped")

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if c.cfg.Mode == "proxy" {
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
