package config

import (
	"os"
	"path/filepath"
	"testing"
)

// @sk-test prod-issue#T2.1: smoke test for server config load and defaults (AC-006)
func TestLoadServerConfigSmoke(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	content := `
listen: ":8443"
tls:
  cert: "/etc/certs/server.pem"
  key: "/etc/certs/server-key.pem"
auth:
  tokens:
    - name: test
      secret: test-secret
network:
  pool_ipv4:
    subnet: "10.88.0.0/16"
    gateway: "10.88.0.1"
    range_start: "10.88.0.10"
    range_end: "10.88.0.200"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadServerConfig(path)
	if err != nil {
		t.Fatalf("LoadServerConfig: %v", err)
	}

	if cfg.Listen != ":8443" {
		t.Errorf("expected :8443, got %s", cfg.Listen)
	}
	if len(cfg.Auth.Tokens) != 1 || cfg.Auth.Tokens[0].Name != "test" {
		t.Errorf("unexpected tokens: %+v", cfg.Auth.Tokens)
	}
	if cfg.RateLimiting.AuthBurst != 5 {
		t.Errorf("expected default AuthBurst 5, got %d", cfg.RateLimiting.AuthBurst)
	}
	if cfg.Admin.Listen != "localhost:8443" {
		t.Errorf("expected default Admin.Listen localhost:8443, got %s", cfg.Admin.Listen)
	}
}

// @sk-test prod-issue#T2.1: smoke test for client config load and defaults (AC-006)
func TestLoadClientConfigSmoke(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.yaml")
	content := `
server: "wss://example.com:8443/tunnel"
auth:
  token: "test-token"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadClientConfig(path)
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}

	if cfg.Server != "wss://example.com:8443/tunnel" {
		t.Errorf("unexpected server: %s", cfg.Server)
	}
	if cfg.TLS.VerifyMode != "verify" {
		t.Errorf("expected default VerifyMode 'verify', got %s", cfg.TLS.VerifyMode)
	}
	if cfg.Routing == nil || cfg.Routing.DefaultRoute != "server" {
		t.Errorf("expected default routing, got %+v", cfg.Routing)
	}
}

// @sk-test prod-issue#T2.1: server config load failure on missing file (AC-006)
func TestLoadServerConfigMissingFile(t *testing.T) {
	_, err := LoadServerConfig("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}
