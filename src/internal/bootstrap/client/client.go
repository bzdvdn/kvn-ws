package client

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	"github.com/bzdvdn/kvn-ws/src/internal/dnsproxy"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
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
// @sk-task geoip-geosite-integration#T3.1: resolve routing sources (AC-002)
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

	if cfg.DNSProxy.Listen == "" {
		cfg.DNSProxy.Listen = "127.0.0.54:53"
	}
	if len(cfg.DNSProxy.Upstreams) == 0 {
		cfg.DNSProxy.Upstreams = append([]string{}, config.DefaultDNSUpstreams...)
	}

	// @sk-task dns-response-tracker#T1.2: DNSCache defaults (AC-003)
	if cfg.Routing != nil && cfg.Routing.DNSCache == nil {
		cfg.Routing.DNSCache = &config.DNSCacheCfg{Enabled: false, TTL: 60}
	}

	if cfg.Routing != nil && (len(cfg.Routing.IncludeSources) > 0 || len(cfg.Routing.ExcludeSources) > 0) {
		cacheDir := filepath.Join(".", ".source-cache")
		srcResolver := routing.NewResolver(cfg.Routing, cacheDir, logger)
		resolved, rErr := srcResolver.Resolve()
		if rErr != nil {
			logger.Warn("routing source resolve failed, using original config", zap.Error(rErr))
		} else {
			cfg.Routing = resolved
			logger.Info("routing sources resolved",
				zap.Int("include_ranges", len(resolved.IncludeRanges)),
				zap.Int("exclude_ranges", len(resolved.ExcludeRanges)),
			)
		}
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

	cl := &Client{
		cfg:       cfg,
		logger:    logger,
		masterKey: masterKey,
	}
	cl.resolveRoutingSources(*cfgPath)
	return cl, nil
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

// @sk-task geoip-geosite-integration#T3.1: resolve routing sources in constructor (AC-002)
func (c *Client) resolveRoutingSources(cfgPath string) {
	if c.cfg.Routing == nil || (len(c.cfg.Routing.IncludeSources) == 0 && len(c.cfg.Routing.ExcludeSources) == 0) {
		return
	}
	cacheDir := filepath.Join(filepath.Dir(cfgPath), ".source-cache")
	srcResolver := routing.NewResolver(c.cfg.Routing, cacheDir, c.logger)
	resolved, err := srcResolver.Resolve()
	if err != nil {
		c.logger.Warn("routing source resolve failed, using original config", zap.Error(err))
		return
	}
	c.cfg.Routing = resolved
	c.logger.Info("routing sources resolved",
		zap.Int("include_ranges", len(resolved.IncludeRanges)),
		zap.Int("exclude_ranges", len(resolved.ExcludeRanges)),
	)
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

	if c.cfg.Mode == "proxy" {
		var serverCIDR string
		var serverNoProxy string
		u, uErr := url.Parse(c.cfg.Server)
		if uErr == nil {
			host := u.Hostname()
			if ip := net.ParseIP(host); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				serverCIDR = host + "/" + strconv.Itoa(bits)
				serverNoProxy = host
			} else {
				if resolved := resolveServerIP(host); resolved != nil {
					bits := 32
					if resolved.To4() == nil {
						bits = 128
					}
					serverCIDR = resolved.String() + "/" + strconv.Itoa(bits)
					serverNoProxy = resolved.String() + "," + host
				}
			}
		}

		if c.cfg.SystemProxy != nil && *c.cfg.SystemProxy {
			sysProxy := systemproxy.New(c.cfg)
			addr := c.cfg.ProxyListen
			if addr == "" {
				addr = "127.0.0.1:2310"
			}
			noProxy := systemproxy.NOProxyBuilder(c.cfg.Routing)
			if serverNoProxy != "" {
				if noProxy != "" {
					noProxy += ","
				}
				noProxy += serverNoProxy
			}
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
				if serverCIDR != "" {
					excludes = append(excludes, serverCIDR)
				}
				if c.cfg.Routing != nil {
					for _, r := range c.cfg.Routing.ExcludeRanges {
						if r != "0.0.0.0/0" && r != "::/0" {
							excludes = append(excludes, r)
						}
					}
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
	// @sk-task relay-terminator#T9.3: TUN cleanup on graceful disconnect (AC-006)
	defer func() { _ = tunDev.Close() }()

	// Force-close TUN on context cancellation to unblock tunnel reads
	// (tunDev.Read() does not take a context). This ensures that the session
	// goroutines unblock, defers run, and resolv.conf / routes are cleaned up.
	go func() {
		select {
		case <-ctx.Done():
			_ = tunDev.Close()
		}
	}()

	c.reconnectLoop(ctx, tunDev)
	return nil
}
