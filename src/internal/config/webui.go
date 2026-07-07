package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// @sk-task multi-server#T1.1: ServerEntry type (AC-001)
type ServerEntry struct {
	Name         string `json:"name" yaml:"name"`
	ClientConfig `json:",inline" yaml:",inline"`
}

// @sk-task dns-upstreams-list#T1.3: defaultWebUIConfig Upstreams (AC-003)
// @sk-task multi-server#T1.1: WebUIConfig type (AC-001)
type WebUIConfig struct {
	ClientConfig `json:",inline" yaml:",inline"`
	ActiveServer string        `json:"active_server" yaml:"active_server"`
	Servers      []ServerEntry `json:"servers" yaml:"servers"`
}

// @sk-task multi-server#T1.1: LoadWebUIConfig (AC-001)
func LoadWebUIConfig(path string) (*WebUIConfig, error) {
	dir := filepath.Dir(path)
	name := filepath.Base(path)
	root := os.DirFS(dir)
	data, err := fs.ReadFile(root, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
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

	// backward compat: routing.dns_cache → routing.dns_routing in YAML
	migrateDNSCompatYAML(data, cfg)

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

// migrateDNSCompatYAML migrates old routing.dns_cache to routing.dns_routing in YAML configs
func migrateDNSCompatYAML(rawYAML []byte, cfg *WebUIConfig) {
	// quick check: if global routing already has DNSRouting and all servers too, skip
	if cfg.Routing != nil && cfg.Routing.DNSRouting != nil {
		allSet := true
		for i := range cfg.Servers {
			if cfg.Servers[i].Routing != nil && cfg.Servers[i].Routing.DNSRouting == nil {
				allSet = false
				break
			}
		}
		if allSet {
			return
		}
	}

	var rawMap map[string]interface{}
	if err := yaml.Unmarshal(rawYAML, &rawMap); err != nil {
		return
	}

	readOldDNS := func(raw map[string]interface{}, r *RoutingCfg) {
		if r == nil || r.DNSRouting != nil {
			return
		}
		if old, ok := raw["dns_cache"]; ok {
			buf, err := json.Marshal(old)
			if err != nil {
				return
			}
			var dnsCfg DNSRoutingCfg
			if json.Unmarshal(buf, &dnsCfg) == nil {
				r.DNSRouting = &dnsCfg
			}
		}
	}

	if rRaw, ok := rawMap["routing"].(map[string]interface{}); ok {
		readOldDNS(rRaw, cfg.Routing)
	}
	if serversRaw, ok := rawMap["servers"].([]interface{}); ok {
		for i, sRaw := range serversRaw {
			if i >= len(cfg.Servers) {
				break
			}
			if sMap, ok := sRaw.(map[string]interface{}); ok {
				if rRaw, ok := sMap["routing"].(map[string]interface{}); ok {
					readOldDNS(rRaw, cfg.Servers[i].Routing)
				}
			}
		}
	}
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
			DNSProxy: DNSProxyCfg{
				Listen:    "127.0.0.54:53",
				Upstreams: []string{"1.1.1.1:53"},
			},
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
