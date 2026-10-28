package webui

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

func setupV2TestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	configDir := filepath.Join(dir, "kvn")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	s := &Server{state: NewAppState(), configDir: configDir, v2Only: true}
	return s, configDir
}

// @sk-test kvn-v2-transport#T3.4: v2 UI writes only config.yaml (AC-005)
func TestConfigStoreIsolation(t *testing.T) {
	s, configDir := setupV2TestServer(t)
	rootPath := filepath.Join(configDir, "config.yaml")
	v2Path := rootPath

	if err := os.WriteFile(rootPath, []byte("server: wss://v1.example/tunnel\nmode: tun\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/servers", s.handleListServers)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("POST /api/servers", s.handleCreateServer)
	mux.HandleFunc("PUT /api/servers/{name}", s.handleUpdateServer)
	mux.HandleFunc("DELETE /api/servers/{name}", s.handleDeleteServer)
	mux.HandleFunc("PUT /api/config/global", s.handleSaveGlobalConfig)

	w := doRequest(t, mux, "GET", "/api/config", nil)
	var emptyCfg struct {
		Config config.ClientConfig `json:"config"`
	}
	if err := json.NewDecoder(w.Body).Decode(&emptyCfg); err != nil {
		t.Fatal(err)
	}
	if emptyCfg.Config.Protocol != "v2" || emptyCfg.Config.Mode != "tun" || emptyCfg.Config.MTU != 1200 {
		t.Fatalf("v2 empty defaults = %+v", emptyCfg.Config)
	}
	if emptyCfg.Config.UTLS == nil || emptyCfg.Config.UTLS.Profile != "chrome" || len(emptyCfg.Config.UTLS.Profiles) < 3 {
		t.Fatalf("v2 utls defaults = %+v", emptyCfg.Config.UTLS)
	}
	if emptyCfg.Config.Stealth == nil || emptyCfg.Config.Stealth.Padding == nil || !*emptyCfg.Config.Stealth.Padding {
		t.Fatalf("v2 stealth defaults = %+v", emptyCfg.Config.Stealth)
	}

	w = doRequest(t, mux, "POST", "/api/servers", map[string]any{
		"name":   "v2-test",
		"server": "https://88.210.63.157",
		"auth":   map[string]any{"token": "test-token"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create v2 server: %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(v2Path); err != nil {
		t.Fatalf("v2 config missing: %v", err)
	}
	backup, err := os.ReadFile(rootPath + ".v1.bak")
	if err != nil {
		t.Fatalf("v1 backup missing: %v", err)
	}
	if string(backup) != string(before) {
		t.Fatalf("v1 config backup changed:\n%s\n---\n%s", before, backup)
	}

	v2, err := config.LoadWebUIConfigOptional(v2Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(v2.Servers) != 1 || v2.Servers[0].Protocol != "v2" {
		t.Fatalf("v2 servers = %+v", v2.Servers)
	}
	if v2.Servers[0].Mode != "tun" || v2.Servers[0].MTU != 1200 {
		t.Fatalf("v2 defaults not applied: %+v", v2.Servers[0].ClientConfig)
	}

	// Global save must stay in v2 file.
	w = doRequest(t, mux, "PUT", "/api/config/global", map[string]any{
		"active_server": "v2-test",
		"protocol":      "v2",
		"mode":          "tun",
		"mtu":           1200,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("save global: %d %s", w.Code, w.Body.String())
	}

	// List sees only the v2 config, without the synthetic v1 Default server.
	w = doRequest(t, mux, "GET", "/api/servers", nil)
	var list struct {
		ActiveServer string               `json:"active_server"`
		Servers      []config.ServerEntry `json:"servers"`
	}
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Servers) != 1 || list.Servers[0].Name != "v2-test" {
		t.Fatalf("v2 list = %+v", list)
	}
	if list.ActiveServer != "v2-test" {
		t.Fatalf("active = %q", list.ActiveServer)
	}

	w = doRequest(t, mux, "DELETE", "/api/servers/v2-test", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("delete last server: expected 409, got %d", w.Code)
	}
}

// @sk-test kvn-v2-transport#T3.4: old shared v2 entries migrate without touching v1 (AC-005)
func TestConfigStoreMigratesLegacyV2(t *testing.T) {
	s, configDir := setupV2TestServer(t)
	rootPath := filepath.Join(configDir, "config.yaml")
	root := &config.WebUIConfig{
		ClientConfig: config.ClientConfig{Server: "wss://v1.example/tunnel", Mode: "tun"},
		ActiveServer: "v2-existing",
		Servers: []config.ServerEntry{
			{Name: "v1-existing", ClientConfig: config.ClientConfig{Server: "wss://v1.example/tunnel"}},
			{Name: "v2-existing", ClientConfig: config.ClientConfig{Protocol: "v2", Server: "https://v2.example", Mode: "tun"}},
		},
	}
	if err := config.SaveWebUIConfig(rootPath, root); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/servers", s.handleListServers)
	w := doRequest(t, mux, "GET", "/api/servers", nil)
	var list struct {
		ActiveServer string               `json:"active_server"`
		Servers      []config.ServerEntry `json:"servers"`
	}
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Servers) != 1 || list.Servers[0].Name != "v2-existing" || list.ActiveServer != "v2-existing" {
		t.Fatalf("migrated list = %+v", list)
	}
	after, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) == string(before) {
		t.Fatal("migration did not unify v2 config")
	}
	backup, err := os.ReadFile(rootPath + ".v1.bak")
	if err != nil {
		t.Fatalf("v1 backup missing: %v", err)
	}
	if string(backup) != string(before) {
		t.Fatal("v1 backup changed")
	}
}
