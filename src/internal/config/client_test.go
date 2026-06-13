package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "client.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// @sk-test transparent-proxy#T4.4: transparent config parsing (AC-001, AC-009)
func TestTransparentConfig(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
transparent: true
dns_proxy:
  listen: "127.0.0.1:5353"
`)
	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if !cfg.Transparent {
		t.Error("Transparent = false, want true")
	}
	if cfg.DNSProxy.Listen != "127.0.0.1:5353" {
		t.Errorf("DNSProxy.Listen = %q, want %q", cfg.DNSProxy.Listen, "127.0.0.1:5353")
	}
}

// @sk-test transparent-proxy#T4.4: transparent default values (AC-001)
func TestTransparentDefaults(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
`)
	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.Transparent {
		t.Error("Transparent = true by default, want false")
	}
	if cfg.DNSProxy.Listen != "127.0.0.54:53" {
		t.Errorf("DNSProxy.Listen = %q, want %q", cfg.DNSProxy.Listen, "127.0.0.54:53")
	}
}

// @sk-test whitelist-obfuscation#T5.1: config decoder tests (AC-001, AC-004, AC-005)
// @sk-test whitelist-obfuscation#T5.1: backward compat obfuscation: true (AC-001)
func TestObfuscationBoolBackwardCompat(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
obfuscation: true
`)
	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.Obfuscation == nil {
		t.Fatal("Obfuscation = nil, want non-nil")
	}
	if !cfg.Obfuscation.Enabled {
		t.Fatal("Obfuscation.Enabled = false, want true")
	}
}

// @sk-test whitelist-obfuscation#T5.1: full obfuscation struct + SNI (AC-004, AC-005)
func TestObfuscationStructFull(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
obfuscation:
  enabled: true
  utls:
    enabled: true
    fallback: false
  padding:
    enabled: true
    size: 1024
tls:
  sni:
    - www.cloudflare.com
    - www.google.com
`)
	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.Obfuscation == nil || !cfg.Obfuscation.Enabled {
		t.Fatal("Obfuscation.Enabled should be true")
	}
	if cfg.Obfuscation.UTLS == nil || !cfg.Obfuscation.UTLS.Enabled {
		t.Fatal("UTLS.Enabled should be true")
	}
	if cfg.Obfuscation.UTLS.Fallback {
		t.Fatal("UTLS.Fallback should be false")
	}
	if cfg.Obfuscation.Padding == nil || !cfg.Obfuscation.Padding.Enabled {
		t.Fatal("Padding.Enabled should be true")
	}
	if cfg.Obfuscation.Padding.Size != 1024 {
		t.Fatalf("Padding.Size = %d, want 1024", cfg.Obfuscation.Padding.Size)
	}
	if len(cfg.TLS.SNI) != 2 {
		t.Fatalf("SNI count = %d, want 2", len(cfg.TLS.SNI))
	}
}

// @sk-test arch-refactoring#T4.1: defaults for MaxMessageSize, TunnelTimeout, ProxyMaxConcurrency (AC-006)
func TestNewFieldsDefaults(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
`)
	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.MaxMessageSize != 10*1024*1024 {
		t.Fatalf("MaxMessageSize = %d, want %d", cfg.MaxMessageSize, 10*1024*1024)
	}
	if cfg.TunnelTimeout != 30 {
		t.Fatalf("TunnelTimeout = %d, want 30", cfg.TunnelTimeout)
	}
	if cfg.ProxyMaxConcurrency != 1000 {
		t.Fatalf("ProxyMaxConcurrency = %d, want 1000", cfg.ProxyMaxConcurrency)
	}
}

// @sk-test arch-refactoring#T4.1: custom MaxMessageSize, TunnelTimeout, ProxyMaxConcurrency (AC-006)
func TestNewFieldsCustom(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
max_message_size: 5242880
tunnel_timeout: 60
proxy_max_concurrency: 500
`)
	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.MaxMessageSize != 5*1024*1024 {
		t.Fatalf("MaxMessageSize = %d, want %d", cfg.MaxMessageSize, 5*1024*1024)
	}
	if cfg.TunnelTimeout != 60 {
		t.Fatalf("TunnelTimeout = %d, want 60", cfg.TunnelTimeout)
	}
	if cfg.ProxyMaxConcurrency != 500 {
		t.Fatalf("ProxyMaxConcurrency = %d, want 500", cfg.ProxyMaxConcurrency)
	}
}

