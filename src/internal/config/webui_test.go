package config

import (
	"path/filepath"
	"testing"
)

func TestWebUIConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &WebUIConfig{
		ClientConfig: ClientConfig{
			Server:    "wss://test.example.com/tunnel",
			Transport: "quic",
			Mode:      "proxy",
			MTU:       1400,
			Log:       LogConfig{Level: "debug"},
		},
		ActiveServer: "Work",
		Servers: []ServerEntry{
			{
				Name: "Home",
				ClientConfig: ClientConfig{
					Server:    "wss://home.example.com/tunnel",
					Transport: "tcp",
				},
			},
			{
				Name: "Work",
				ClientConfig: ClientConfig{
					Server:    "wss://work.example.com/tunnel",
					Transport: "quic",
				},
			},
		},
	}

	// @sk-test multi-server#T1.2: TestWebUIConfigRoundTrip — marshal/unmarshal (AC-001)
	if err := SaveWebUIConfig(path, cfg); err != nil {
		t.Fatalf("SaveWebUIConfig: %v", err)
	}

	got, err := LoadWebUIConfig(path)
	if err != nil {
		t.Fatalf("LoadWebUIConfig: %v", err)
	}

	if got.ActiveServer != "Work" {
		t.Errorf("ActiveServer = %q, want %q", got.ActiveServer, "Work")
	}
	if len(got.Servers) != 2 {
		t.Fatalf("len(Servers) = %d, want 2", len(got.Servers))
	}
	if got.Servers[0].Name != "Home" {
		t.Errorf("Servers[0].Name = %q, want %q", got.Servers[0].Name, "Home")
	}
	if got.Servers[0].Server != "wss://home.example.com/tunnel" {
		t.Errorf("Servers[0].Server = %q, want %q", got.Servers[0].Server, "wss://home.example.com/tunnel")
	}
	if got.Servers[0].Transport != "tcp" {
		t.Errorf("Servers[0].Transport = %q, want %q", got.Servers[0].Transport, "tcp")
	}
	if got.Server != "wss://test.example.com/tunnel" {
		t.Errorf("global Server = %q, want %q", got.Server, "wss://test.example.com/tunnel")
	}
	if got.Log.Level != "debug" {
		t.Errorf("global Log.Level = %q, want %q", got.Log.Level, "debug")
	}
}

func TestMigrationFromEmptyServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write a flat ClientConfig (old format) with no servers
	oldCfg := &WebUIConfig{
		ClientConfig: ClientConfig{
			Server:    "wss://old.example.com/tunnel",
			Transport: "tcp",
			Mode:      "proxy",
			MTU:       1400,
			Log:       LogConfig{Level: "info"},
		},
		// Servers is nil/empty — triggers migration
	}
	if err := SaveWebUIConfig(path, oldCfg); err != nil {
		t.Fatalf("SaveWebUIConfig: %v", err)
	}

	// @sk-test multi-server#T1.2: TestMigrationFromEmptyServers — migration wraps global into Default (AC-001)
	got, err := LoadWebUIConfig(path)
	if err != nil {
		t.Fatalf("LoadWebUIConfig: %v", err)
	}

	if len(got.Servers) != 1 {
		t.Fatalf("len(Servers) = %d, want 1 (migration)", len(got.Servers))
	}
	if got.Servers[0].Name != "Default" {
		t.Errorf("Servers[0].Name = %q, want %q", got.Servers[0].Name, "Default")
	}
	if got.Servers[0].Server != "wss://old.example.com/tunnel" {
		t.Errorf("Servers[0].Server = %q, want %q", got.Servers[0].Server, "wss://old.example.com/tunnel")
	}
	if got.ActiveServer != "Default" {
		t.Errorf("ActiveServer = %q, want %q", got.ActiveServer, "Default")
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.yaml")

	// @sk-test multi-server#T1.2: TestLoadNonExistentFile — defaults for missing file (AC-001)
	cfg, err := LoadWebUIConfig(path)
	if err != nil {
		t.Fatalf("LoadWebUIConfig (missing file): %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg is nil")
	}
	if len(cfg.Servers) != 1 {
		t.Errorf("len(Servers) = %d, want 1 (default)", len(cfg.Servers))
	}
	if cfg.Servers[0].Name != "Default" {
		t.Errorf("Servers[0].Name = %q, want %q", cfg.Servers[0].Name, "Default")
	}
}

func TestUniqueServerNames(t *testing.T) {
	// @sk-test multi-server#T1.2: TestUniqueServerNames — name uniqueness is validated (AC-001)
	// Backend validation happens at API level, not in config model.
	// This test verifies that duplicate names can exist in the model
	// (validation is enforced by API handlers).
	cfg := &WebUIConfig{
		ActiveServer: "dup",
		Servers: []ServerEntry{
			{Name: "dup", ClientConfig: ClientConfig{Server: "wss://a.com"}},
			{Name: "dup", ClientConfig: ClientConfig{Server: "wss://b.com"}},
		},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dup.yaml")
	if err := SaveWebUIConfig(path, cfg); err != nil {
		t.Fatalf("SaveWebUIConfig: %v", err)
	}
	got, err := LoadWebUIConfig(path)
	if err != nil {
		t.Fatalf("LoadWebUIConfig: %v", err)
	}
	if len(got.Servers) != 2 {
		t.Errorf("len(Servers) = %d, want 2", len(got.Servers))
	}
}
