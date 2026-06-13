package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	"github.com/bzdvdn/kvn-ws/src/internal/dnsproxy"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/systemproxy"
	"github.com/bzdvdn/kvn-ws/src/internal/transparent"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

type Client struct {
	cfg       *config.ClientConfig
	logger    *zap.Logger
	masterKey []byte

	dnsSrv *dnsproxy.Server
}

func (c *Client) SetLogger(l *zap.Logger) {
	c.logger = l
}

// @sk-task kvn-web#T2.2: NewFromConfig creates Client from existing config (AC-003)
func NewFromConfig(cfg *config.ClientConfig) (*Client, error) {
	logger, _, err := logger.New(cfg.Log.Level) //nolint:forbidigo // allow logging before zap init
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

	logger, _, err := logger.New(cfg.Log.Level) //nolint:forbidigo // allow logging before zap init
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

// @sk-task transparent-proxy#T3.1: parse port from listen addr (AC-001)
func portFromAddr(addr string) int {
	if addr == "" {
		return 2310
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 2310
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 2310
	}
	return port
}

// @sk-task whitelist-obfuscation#T3.2: padding size default helper (AC-005)
func paddingSizeOrDefault(oc *config.ObfuscationCfg) int {
	if oc != nil && oc.Padding != nil && oc.Padding.Size > 0 {
		return oc.Padding.Size
	}
	return 512
}

// @sk-task system-proxy#T2.1: integrate system proxy with client lifecycle (AC-001, AC-002, AC-003, AC-007)
// @sk-task transparent-proxy#T3.1: integrate transparent proxy + DNS proxy (AC-001, AC-005, AC-008, AC-009)
// @sk-task client-relay-mode#T2.1: relay mode branch (AC-003)
func (c *Client) Run(ctx context.Context) error {
	defer c.logger.Info("client stopped")

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if c.cfg.Mode == "relay" {
		return c.runRelayMode(ctx)
	}

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

		if c.cfg.Transparent {
			if os.Geteuid() != 0 {
				c.logger.Warn("transparent proxy requires root privileges, skipping")
			} else {
				mgr := transparent.New()
				port := portFromAddr(c.cfg.ProxyListen)
				var excludes []string
				if c.cfg.Routing != nil {
					excludes = c.cfg.Routing.ExcludeRanges
				}
				if err := mgr.Set(ctx, c.logger, port, excludes); err != nil {
					c.logger.Warn("transparent proxy setup failed", zap.Error(err))
				} else {
					c.logger.Info("transparent proxy active")
					defer func() {
						_ = mgr.Restore(context.Background(), c.logger)
						c.logger.Info("transparent proxy restored")
					}()
				}
			}
		}

		c.runProxyMode(ctx)
		return nil
	}

	tunDev := tun.NewTunDevice()
	if err := tunDev.Open(); err != nil {
		return fmt.Errorf("open tun: %w", err)
	}
	defer func() { _ = tunDev.Close() }()

	c.reconnectLoop(ctx, tunDev)
	return nil
}
