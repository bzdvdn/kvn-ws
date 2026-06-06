// @sk-test whitelist-obfuscation#T5.1: config decoder tests (AC-001, AC-004, AC-005)
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
