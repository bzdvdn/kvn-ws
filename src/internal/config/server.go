package config

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/viper"
)

// @sk-task foundation#T2.3: server config struct (AC-007)
// @sk-task security-acl#T1: TokenCfg structured config
// @sk-task performance-and-polish#T1.1: add Compression, Multiplex, MTU fields (AC-004, AC-006, AC-007)
// @sk-task app-crypto#T3: add Crypto config (AC-006)
type ServerConfig struct {
	Listen       string       `mapstructure:"listen"`
	TLS          TLSCfg       `mapstructure:"tls"`
	Network      NetworkCfg   `mapstructure:"network"`
	Session      SessionCfg   `mapstructure:"session"`
	Auth         ServerAuth   `mapstructure:"auth"`
	Logging      LogConfig    `mapstructure:"logging"`
	RateLimiting RateLimitCfg `mapstructure:"rate_limiting"`
	BoltDBPath   string       `mapstructure:"bolt_db_path"`
	ACL          ACLCfg       `mapstructure:"acl"`
	Origin       OriginCfg    `mapstructure:"origin"`
	Admin        AdminCfg     `mapstructure:"admin"`
	Compression  bool         `mapstructure:"compression"`
	Multiplex    bool         `mapstructure:"multiplex"`
	MTU          int          `mapstructure:"mtu"`
	Crypto       CryptoCfg    `mapstructure:"crypto"`
}

type CryptoCfg struct {
	Enabled bool   `mapstructure:"enabled"`
	Key     string `mapstructure:"key"`
}

// @sk-task security-acl#T1: TLSCfg extended with mTLS fields
type TLSCfg struct {
	Cert         string `mapstructure:"cert"`
	Key          string `mapstructure:"key"`
	ClientCAFile string `mapstructure:"client_ca_file"`
	ClientAuth   string `mapstructure:"client_auth"`
}

// @sk-task ipv6-dual-stack#T1.1: add PoolIPv6 config field (AC-001, AC-002)
type NetworkCfg struct {
	PoolIPv4 PoolCfg `mapstructure:"pool_ipv4"`
	PoolIPv6 PoolCfg `mapstructure:"pool_ipv6"`
}

type PoolCfg struct {
	Subnet     string `mapstructure:"subnet"`
	Gateway    string `mapstructure:"gateway"`
	RangeStart string `mapstructure:"range_start"`
	RangeEnd   string `mapstructure:"range_end"`
}

// @sk-task production-hardening#T1.1: rate limit config (AC-004)
type RateLimitCfg struct {
	AuthBurst     int `mapstructure:"auth_burst"`
	AuthPerMinute int `mapstructure:"auth_per_minute"`
	PacketsPerSec int `mapstructure:"packets_per_sec"`
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
// @sk-task production-readiness-hardening#T3.1: all type assertions use ok (already safe) (AC-007)
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

// @sk-task production-readiness-gap#T1: secrets management — env override with YAML warning (AC-001)
func LoadServerConfig(path string) (*ServerConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("KVN_SERVER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	envPrefixForWarning = "KVN_SERVER"

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &ServerConfig{}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config %s: %w", path, err)
	}

	if rawTokens, ok := v.Get("auth.tokens").([]interface{}); ok && len(cfg.Auth.Tokens) == 0 {
		cfg.Auth.Tokens = convertRawTokens(rawTokens)
	}

	// @sk-task production-readiness-gap#T1: load tokens from KVN_SERVER_AUTH_TOKENS_JSON env var (AC-001)
	if envTokens, ok := loadTokensFromEnvJSON("KVN_SERVER"); ok && len(envTokens) > 0 {
		cfg.Auth.Tokens = envTokens
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
	if err := normalizeTLS(cfg); err != nil {
		return nil, err
	}
	normalizeTokens(cfg)
	if cfg.Crypto.Enabled && cfg.Crypto.Key == "" {
		return nil, fmt.Errorf("crypto.key is required when crypto.enabled is true")
	}
	// @sk-task production-readiness-gap#T1: warn when secrets come from config file (AC-001)
	secretKeys := []string{"auth.tokens", "crypto.key", "admin.token"}
	if warnSecretInFile(secretKeys) {
		log.Println("[config] WARNING: secrets (auth.tokens, crypto.key, admin.token) loaded from config file. Use environment variables KVN_SERVER_* for production.")
	}
	return cfg, nil
}

// @sk-task production-gap#T1.2: normalize server mTLS semantics (AC-002)
func normalizeTLS(cfg *ServerConfig) error {
	if cfg.TLS.ClientAuth == "" {
		if cfg.TLS.ClientCAFile != "" {
			cfg.TLS.ClientAuth = "verify"
		}
		return nil
	}

	switch cfg.TLS.ClientAuth {
	case "request":
		return nil
	case "require", "verify":
		if cfg.TLS.ClientCAFile == "" {
			return fmt.Errorf("tls.client_ca_file is required when tls.client_auth=%q", cfg.TLS.ClientAuth)
		}
		return nil
	default:
		return fmt.Errorf("unsupported tls.client_auth %q", cfg.TLS.ClientAuth)
	}
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
