package webui

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
)

// @sk-task kvn-web#T2.1: config API handlers (AC-005, AC-007)
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfgPath := filepath.Join(s.configDir, "config.yaml")
	cfg, err := config.LoadClientConfig(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusOK, configResponse{Config: defaultConfig()})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, configResponse{Config: cfg})
}

func boolPtr(b bool) *bool { return &b }

func defaultConfig() *config.ClientConfig {
	autoReconnect := true
	return &config.ClientConfig{
		MTU:                 1400,
		ProxyListen:         "127.0.0.1:2310",
		AutoReconnect:       &autoReconnect,
		Log:                 config.LogConfig{Level: "info"},
		Mode:                "proxy",
		Transport:           "quic",
		Obfuscation:         &config.ObfuscationCfg{Enabled: true},
		TLS:                 config.ClientTLSCfg{VerifyMode: "verify"},
		MaxMessageSize:      10 * 1024 * 1024,
		TunnelTimeout:       30,
		ProxyMaxConcurrency: 1000,
		SystemProxy:         boolPtr(false),
		Transparent:         false,
		DNSProxy:            config.DNSProxyCfg{Listen: "127.0.0.53:53"},
		Routing: &config.RoutingCfg{
			DefaultRoute:  "server",
			ExcludeRanges: append([]string{}, config.DefaultExcludeRanges...),
		},
	}
}

type configResponse struct {
	Config *config.ClientConfig `json:"config"`
}

type configSaveRequest struct {
	Config *config.ClientConfig `json:"config"`
}

func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req configSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	cfgPath := filepath.Join(s.configDir, "config.yaml")
	if err := config.SaveClientConfig(cfgPath, req.Config); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
