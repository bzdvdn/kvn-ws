package config

// @sk-task foundation#T2.3: server config struct (AC-007)
type ServerConfig struct {
	Listen       string         `mapstructure:"listen"`
	TLS          TLSCfg         `mapstructure:"tls"`
	Network      NetworkCfg     `mapstructure:"network"`
	Session      SessionCfg     `mapstructure:"session"`
	Auth         ServerAuth     `mapstructure:"auth"`
	Logging      LogConfig      `mapstructure:"logging"`
	RateLimiting RateLimitCfg   `mapstructure:"rate_limiting"`
	BoltDBPath   string         `mapstructure:"bolt_db_path"`
}

type TLSCfg struct {
	Cert       string `mapstructure:"cert"`
	Key        string `mapstructure:"key"`
	MinVersion string `mapstructure:"min_version"`
}

type NetworkCfg struct {
	PoolIPv4 PoolCfg `mapstructure:"pool_ipv4"`
}

type PoolCfg struct {
	Subnet     string `mapstructure:"subnet"`
	Gateway    string `mapstructure:"gateway"`
	RangeStart string `mapstructure:"range_start"`
	RangeEnd   string `mapstructure:"range_end"`
}

// @sk-task production-hardening#T1.1: rate limit config (AC-004)
type RateLimitCfg struct {
	AuthBurst      int `mapstructure:"auth_burst"`
	AuthPerMinute  int `mapstructure:"auth_per_minute"`
	PacketsPerSec  int `mapstructure:"packets_per_sec"`
}

// @sk-task production-hardening#T1.1: session expiry config (AC-005)
type SessionExpiryCfg struct {
	SessionTTLSec   int `mapstructure:"session_ttl_sec"`
	ReclaimInterval int `mapstructure:"reclaim_interval_sec"`
}

type SessionCfg struct {
	MaxClients     int               `mapstructure:"max_clients"`
	IdleTimeoutSec int               `mapstructure:"idle_timeout_sec"`
	Expiry         *SessionExpiryCfg `mapstructure:"expiry"`
}

type ServerAuth struct {
	Mode   string   `mapstructure:"mode"`
	Tokens []string `mapstructure:"tokens"`
}

func LoadServerConfig(path string) (*ServerConfig, error) {
	cfg := &ServerConfig{}
	if err := load(path, "KVN_SERVER", cfg); err != nil {
		return nil, err
	}
	if cfg.RateLimiting.AuthBurst == 0 {
		cfg.RateLimiting.AuthBurst = 5
	}
	if cfg.RateLimiting.AuthPerMinute == 0 {
		cfg.RateLimiting.AuthPerMinute = 1
	}
	if cfg.RateLimiting.PacketsPerSec == 0 {
		cfg.RateLimiting.PacketsPerSec = 1000
	}
	return cfg, nil
}
