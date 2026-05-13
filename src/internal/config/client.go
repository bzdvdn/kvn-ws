package config

// @sk-task foundation#T2.3: client config struct (AC-006)
type ClientConfig struct {
	Server        string        `mapstructure:"server"`
	Auth          AuthCfg       `mapstructure:"auth"`
	MTU           int           `mapstructure:"mtu"`
	IPv6          bool          `mapstructure:"ipv6"`
	AutoReconnect bool          `mapstructure:"auto_reconnect"`
	Log           LogConfig     `mapstructure:"log"`
	Routing       *RoutingCfg   `mapstructure:"routing"`
	KillSwitch    *KillSwitchCfg `mapstructure:"kill_switch"`
	Reconnect     *ReconnectCfg  `mapstructure:"reconnect"`
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
func LoadClientConfig(path string) (*ClientConfig, error) {
	cfg := &ClientConfig{}
	if err := load(path, "KVN_CLIENT", cfg); err != nil {
		return nil, err
	}
	if cfg.Routing == nil {
		cfg.Routing = &RoutingCfg{DefaultRoute: "server"}
	} else if cfg.Routing.DefaultRoute == "" {
		cfg.Routing.DefaultRoute = "server"
	}
	return cfg, nil
}
