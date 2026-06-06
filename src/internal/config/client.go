package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/go-viper/mapstructure/v2"
	"go.yaml.in/yaml/v3"
)

// @sk-task foundation#T2.3: client config struct (AC-006)
// @sk-task performance-and-polish#T1.1: add Compression, Multiplex fields (AC-006, AC-007)
// @sk-task local-proxy-mode#T1.1: add Mode, ProxyListen, ProxyAuth fields (AC-003, AC-004)
// @sk-task app-crypto#T3: add Crypto config (AC-006)
// @sk-task quic-transport#T1.2: add Transport field (AC-001, AC-004)
// @sk-task quic-obfuscation#T2.1: add Obfuscation field (AC-001)
type ClientConfig struct {
	Server        string         `json:"server" mapstructure:"server"`
	Transport     string         `json:"transport" mapstructure:"transport"`
	Obfuscation   bool           `json:"obfuscation" mapstructure:"obfuscation"`
	Auth          AuthCfg        `json:"auth" mapstructure:"auth"`
	TLS           ClientTLSCfg   `json:"tls" mapstructure:"tls"`
	MTU           int            `json:"mtu" mapstructure:"mtu"`
	IPv6          bool           `json:"ipv6" mapstructure:"ipv6"`
	AutoReconnect *bool          `json:"auto_reconnect" mapstructure:"auto_reconnect"`
	Log           LogConfig      `json:"log" mapstructure:"log"`
	Routing       *RoutingCfg    `json:"routing" mapstructure:"routing"`
	KillSwitch    *KillSwitchCfg `json:"kill_switch" mapstructure:"kill_switch"`
	Reconnect     *ReconnectCfg  `json:"reconnect" mapstructure:"reconnect"`
	Compression   bool           `json:"compression" mapstructure:"compression"`
	Multiplex     bool           `json:"multiplex" mapstructure:"multiplex"`
	Mode          string         `json:"mode" mapstructure:"mode"`
	ProxyListen   string         `json:"proxy_listen" mapstructure:"proxy_listen"`
	ProxyAuth     *ProxyAuthCfg  `json:"proxy_auth" mapstructure:"proxy_auth"`
	Crypto        CryptoCfg      `json:"crypto" mapstructure:"crypto"`
}

// @sk-task production-gap#T1.1: explicit client TLS trust surface (AC-001)
type ClientTLSCfg struct {
	CAFile     string `json:"ca_file" mapstructure:"ca_file"`
	ServerName string `json:"server_name" mapstructure:"server_name"`
	VerifyMode string `json:"verify_mode" mapstructure:"verify_mode"`
}

type ProxyAuthCfg struct {
	Username string `json:"username" mapstructure:"username"`
	Password string `json:"password" mapstructure:"password"`
}

// @sk-task routing-split-tunnel#T1.1: routing config struct (AC-009)
type RoutingCfg struct {
	DefaultRoute   string   `json:"default_route" mapstructure:"default_route"`
	IncludeRanges  []string `json:"include_ranges" mapstructure:"include_ranges"`
	ExcludeRanges  []string `json:"exclude_ranges" mapstructure:"exclude_ranges"`
	IncludeIPs     []string `json:"include_ips" mapstructure:"include_ips"`
	ExcludeIPs     []string `json:"exclude_ips" mapstructure:"exclude_ips"`
	IncludeDomains []string `json:"include_domains" mapstructure:"include_domains"`
	ExcludeDomains []string `json:"exclude_domains" mapstructure:"exclude_domains"`
}

// @sk-task production-hardening#T1.1: kill switch config (AC-003)
type KillSwitchCfg struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"`
}

// @sk-task production-hardening#T1.1: reconnect config (AC-001)
type ReconnectCfg struct {
	MinBackoffSec int `json:"min_backoff_sec" mapstructure:"min_backoff_sec"`
	MaxBackoffSec int `json:"max_backoff_sec" mapstructure:"max_backoff_sec"`
}

type AuthCfg struct {
	Token string `json:"token" mapstructure:"token"`
}

