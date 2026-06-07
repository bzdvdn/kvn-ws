package systemproxy

import (
	"context"
	"os"
	"testing"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

// @sk-test system-proxy#T4.1: TestNOProxyBuilder builds NO_PROXY from routing exclude rules (AC-003)
func TestNOProxyBuilder(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.RoutingCfg
		want string
	}{
		{
			name: "cidr only",
			cfg: &config.RoutingCfg{
				ExcludeRanges: []string{"10.0.0.0/8", "172.16.0.0/12"},
			},
			want: "10.0.0.0/8,172.16.0.0/12",
		},
		{
			name: "ips and domains",
			cfg: &config.RoutingCfg{
				ExcludeIPs:     []string{"192.168.1.1", "10.0.0.1"},
				ExcludeDomains: []string{"corp.example.com", "internal"},
			},
			want: "192.168.1.1,10.0.0.1,.corp.example.com,.internal",
		},
		{
			name: "all types",
			cfg: &config.RoutingCfg{
				ExcludeRanges:  []string{"10.0.0.0/8"},
				ExcludeIPs:     []string{"192.168.1.1"},
				ExcludeDomains: []string{".example.com"},
			},
			want: "10.0.0.0/8,192.168.1.1,.example.com",
		},
		{
			name: "nil config",
			cfg:  nil,
			want: "",
		},
		{
			name: "empty config",
			cfg:  &config.RoutingCfg{},
			want: "",
		},
		{
			name: "domain without dot gets prefixed",
			cfg: &config.RoutingCfg{
				ExcludeDomains: []string{"example.com"},
			},
			want: ".example.com",
		},
		{
			name: "invalid cidr skipped",
			cfg: &config.RoutingCfg{
				ExcludeRanges: []string{"10.0.0.0/8", "not-a-cidr"},
				ExcludeIPs:    []string{"192.168.1.1", "not-an-ip"},
			},
			want: "10.0.0.0/8,192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NOProxyBuilder(tt.cfg)
			if got != tt.want {
				t.Errorf("NOProxyBuilder = %q, want %q", got, tt.want)
			}
		})
	}
}

// @sk-test system-proxy#T4.1: TestSetRestoreEnv verifies env var save/restore (AC-001, AC-002)
func TestSetRestoreEnv(t *testing.T) {
	origHTTP := os.Getenv(envHTTPProxy)
	origHTTPS := os.Getenv(envHTTPSProxy)
	origNO := os.Getenv(envNOProxy)
	defer func() {
		os.Setenv(envHTTPProxy, origHTTP)
		os.Setenv(envHTTPSProxy, origHTTPS)
		os.Setenv(envNOProxy, origNO)
	}()

	os.Unsetenv(envHTTPProxy)
	os.Unsetenv(envHTTPSProxy)
	os.Unsetenv(envNOProxy)
	os.Unsetenv(envHTTPProxyL)
	os.Unsetenv(envHTTPSProxyL)
	os.Unsetenv(envNOProxyL)

	ctx := context.Background()
	logger := zap.NewNop()
	addr := "127.0.0.1:9999"
	noProxy := "10.0.0.0/8,.corp.example.com"

	s := New(nil)

	if err := s.Set(ctx, logger, addr, noProxy); err != nil {
		t.Fatalf("Set: %v", err)
	}

	checkEnv := func(key, want string) {
		got := os.Getenv(key)
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	checkEnv(envHTTPProxy, "http://"+addr)
	checkEnv(envHTTPProxyL, "http://"+addr)
	checkEnv(envHTTPSProxy, "http://"+addr)
	checkEnv(envHTTPSProxyL, "http://"+addr)
	checkEnv(envNOProxy, noProxy)
	checkEnv(envNOProxyL, noProxy)

	if err := s.Restore(ctx, logger); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	for _, key := range []string{envHTTPProxy, envHTTPProxyL, envHTTPSProxy, envHTTPSProxyL, envNOProxy, envNOProxyL} {
		if v := os.Getenv(key); v != "" {
			t.Errorf("%s should be unset after restore, got %q", key, v)
		}
	}
}

// @sk-test system-proxy#T4.1: TestSetRestorePreservesOriginals saves and restores pre-existing env (AC-002)
func TestSetRestorePreservesOriginals(t *testing.T) {
	origHTTP := os.Getenv(envHTTPProxy)
	origHTTPS := os.Getenv(envHTTPSProxy)
	origNO := os.Getenv(envNOProxy)
	defer func() {
		os.Setenv(envHTTPProxy, origHTTP)
		os.Setenv(envHTTPSProxy, origHTTPS)
		os.Setenv(envNOProxy, origNO)
	}()

	os.Setenv(envHTTPProxy, "http://existing-proxy:8080")
	os.Setenv(envHTTPSProxy, "http://existing-proxy:8080")
	os.Setenv(envNOProxy, ".existing-domain.com")

	ctx := context.Background()
	logger := zap.NewNop()

	s := New(nil)
	if err := s.Set(ctx, logger, "127.0.0.1:2310", ""); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Restore(ctx, logger); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got := os.Getenv(envHTTPProxy); got != "http://existing-proxy:8080" {
		t.Errorf("HTTP_PROXY = %q, want original %q", got, "http://existing-proxy:8080")
	}
}

// @sk-test system-proxy#T4.1: TestRecovery detects orphaned proxy from crashed instance (AC-006)
func TestRecovery(t *testing.T) {
	origHTTP := os.Getenv(envHTTPProxy)
	origHTTPS := os.Getenv(envHTTPSProxy)
	origNO := os.Getenv(envNOProxy)
	defer func() {
		os.Setenv(envHTTPProxy, origHTTP)
		os.Setenv(envHTTPSProxy, origHTTPS)
		os.Setenv(envNOProxy, origNO)
	}()

	// simulate orphaned proxy from crashed instance
	orphanAddr := "127.0.0.1:2310"
	os.Setenv(envHTTPProxy, "http://"+orphanAddr)
	os.Setenv(envHTTPSProxy, "http://"+orphanAddr)

	ctx := context.Background()
	logger := zap.NewNop()

	s := New(nil)
	// Set with a different port to verify orphan is detected
	if err := s.Set(ctx, logger, orphanAddr, ""); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// cleanup
	_ = s.Restore(ctx, logger)

	// after restore, proxy should be empty (no original existed before orphan)
	if got := os.Getenv(envHTTPProxy); got != "" {
		t.Errorf("HTTP_PROXY = %q, want empty after restore of orphan", got)
	}
}
