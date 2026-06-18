package config

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// @sk-task multi-server#T1.1: ServerEntry type (AC-001)
type ServerEntry struct {
	Name         string `json:"name" yaml:"name"`
	ClientConfig `json:",inline" yaml:",inline"`
}

// @sk-task multi-server#T1.1: WebUIConfig type (AC-001)
type WebUIConfig struct {
	ClientConfig `json:",inline" yaml:",inline"`
	ActiveServer string        `json:"active_server" yaml:"active_server"`
	Servers      []ServerEntry `json:"servers" yaml:"servers"`
}

// @sk-task multi-server#T1.1: LoadWebUIConfig (AC-001)
func LoadWebUIConfig(path string) (*WebUIConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultWebUIConfig(), nil
		}
		return nil, fmt.Errorf("read webui config %s: %w", path, err)
	}

	cfg := &WebUIConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse webui config %s: %w", path, err)
	}

	// @sk-task multi-server#T1.2: migration — wrap existing config into servers[0] (AC-001)
	if len(cfg.Servers) == 0 {
		cfg.Servers = []ServerEntry{{
			Name:         "Default",
			ClientConfig: cfg.ClientConfig,
		}}
	}

	if cfg.ActiveServer == "" && len(cfg.Servers) > 0 {
		cfg.ActiveServer = cfg.Servers[0].Name
	}

	return cfg, nil
}

// @sk-task multi-server#T1.1: SaveWebUIConfig (AC-001)
func SaveWebUIConfig(path string, cfg *WebUIConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal webui config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write webui config %s: %w", path, err)
	}

	return nil
}

func defaultWebUIConfig() *WebUIConfig {
	return &WebUIConfig{
		ClientConfig: ClientConfig{
			Server:    "",
			Transport: "quic",
			MTU:       1400,
			Mode:      "proxy",
			Log: LogConfig{
				Level: "info",
			},
			ProxyListen: "127.0.0.1:2310",
			TLS: ClientTLSCfg{
				VerifyMode: "verify",
			},
			MaxMessageSize: 10 * 1024 * 1024,
			TunnelTimeout:  30,
			Routing: &RoutingCfg{
				DefaultRoute:  "server",
				ExcludeRanges: DefaultExcludeRanges,
			},
		},
		ActiveServer: "Default",
		Servers: []ServerEntry{
			{
				Name: "Default",
				ClientConfig: ClientConfig{
					Server:    "",
					Transport: "quic",
				},
			},
		},
	}
}