// @sk-test arch-refactoring#T4.1: zero values fallback to defaults (AC-006)
func TestNewFieldsZeroDefaults(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
max_message_size: 0
tunnel_timeout: 0
proxy_max_concurrency: 0
`)
	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.MaxMessageSize != 10*1024*1024 {
		t.Fatalf("MaxMessageSize = %d, want %d", cfg.MaxMessageSize, 10*1024*1024)
	}
	if cfg.TunnelTimeout != 30 {
		t.Fatalf("TunnelTimeout = %d, want 30", cfg.TunnelTimeout)
	}
	if cfg.ProxyMaxConcurrency != 1000 {
		t.Fatalf("ProxyMaxConcurrency = %d, want 1000", cfg.ProxyMaxConcurrency)
	}
}

func TestObfuscationMissingDefaults(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
`)
	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.Obfuscation != nil {
		t.Fatal("Obfuscation = non-nil when not in config, want nil")
	}
}

// @sk-test client-relay-mode#T4.1: relay mode without relay block (AC-003)
func TestRelayModeMissingBlock(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
mode: relay
`)
	_, err := LoadClientConfig(path)
	if err == nil {
		t.Fatal("LoadClientConfig: expected error for mode=relay without relay block")
	}
}

// @sk-test client-relay-mode#T4.1: relay mode with valid config (AC-003)
func TestRelayModeValidConfig(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
mode: relay
relay:
  listen: 0.0.0.0:443
`)
	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.Relay == nil {
		t.Fatal("Relay is nil, want non-nil")
	}
	if cfg.Relay.Listen != "0.0.0.0:443" {
		t.Fatalf("Relay.Listen = %q, want %q", cfg.Relay.Listen, "0.0.0.0:443")
	}
	if cfg.Relay.MaxConnections != 100 {
		t.Fatalf("Relay.MaxConnections = %d, want 100", cfg.Relay.MaxConnections)
	}
	if len(cfg.Relay.WSPaths) != 1 || cfg.Relay.WSPaths[0] != "/tunnel" {
		t.Fatalf("Relay.WSPaths = %v, want [\"/tunnel\"]", cfg.Relay.WSPaths)
	}
}

// @sk-test client-relay-mode#T4.1: relay mode with full custom config (AC-003)
func TestRelayModeCustomConfig(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
mode: relay
relay:
  listen: 0.0.0.0:8443
  ws_paths:
    - /api/v1/events
  max_connections: 200
  tls:
    cert: /etc/relay/cert.pem
    key: /etc/relay/key.pem
`)
	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.Relay.Listen != "0.0.0.0:8443" {
		t.Fatalf("Relay.Listen = %q, want %q", cfg.Relay.Listen, "0.0.0.0:8443")
	}
	if len(cfg.Relay.WSPaths) != 1 || cfg.Relay.WSPaths[0] != "/api/v1/events" {
		t.Fatalf("Relay.WSPaths = %v, want [\"/api/v1/events\"]", cfg.Relay.WSPaths)
	}
	if cfg.Relay.MaxConnections != 200 {
		t.Fatalf("Relay.MaxConnections = %d, want 200", cfg.Relay.MaxConnections)
	}
	if cfg.Relay.TLS == nil || cfg.Relay.TLS.Cert != "/etc/relay/cert.pem" || cfg.Relay.TLS.Key != "/etc/relay/key.pem" {
		t.Fatal("Relay.TLS config mismatch")
	}
}

// @sk-test client-relay-mode#T4.1: relay mode missing listen (AC-003)
func TestRelayModeMissingListen(t *testing.T) {
	path := writeConfig(t, `
server: wss://example.com/tunnel
mode: relay
relay:
  max_connections: 50
`)
	_, err := LoadClientConfig(path)
	if err == nil {
		t.Fatal("LoadClientConfig: expected error for missing relay.listen")
	}
}