// @sk-task routing-split-tunnel#T2.1: load config with routing defaults (AC-009)
// @sk-task production-readiness-gap#T1: secrets management — warn when secrets in YAML (AC-001)
func LoadClientConfig(path string) (*ClientConfig, error) {
	cfg := &ClientConfig{}
	if err := load(path, "KVN_CLIENT", cfg); err != nil {
		return nil, err
	}
	if cfg.MTU == 0 {
		cfg.MTU = 1400
	}
	if cfg.ProxyListen == "" {
		cfg.ProxyListen = "127.0.0.1:2310"
	}
	if cfg.AutoReconnect == nil {
		v := true
		cfg.AutoReconnect = &v
	}
	if cfg.Mode == "" {
		cfg.Mode = "proxy"
	}
	// @sk-task production-gap#T1.1: default to trusted client verification (AC-001)
	if cfg.TLS.VerifyMode == "" {
		cfg.TLS.VerifyMode = "verify"
	}
	if cfg.Routing == nil {
		cfg.Routing = &RoutingCfg{
			DefaultRoute:  "server",
			ExcludeRanges: DefaultExcludeRanges,
		}
	} else if cfg.Routing.DefaultRoute == "" {
		cfg.Routing.DefaultRoute = "server"
	}
	// deduplicate and ensure all DefaultExcludeRanges are present
	unique := make([]string, 0, len(cfg.Routing.ExcludeRanges)+len(DefaultExcludeRanges))
	seen := make(map[string]bool, len(cfg.Routing.ExcludeRanges))
	for _, r := range cfg.Routing.ExcludeRanges {
		if !seen[r] {
			unique = append(unique, r)
			seen[r] = true
		}
	}
	for _, d := range DefaultExcludeRanges {
		if !seen[d] {
			unique = append([]string{d}, unique...)
			seen[d] = true
		}
	}
	cfg.Routing.ExcludeRanges = unique
	if cfg.Crypto.Enabled && cfg.Crypto.Key == "" {
		cfg.Crypto.Enabled = false
	}

	// @sk-task production-readiness-gap#T1: warn when secrets come from config file (AC-001)
	secretKeys := []string{"auth.token", "crypto.key"}
	if cfg.ProxyAuth != nil {
		secretKeys = append(secretKeys, "proxy_auth.username", "proxy_auth.password")
	}
	if warnSecretInFile(secretKeys) {
		log.Println("[config] WARNING: secrets (auth.token, crypto.key) loaded from config file. Use environment variables KVN_CLIENT_* for production.")
	}
	return cfg, nil
}

// @sk-task kvn-web#T1.1: SaveClientConfig writes config to YAML (AC-005)
// @sk-task quic-obfuscation#T3.3: normalize routing defaults on save (AC-001)
func SaveClientConfig(path string, cfg *ClientConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	if cfg.Routing == nil {
		cfg.Routing = &RoutingCfg{
			DefaultRoute:  "server",
			ExcludeRanges: DefaultExcludeRanges,
		}
	} else {
		// deduplicate and ensure all DefaultExcludeRanges are present
		unique := make([]string, 0, len(cfg.Routing.ExcludeRanges)+len(DefaultExcludeRanges))
		seen := make(map[string]bool, len(cfg.Routing.ExcludeRanges))
		for _, r := range cfg.Routing.ExcludeRanges {
			if !seen[r] {
				unique = append(unique, r)
				seen[r] = true
			}
		}
		for _, d := range DefaultExcludeRanges {
			if !seen[d] {
				unique = append([]string{d}, unique...)
				seen[d] = true
			}
		}
		cfg.Routing.ExcludeRanges = unique
	}

	var m map[string]interface{}
	if err := mapstructure.Decode(cfg, &m); err != nil {
		return err
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

var DefaultExcludeRanges = []string{
	"127.0.0.0/8",
	"::1/128",
	"224.0.0.0/4",
	"239.0.0.0/8",
	"ff00::/8",
	"255.255.255.255/32",
	"169.254.0.0/16",
	"fe80::/10",
}
