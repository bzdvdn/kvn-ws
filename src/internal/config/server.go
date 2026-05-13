package config

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/viper"
)

// @sk-task foundation#T2.3: server config struct (AC-007)
// @sk-task security-acl#T1: TokenCfg structured config
type ServerConfig struct {
	Listen       string         `mapstructure:"listen"`
	TLS          TLSCfg         `mapstructure:"tls"`
	Network      NetworkCfg     `mapstructure:"network"`
	Session      SessionCfg     `mapstructure:"session"`
	Auth         ServerAuth     `mapstructure:"auth"`
	Logging      LogConfig      `mapstructure:"logging"`
	RateLimiting RateLimitCfg   `mapstructure:"rate_limiting"`
	BoltDBPath   string         `mapstructure:"bolt_db_path"`
	ACL          ACLCfg         `mapstructure:"acl"`
	Origin       OriginCfg      `mapstructure:"origin"`
	Admin        AdminCfg       `mapstructure:"admin"`
}

// @sk-task security-acl#T1: TLSCfg extended with mTLS fields
type TLSCfg struct {
	Cert         string `mapstructure:"cert"`
	Key          string `mapstructure:"key"`
	MinVersion   string `mapstructure:"min_version"`
	ClientCAFile string `mapstructure:"client_ca_file"`
	ClientAuth   string `mapstructure:"client_auth"`
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

// @sk-task security-acl#T1: TokenCfg with per-token limits
type TokenCfg struct {
	Name         string `mapstructure:"name"`
	Secret       string `mapstructure:"secret"`
	BandwidthBPS int    `mapstructure:"bandwidth_bps"`
	MaxSessions  int    `mapstructure:"max_sessions"`
}

// @sk-task security-acl#T1: ServerAuth with structured tokens
type ServerAuth struct {
	Mode   string     `mapstructure:"mode"`
	Tokens []TokenCfg `mapstructure:"tokens"`
}

// @sk-task security-acl#T2: ACLCfg for CIDR allow/deny lists
type ACLCfg struct {
	DenyCIDRs  []string `mapstructure:"deny_cidrs"`
	AllowCIDRs []string `mapstructure:"allow_cidrs"`
}

// @sk-task security-acl#T4: OriginCfg for Origin/Referer validation
type OriginCfg struct {
	Whitelist  []string `mapstructure:"whitelist"`
	AllowEmpty bool     `mapstructure:"allow_empty"`
}

// @sk-task security-acl#T8: AdminCfg for Admin API
type AdminCfg struct {
	Enabled bool   `mapstructure:"enabled"`
	Listen  string `mapstructure:"listen"`
	Token   string `mapstructure:"token"`
}

// @sk-task security-acl#T12: backward-compat token parsing from viper raw value
func convertRawTokens(raw interface{}) []TokenCfg {
	switch v := raw.(type) {
	case []interface{}:
		result := make([]TokenCfg, 0, len(v))
		for _, item := range v {
			switch item := item.(type) {
			case string:
				result = append(result, TokenCfg{
					Name:         item,
					Secret:       item,
					BandwidthBPS: 0,
					MaxSessions:  0,
				})
			case map[string]interface{}:
				tc := TokenCfg{}
				if name, ok := item["name"].(string); ok {
					tc.Name = name
				}
				if secret, ok := item["secret"].(string); ok {
					tc.Secret = secret
				}
				if bw, ok := item["bandwidth_bps"].(int); ok {
					tc.BandwidthBPS = bw
				}
				if ms, ok := item["max_sessions"].(int); ok {
					tc.MaxSessions = ms
				}
				result = append(result, tc)
			}
		}
		return result
	case []map[string]interface{}:
		result := make([]TokenCfg, 0, len(v))
		for _, item := range v {
			tc := TokenCfg{}
			if name, ok := item["name"].(string); ok {
				tc.Name = name
			}
			if secret, ok := item["secret"].(string); ok {
				tc.Secret = secret
			}
			if bw, ok := item["bandwidth_bps"].(int); ok {
				tc.BandwidthBPS = bw
			}
			if ms, ok := item["max_sessions"].(int); ok {
				tc.MaxSessions = ms
			}
			result = append(result, tc)
		}
		return result
	}
	return nil
}

// @sk-task security-acl#T12: load config with backward-compat token parsing
func LoadServerConfig(path string) (*ServerConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("KVN_SERVER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &ServerConfig{}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config %s: %w", path, err)
	}

	rawTokens := v.Get("auth.tokens")
	if rawTokens != nil {
		if _, ok := rawTokens.([]interface{}); ok {
			if len(cfg.Auth.Tokens) == 0 {
				cfg.Auth.Tokens = convertRawTokens(rawTokens)
			}
		}
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
	if cfg.Admin.Listen == "" {
		cfg.Admin.Listen = "localhost:8443"
	}
	normalizeTokens(cfg)
	return cfg, nil
}

func normalizeTokens(cfg *ServerConfig) {
	for i, t := range cfg.Auth.Tokens {
		if t.Name == "" {
			cfg.Auth.Tokens[i].Name = fmt.Sprintf("token-%d", i)
		}
		if t.Secret == "" {
			cfg.Auth.Tokens[i].Secret = cfg.Auth.Tokens[i].Name
		}
	}
	log.Printf("[config] loaded %d tokens", len(cfg.Auth.Tokens))
}
