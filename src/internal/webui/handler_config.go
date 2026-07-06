package webui

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
)

// @sk-task multi-server#T2.1: multi-server API handlers (AC-001, AC-002, AC-003)
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadWebUIConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// @sk-task kvn-web-config-update#T3.1: apply defaults so UI shows correct values (AC-005, AC-006)
	config.SetClientDefaults(&cfg.ClientConfig)
	writeJSON(w, http.StatusOK, map[string]any{"config": cfg.ClientConfig})
}

// @sk-task multi-server#T2.1: save global settings + active_server (AC-006)
func (s *Server) handleSaveGlobalConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadWebUIConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var body struct {
		ActiveServer string `json:"active_server"`
		config.ClientConfig
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if body.ActiveServer != "" {
		cfg.ActiveServer = body.ActiveServer
	}
	cfg.ClientConfig = body.ClientConfig
	dedupRoutingStrings(cfg.Routing)

	if err := s.saveWebUIConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// @sk-task multi-server#T2.1: list servers (AC-001)
func (s *Server) handleListServers(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadWebUIConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for i := range cfg.Servers {
		config.SetClientDefaults(&cfg.Servers[i].ClientConfig)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active_server": cfg.ActiveServer,
		"servers":       cfg.Servers,
	})
}

// @sk-task multi-server#T2.1: create server (AC-003)
func (s *Server) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadWebUIConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var entry config.ServerEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if entry.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	for i := range cfg.Servers {
		if cfg.Servers[i].Name == entry.Name {
			http.Error(w, "server name already exists", http.StatusConflict)
			return
		}
	}

	// @sk-task multi-server: ensure routing defaults on create (AC-001)
	if entry.Routing == nil {
		entry.Routing = &config.RoutingCfg{
			DefaultRoute:  "server",
			ExcludeRanges: config.DefaultExcludeRanges,
		}
	} else {
		if entry.Routing.DefaultRoute == "" {
			entry.Routing.DefaultRoute = "server"
		}
		if len(entry.Routing.ExcludeRanges) == 0 {
			entry.Routing.ExcludeRanges = config.DefaultExcludeRanges
		}
	}

	cfg.Servers = append(cfg.Servers, entry)
	if err := s.saveWebUIConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"name": entry.Name})
}

// @sk-task multi-server#T2.1: update server (AC-002, AC-003)
func (s *Server) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	cfg, err := s.loadWebUIConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var entry config.ServerEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	idx := -1
	for i := range cfg.Servers {
		if cfg.Servers[i].Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	// Check for name conflicts when renaming
	if entry.Name != "" && entry.Name != name {
		for i := range cfg.Servers {
			if cfg.Servers[i].Name == entry.Name {
				http.Error(w, "server name already exists", http.StatusConflict)
				return
			}
		}
	}

	if entry.Name != "" {
		cfg.Servers[idx].Name = entry.Name
	}
	cfg.Servers[idx].ClientConfig = entry.ClientConfig
	dedupRoutingStrings(cfg.Servers[idx].Routing)

	// @sk-task multi-server: ensure routing defaults on update (AC-001)
	sv := &cfg.Servers[idx]
	if sv.Routing == nil {
		sv.Routing = &config.RoutingCfg{
			DefaultRoute:  "server",
			ExcludeRanges: config.DefaultExcludeRanges,
		}
	} else {
		if sv.Routing.DefaultRoute == "" {
			sv.Routing.DefaultRoute = "server"
		}
		if len(sv.Routing.ExcludeRanges) == 0 {
			sv.Routing.ExcludeRanges = config.DefaultExcludeRanges
		}
	}

	// Update active_server if the renamed server was active
	if cfg.ActiveServer == name {
		cfg.ActiveServer = cfg.Servers[idx].Name
	}

	if err := s.saveWebUIConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"name": cfg.Servers[idx].Name})
}

// @sk-task multi-server#T2.1: delete server (AC-003)
func (s *Server) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	cfg, err := s.loadWebUIConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(cfg.Servers) <= 1 {
		http.Error(w, "cannot delete the last server", http.StatusConflict)
		return
	}

	idx := -1
	for i := range cfg.Servers {
		if cfg.Servers[i].Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	cfg.Servers = append(cfg.Servers[:idx], cfg.Servers[idx+1:]...)

	// Update active_server if the deleted server was active
	if cfg.ActiveServer == name {
		if len(cfg.Servers) > 0 {
			cfg.ActiveServer = cfg.Servers[0].Name
		} else {
			cfg.ActiveServer = ""
		}
	}

	if err := s.saveWebUIConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// @sk-task geoip-geosite-integration#T5.1: refresh sources endpoint (AC-010, AC-011)
func (s *Server) handleRefreshSources(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadWebUIConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rc := cfg.ClientConfig.Routing
	if rc == nil || (len(rc.IncludeSources) == 0 && len(rc.ExcludeSources) == 0) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "no sources to refresh"})
		return
	}

	cacheDir := filepath.Join(s.configDir, ".source-cache")
	resolver := routing.NewResolver(rc, cacheDir, zap.NewNop())
	resolved, err := resolver.Refresh()
	if err != nil {
		http.Error(w, "refresh failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"include_ranges":  resolved.IncludeRanges,
		"exclude_ranges":  resolved.ExcludeRanges,
		"include_domains": resolved.IncludeDomains,
		"exclude_domains": resolved.ExcludeDomains,
	})
}

func (s *Server) cfgPath() string {
	return filepath.Join(s.configDir, "config.yaml")
}

func (s *Server) loadWebUIConfig() (*config.WebUIConfig, error) {
	return config.LoadWebUIConfig(s.cfgPath())
}

func (s *Server) saveWebUIConfig(cfg *config.WebUIConfig) error {
	return config.SaveWebUIConfig(s.cfgPath(), cfg)
}

// @sk-task kvn-web-config-update#T3.1: dedup routes (AC-004, AC-007)
func dedupRoutingStrings(r *config.RoutingCfg) {
	if r == nil {
		return
	}
	dedup := func(s []string) []string {
		seen := make(map[string]struct{}, len(s))
		res := make([]string, 0, len(s))
		for _, v := range s {
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				res = append(res, v)
			}
		}
		return res
	}
	r.IncludeRanges = dedup(r.IncludeRanges)
	r.ExcludeRanges = dedup(r.ExcludeRanges)
	r.IncludeIPs = dedup(r.IncludeIPs)
	r.ExcludeIPs = dedup(r.ExcludeIPs)
	r.IncludeDomains = dedup(r.IncludeDomains)
	r.ExcludeDomains = dedup(r.ExcludeDomains)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
