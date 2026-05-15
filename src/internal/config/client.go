package config

import "log"

// @sk-task foundation#T2.3: client config struct (AC-006)
// @sk-task performance-and-polish#T1.1: add Compression, Multiplex fields (AC-006, AC-007)
// @sk-task local-proxy-mode#T1.1: add Mode, ProxyListen, ProxyAuth fields (AC-003, AC-004)
// @sk-task app-crypto#T3: add Crypto config (AC-006)
type ClientConfig struct {
	Server        string         `mapstructure:"server"`
	Auth          AuthCfg        `mapstructure:"auth"`
	TLS           ClientTLSCfg   `mapstructure:"tls"`
	MTU           int            `mapstructure:"mtu"`
	IPv6          bool           `mapstructure:"ipv6"`
	AutoReconnect bool           `mapstructure:"auto_reconnect"`
	Log           LogConfig      `mapstructure:"log"`
	Routing       *RoutingCfg    `mapstructure:"routing"`
	KillSwitch    *KillSwitchCfg `mapstructure:"kill_switch"`
	Reconnect     *ReconnectCfg  `mapstructure:"reconnect"`
	Compression   bool           `mapstructure:"compression"`
	Multiplex     bool           `mapstructure:"multiplex"`
	Mode          string         `mapstructure:"mode"`
	ProxyListen   string         `mapstructure:"proxy_listen"`
	ProxyAuth     *ProxyAuthCfg  `mapstructure:"proxy_auth"`
	Crypto        CryptoCfg      `mapstructure:"crypto"`
}

// @sk-task production-gap#T1.1: explicit client TLS trust surface (AC-001)
type ClientTLSCfg struct {
	CAFile     string `mapstructure:"ca_file"`
	ServerName string `mapstructure:"server_name"`
	VerifyMode string `mapstructure:"verify_mode"`
}

type ProxyAuthCfg struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// @sk-task routing-split-tunnel#T1.1: routing config struct (AC-009)
type RoutingCfg struct {
	DefaultRoute   string   `mapstructure:"default_route"`
	IncludeRanges  []string `mapstructure:"include_ranges"`
	ExcludeRanges  []string `mapstructure:"exclude_ranges"`
	IncludeIPs     []string `mapstructure:"include_ips"`
	ExcludeIPs     []string `mapstructure:"exclude_ips"`
	IncludeDomains []string `mapstructure:"include_domains"`
	ExcludeDomains []string `mapstructure:"exclude_domains"`
}

// @sk-task production-hardening#T1.1: kill switch config (AC-003)
type KillSwitchCfg struct {
	Enabled bool `mapstructure:"enabled"`
}

// @sk-task production-hardening#T1.1: reconnect config (AC-001)
type ReconnectCfg struct {
	MinBackoffSec int `mapstructure:"min_backoff_sec"`
	MaxBackoffSec int `mapstructure:"max_backoff_sec"`
}

type AuthCfg struct {
	Token string `mapstructure:"token"`
}

// @sk-task routing-split-tunnel#T2.1: load config with routing defaults (AC-009)
// @sk-task production-readiness-gap#T1: secrets management — warn when secrets in YAML (AC-001)
func LoadClientConfig(path string) (*ClientConfig, error) {
	cfg := &ClientConfig{}
	if err := load(path, "KVN_CLIENT", cfg); err != nil {
		return nil, err
	}
	// @sk-task production-gap#T1.1: default to trusted client verification (AC-001)
	if cfg.TLS.VerifyMode == "" {
		cfg.TLS.VerifyMode = "verify"
	}
	if cfg.Routing == nil {
		cfg.Routing = &RoutingCfg{DefaultRoute: "server"}
	} else if cfg.Routing.DefaultRoute == "" {
		cfg.Routing.DefaultRoute = "server"
	}
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
