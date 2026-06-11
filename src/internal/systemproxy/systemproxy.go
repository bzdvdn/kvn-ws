package systemproxy

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

const (
	envHTTPProxy   = "HTTP_PROXY"
	envHTTPSProxy  = "HTTPS_PROXY"
	envNOProxy     = "NO_PROXY"
	envHTTPProxyL  = "http_proxy"
	envHTTPSProxyL = "https_proxy"
	envNOProxyL    = "no_proxy"
)

// @sk-task system-proxy#T1.1: platform-specific proxy manager (AC-004, AC-005)
type PlatformManager interface {
	Set(ctx context.Context, logger *zap.Logger, addr string, noProxy string) error
	Restore(ctx context.Context, logger *zap.Logger) error
}

// @sk-task system-proxy#T1.1: State holds original env values and platform manager (AC-002)
type State struct {
	origHTTP  string
	origHTTPS string
	origNO    string
	hadHTTP   bool
	hadHTTPS  bool
	hadNO     bool
	platform  PlatformManager
}

func New(cfg *config.ClientConfig) *State {
	return &State{platform: newPlatformManager()}
}

// @sk-task system-proxy#T1.1: set system proxy (env vars + platform API) (AC-001, AC-002, AC-003)
// @sk-task system-proxy#T3.3: recovery check — detect orphaned proxy from crashed instance (AC-006)
func (s *State) Set(ctx context.Context, logger *zap.Logger, addr, noProxy string) error {
	proxyURL := fmt.Sprintf("http://%s", addr)

	// recovery: if current HTTP_PROXY points to us, it's orphaned from a crash
	if cur, ok := os.LookupEnv(envHTTPProxy); ok && cur == proxyURL {
		logger.Warn("recovering orphaned system proxy from previous instance",
			zap.String("orphaned", cur),
		)
		os.Unsetenv(envHTTPProxy)
		os.Unsetenv(envHTTPProxyL)
		os.Unsetenv(envHTTPSProxy)
		os.Unsetenv(envHTTPSProxyL)
		os.Unsetenv(envNOProxy)
		os.Unsetenv(envNOProxyL)
	}

	// save originals
	s.origHTTP, s.hadHTTP = os.LookupEnv(envHTTPProxy)
	s.origHTTPS, s.hadHTTPS = os.LookupEnv(envHTTPSProxy)
	s.origNO, s.hadNO = os.LookupEnv(envNOProxy)

	if err := os.Setenv(envHTTPProxy, proxyURL); err != nil {
		return fmt.Errorf("set %s: %w", envHTTPProxy, err)
	}
	if err := os.Setenv(envHTTPProxyL, proxyURL); err != nil {
		return fmt.Errorf("set %s: %w", envHTTPProxyL, err)
	}
	if err := os.Setenv(envHTTPSProxy, proxyURL); err != nil {
		return fmt.Errorf("set %s: %w", envHTTPSProxy, err)
	}
	if err := os.Setenv(envHTTPSProxyL, proxyURL); err != nil {
		return fmt.Errorf("set %s: %w", envHTTPSProxyL, err)
	}

	if noProxy != "" {
		if err := os.Setenv(envNOProxy, noProxy); err != nil {
			return fmt.Errorf("set %s: %w", envNOProxy, err)
		}
		if err := os.Setenv(envNOProxyL, noProxy); err != nil {
			return fmt.Errorf("set %s: %w", envNOProxyL, err)
		}
	}

	if s.platform != nil {
		if err := s.platform.Set(ctx, logger, addr, noProxy); err != nil {
			logger.Warn("platform system proxy set failed", zap.Error(err))
		}
	}

	logger.Info("system proxy set",
		zap.String("http_proxy", proxyURL),
		zap.String("https_proxy", proxyURL),
		zap.String("no_proxy", noProxy),
	)
	return nil
}

// @sk-task system-proxy#T1.1: restore original proxy values (AC-002)
func (s *State) Restore(ctx context.Context, logger *zap.Logger) error {
	restoreOne := func(key string, value string, existed bool) {
		if existed {
			_ = os.Setenv(key, value)
		} else {
			os.Unsetenv(key)
		}
	}

	restoreOne(envHTTPProxy, s.origHTTP, s.hadHTTP)
	restoreOne(envHTTPProxyL, s.origHTTP, s.hadHTTP)
	restoreOne(envHTTPSProxy, s.origHTTPS, s.hadHTTPS)
	restoreOne(envHTTPSProxyL, s.origHTTPS, s.hadHTTPS)
	restoreOne(envNOProxy, s.origNO, s.hadNO)
	restoreOne(envNOProxyL, s.origNO, s.hadNO)

	if s.platform != nil {
		if err := s.platform.Restore(ctx, logger); err != nil {
			logger.Warn("platform system proxy restore failed", zap.Error(err))
		}
	}

	logger.Info("system proxy restored")
	return nil
}

// @sk-task system-proxy#T1.1: build NO_PROXY from routing exclude rules (AC-003)
func NOProxyBuilder(cfg *config.RoutingCfg) string {
	if cfg == nil {
		return ""
	}
	var parts []string
	for _, cidr := range cfg.ExcludeRanges {
		cidr = strings.TrimSpace(cidr)
		if cidr != "" {
			if _, err := netip.ParsePrefix(cidr); err == nil {
				parts = append(parts, cidr)
			}
		}
	}
	for _, ip := range cfg.ExcludeIPs {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			if _, err := netip.ParseAddr(ip); err == nil {
				parts = append(parts, ip)
			}
		}
	}
	for _, d := range cfg.ExcludeDomains {
		d = strings.TrimSpace(d)
		if d != "" {
			if !strings.HasPrefix(d, ".") {
				parts = append(parts, "."+d)
			} else {
				parts = append(parts, d)
			}
		}
	}
	return strings.Join(parts, ",")
}
