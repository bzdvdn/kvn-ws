package webui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

func setupTestServer(t *testing.T) (srv *Server, dir string) {
	t.Helper()
	dir = t.TempDir()
	configDir := filepath.Join(dir, "kvn")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		state:     NewAppState(),
		configDir: configDir,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/servers", s.handleListServers)
	mux.HandleFunc("POST /api/servers", s.handleCreateServer)
	mux.HandleFunc("PUT /api/servers/{name}", s.handleUpdateServer)
	mux.HandleFunc("DELETE /api/servers/{name}", s.handleDeleteServer)
	mux.HandleFunc("PUT /api/config/global", s.handleSaveGlobalConfig)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)

	return s, dir
}

func doRequest(t *testing.T, mux http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// @sk-test multi-server#T4.1: TestListServersEmpty (AC-001)
func TestListServersEmpty(t *testing.T) {
	_, dir := setupTestServer(t)
	cfgPath := filepath.Join(dir, "kvn", "config.yaml")
	t.Cleanup(func() { os.RemoveAll(dir) })

	s := &Server{
		state:     NewAppState(),
		configDir: filepath.Dir(cfgPath),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/servers", s.handleListServers)

	w := doRequest(t, mux, "GET", "/api/servers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/servers: status %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	servers := resp["servers"].([]any)
	if len(servers) == 0 {
		t.Fatal("expected at least 1 server (migration)")
	}
}

// @sk-test multi-server#T4.1: TestCreateServer (AC-003)
func TestCreateServer(t *testing.T) {
	_, dir := setupTestServer(t)
	cfgPath := filepath.Join(dir, "kvn", "config.yaml")
	t.Cleanup(func() { os.RemoveAll(dir) })

	s := &Server{
		state:     NewAppState(),
		configDir: filepath.Dir(cfgPath),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/servers", s.handleListServers)
	mux.HandleFunc("POST /api/servers", s.handleCreateServer)
	mux.HandleFunc("DELETE /api/servers/{name}", s.handleDeleteServer)

	// Create server
	w := doRequest(t, mux, "POST", "/api/servers", map[string]any{
		"name":   "TestServer",
		"server": "wss://test.example.com/tunnel",
		"mode":   "proxy",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /api/servers: status %d: %s", w.Code, w.Body.String())
	}

	// Verify in list
	w = doRequest(t, mux, "GET", "/api/servers", nil)
	var resp struct {
		ActiveServer string           `json:"active_server"`
		Servers      []map[string]any `json:"servers"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sv := range resp.Servers {
		if sv["name"] == "TestServer" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("TestServer not found after creation")
	}
}

// @sk-test multi-server#T4.1: TestCreateDuplicateServer (AC-003)
func TestCreateDuplicateServer(t *testing.T) {
	_, dir := setupTestServer(t)
	cfgPath := filepath.Join(dir, "kvn", "config.yaml")
	t.Cleanup(func() { os.RemoveAll(dir) })

	s := &Server{
		state:     NewAppState(),
		configDir: filepath.Dir(cfgPath),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/servers", s.handleCreateServer)

	// First create
	w := doRequest(t, mux, "POST", "/api/servers", map[string]any{"name": "Dup"})
	if w.Code != http.StatusCreated {
		t.Fatalf("first create: status %d", w.Code)
	}

	// Duplicate
	w = doRequest(t, mux, "POST", "/api/servers", map[string]any{"name": "Dup"})
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate create: expected 409, got %d", w.Code)
	}
}

// @sk-test multi-server#T4.1: TestUpdateServer (AC-002, AC-003)
func TestUpdateServer(t *testing.T) {
	_, dir := setupTestServer(t)
	cfgPath := filepath.Join(dir, "kvn", "config.yaml")
	t.Cleanup(func() { os.RemoveAll(dir) })

	s := &Server{
		state:     NewAppState(),
		configDir: filepath.Dir(cfgPath),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/servers", s.handleListServers)
	mux.HandleFunc("PUT /api/servers/{name}", s.handleUpdateServer)
	mux.HandleFunc("POST /api/servers", s.handleCreateServer)

	// Create first
	doRequest(t, mux, "POST", "/api/servers", map[string]any{
		"name":   "OldName",
		"server": "wss://old.example.com/tunnel",
	})

	// Update (rename)
	w := doRequest(t, mux, "PUT", "/api/servers/OldName", map[string]any{
		"name":   "NewName",
		"server": "wss://new.example.com/tunnel",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/servers/OldName: status %d: %s", w.Code, w.Body.String())
	}

	// Verify
	w = doRequest(t, mux, "GET", "/api/servers", nil)
	var resp struct {
		Servers []map[string]any `json:"servers"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	for _, sv := range resp.Servers {
		if sv["name"] == "OldName" {
			t.Fatal("OldName should not exist after rename")
		}
	}
	found := false
	for _, sv := range resp.Servers {
		if sv["name"] == "NewName" {
			found = true
			if sv["server"] != "wss://new.example.com/tunnel" {
				t.Errorf("server = %q, want wss://new.example.com/tunnel", sv["server"])
			}
			break
		}
	}
	if !found {
		t.Fatal("NewName not found")
	}
}

// @sk-test multi-server#T4.1: TestDeleteServer (AC-003)
func TestDeleteServer(t *testing.T) {
	_, dir := setupTestServer(t)
	cfgPath := filepath.Join(dir, "kvn", "config.yaml")
	t.Cleanup(func() { os.RemoveAll(dir) })

	s := &Server{
		state:     NewAppState(),
		configDir: filepath.Dir(cfgPath),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/servers", s.handleCreateServer)
	mux.HandleFunc("DELETE /api/servers/{name}", s.handleDeleteServer)
	mux.HandleFunc("GET /api/servers", s.handleListServers)

	// Create an additional server (Default is auto-migrated, so 2 total)
	doRequest(t, mux, "POST", "/api/servers", map[string]any{"name": "Extra"})

	// Delete Extra
	w := doRequest(t, mux, "DELETE", "/api/servers/Extra", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE /api/servers/Extra: status %d: %s", w.Code, w.Body.String())
	}

	// Verify
	w = doRequest(t, mux, "GET", "/api/servers", nil)
	var resp struct {
		Servers []map[string]any `json:"servers"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	for _, sv := range resp.Servers {
		if sv["name"] == "Extra" {
			t.Fatal("Extra should be deleted")
		}
	}
}

// @sk-test multi-server#T4.1: TestDeleteLastServer (AC-003)
func TestDeleteLastServer(t *testing.T) {
	_, dir := setupTestServer(t)
	cfgPath := filepath.Join(dir, "kvn", "config.yaml")
	t.Cleanup(func() { os.RemoveAll(dir) })

	s := &Server{
		state:     NewAppState(),
		configDir: filepath.Dir(cfgPath),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/servers/{name}", s.handleDeleteServer)

	// Delete the only server (Default)
	w := doRequest(t, mux, "DELETE", "/api/servers/Default", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for deleting last server, got %d", w.Code)
	}
}

// @sk-test multi-server#T4.1: TestSaveGlobalConfig (AC-006)
func TestSaveGlobalConfig(t *testing.T) {
	_, dir := setupTestServer(t)
	cfgPath := filepath.Join(dir, "kvn", "config.yaml")
	t.Cleanup(func() { os.RemoveAll(dir) })

	s := &Server{
		state:     NewAppState(),
		configDir: filepath.Dir(cfgPath),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/config/global", s.handleSaveGlobalConfig)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)

	// Save global config
	w := doRequest(t, mux, "PUT", "/api/config/global", map[string]any{
		"log":          map[string]any{"level": "debug"},
		"proxy_listen": "127.0.0.1:3128",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/config/global: status %d: %s", w.Code, w.Body.String())
	}

	// Verify
	w = doRequest(t, mux, "GET", "/api/config", nil)
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	cfg := resp["config"].(map[string]any)
	logCfg := cfg["log"].(map[string]any)
	if logCfg["level"] != "debug" {
		t.Errorf("log.level = %q, want debug", logCfg["level"])
	}
	if cfg["proxy_listen"] != "127.0.0.1:3128" {
		t.Errorf("proxy_listen = %q, want 127.0.0.1:3128", cfg["proxy_listen"])
	}
}

// @sk-test dns-upstreams-list#T4.1: TestMergeConfigDNSUpstreams (AC-009)
func TestMergeConfigDNSUpstreams(t *testing.T) {
	global := config.ClientConfig{
		DNSProxy: config.DNSProxyCfg{
			Listen:    "127.0.0.54:53",
			Upstreams: []string{"1.1.1.1:53"},
		},
	}
	server := config.ClientConfig{
		DNSProxy: config.DNSProxyCfg{
			Listen:    "127.0.0.54:53",
			Upstreams: []string{"10.0.0.1:53"},
		},
	}
	merged := mergeConfig(&global, &server)
	if len(merged.DNSProxy.Upstreams) != 1 {
		t.Fatalf("merged Upstreams = %v, want [10.0.0.1:53]", merged.DNSProxy.Upstreams)
	}
	if merged.DNSProxy.Upstreams[0] != "10.0.0.1:53" {
		t.Errorf("merged Upstreams[0] = %q, want %q", merged.DNSProxy.Upstreams[0], "10.0.0.1:53")
	}
}

// @sk-test dns-upstreams-list#T4.1: TestMergeConfigDNSUpstreamsNotOverridden (AC-009)
func TestMergeConfigDNSUpstreamsNotOverridden(t *testing.T) {
	global := config.ClientConfig{
		DNSProxy: config.DNSProxyCfg{
			Listen:    "127.0.0.54:53",
			Upstreams: []string{"1.1.1.1:53", "8.8.8.8:53"},
		},
	}
	server := config.ClientConfig{}
	merged := mergeConfig(&global, &server)
	if len(merged.DNSProxy.Upstreams) != 2 {
		t.Fatalf("merged Upstreams = %v, want global upstreams preserved", merged.DNSProxy.Upstreams)
	}
	if merged.DNSProxy.Upstreams[0] != "1.1.1.1:53" {
		t.Errorf("merged Upstreams[0] = %q, want %q", merged.DNSProxy.Upstreams[0], "1.1.1.1:53")
	}
}

// @sk-test dns-upstreams-list#T4.1: TestDNSProxyCfgJSONRoundTrip (AC-008)
func TestDNSProxyCfgJSONRoundTrip(t *testing.T) {
	// new format: upstreams
	orig := config.DNSProxyCfg{
		Listen:    "127.0.0.54:53",
		Upstreams: []string{"1.1.1.1:53", "8.8.8.8:53"},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded config.DNSProxyCfg
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Listen != orig.Listen {
		t.Errorf("Listen = %q, want %q", decoded.Listen, orig.Listen)
	}
	if len(decoded.Upstreams) != 2 || decoded.Upstreams[0] != "1.1.1.1:53" {
		t.Errorf("Upstreams = %v, want [1.1.1.1:53 8.8.8.8:53]", decoded.Upstreams)
	}
}

// @sk-test dns-upstreams-list#T4.1: TestDNSProxyCfgJSONBackwardCompat (AC-010)
func TestDNSProxyCfgJSONBackwardCompat(t *testing.T) {
	oldJSON := `{"listen":"127.0.0.54:53","upstream":"1.1.1.1:53"}`
	var cfg config.DNSProxyCfg
	if err := json.Unmarshal([]byte(oldJSON), &cfg); err != nil {
		t.Fatalf("Unmarshal old format: %v", err)
	}
	if len(cfg.Upstreams) != 1 || cfg.Upstreams[0] != "1.1.1.1:53" {
		t.Errorf("Upstreams = %v, want [1.1.1.1:53]", cfg.Upstreams)
	}
}
