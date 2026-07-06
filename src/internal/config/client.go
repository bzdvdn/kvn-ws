package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// @sk-task foundation#T2.3: client config struct (AC-006)
// @sk-task performance-and-polish#T1.1: add Multiplex fields (AC-006, AC-007)
// @sk-task local-proxy-mode#T1.1: add Mode, ProxyListen, ProxyAuth fields (AC-003, AC-004)
// @sk-task app-crypto#T3: add Crypto config (AC-006)
// @sk-task quic-transport#T1.2: add Transport field (AC-001, AC-004)
// @sk-task quic-obfuscation#T2.1: add Obfuscation field (AC-001)
// @sk-task whitelist-obfuscation#T1.1: Obfuscation bool → struct (AC-001)
// @sk-task arch-refactoring#T1.1: add MaxMessageSize, TunnelTimeout, ProxyMaxConcurrency fields (AC-006)
// @sk-task system-proxy#T1.2: add SystemProxy field (AC-001)
// @sk-task transparent-proxy#T1.1: add Transparent and DNSProxyCfg fields (AC-001, AC-008, AC-009)
// @sk-task client-relay-mode#T1.1: add Relay field (AC-003)
type ClientConfig struct {
	Server              string          `json:"server" mapstructure:"server"`
	Transport           string          `json:"transport" mapstructure:"transport"`
	Obfuscation         *ObfuscationCfg `json:"obfuscation,omitempty" mapstructure:"obfuscation"`
	Auth                AuthCfg         `json:"auth" mapstructure:"auth"`
	TLS                 ClientTLSCfg    `json:"tls" mapstructure:"tls"`
	MTU                 int             `json:"mtu" mapstructure:"mtu"`
	IPv6                bool            `json:"ipv6" mapstructure:"ipv6"`
	AutoReconnect       *bool           `json:"auto_reconnect" mapstructure:"auto_reconnect"`
	Log                 LogConfig       `json:"log" mapstructure:"log"`
	Routing             *RoutingCfg     `json:"routing" mapstructure:"routing"`
	KillSwitch          *KillSwitchCfg  `json:"kill_switch" mapstructure:"kill_switch"`
	Reconnect           *ReconnectCfg   `json:"reconnect" mapstructure:"reconnect"`
	Multiplex           bool            `json:"multiplex" mapstructure:"multiplex"`
	Mode                string          `json:"mode" mapstructure:"mode"`
	ProxyListen         string          `json:"proxy_listen" mapstructure:"proxy_listen"`
	ProxyAuth           *ProxyAuthCfg   `json:"proxy_auth" mapstructure:"proxy_auth"`
	Crypto              CryptoCfg       `json:"crypto" mapstructure:"crypto"`
	MaxMessageSize      int             `json:"max_message_size" mapstructure:"max_message_size"`
	TunnelTimeout       int             `json:"tunnel_timeout" mapstructure:"tunnel_timeout"`
	ProxyMaxConcurrency int             `json:"proxy_max_concurrency" mapstructure:"proxy_max_concurrency"`
	ProxyConnections    int             `json:"proxy_connections" mapstructure:"proxy_connections"`
	SystemProxy         *bool           `json:"system_proxy" mapstructure:"system_proxy"`
	Transparent         bool            `json:"transparent" mapstructure:"transparent"`
	DNSProxy            DNSProxyCfg     `json:"dns_proxy" mapstructure:"dns_proxy"`
	Relay               *RelayCfg       `json:"relay,omitempty" mapstructure:"relay"`
}

// @sk-task client-relay-mode#T1.1: relay config struct (AC-003)
// @sk-task quic-relay-mode#T1.1: add Quic field (AC-003)
type RelayCfg struct {
	Listen         string        `json:"listen" mapstructure:"listen"`
	WSPaths        []string      `json:"ws_paths,omitempty" mapstructure:"ws_paths"`
	MaxConnections int           `json:"max_connections" mapstructure:"max_connections"`
	TLS            *RelayTLSCfg  `json:"tls,omitempty" mapstructure:"tls"`
	Quic           *RelayQuicCfg `json:"quic,omitempty" mapstructure:"quic"`
}

// @sk-task client-relay-mode#T1.1: relay TLS config (AC-003)
type RelayTLSCfg struct {
	Cert string `json:"cert" mapstructure:"cert"`
	Key  string `json:"key" mapstructure:"key"`
}

// @sk-task quic-relay-mode#T1.1: QUIC relay config (AC-003)
type RelayQuicCfg struct {
	KeepAlive   int `json:"keep_alive" mapstructure:"keep_alive"`
	IdleTimeout int `json:"idle_timeout" mapstructure:"idle_timeout"`
}

// @sk-task dns-upstreams-list#T1.1: DNSProxyCfg with Upstreams []string + backward compat (AC-001, AC-002, AC-003)
type DNSProxyCfg struct {
	Listen    string   `json:"listen" mapstructure:"listen"`
	Upstreams []string `json:"upstreams,omitempty" mapstructure:"upstreams"`
}

// @sk-task dns-upstreams-list#T1.1: MarshalJSON — output only upstreams (AC-002)
func (d DNSProxyCfg) MarshalJSON() ([]byte, error) {
	m := map[string]interface{}{"listen": d.Listen}
	if len(d.Upstreams) > 0 {
		m["upstreams"] = d.Upstreams
	}
	return json.Marshal(m)
}

// @sk-task dns-upstreams-list#T1.1: UnmarshalJSON — accept both upstream and upstreams (AC-002)
func (d *DNSProxyCfg) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if listen, ok := raw["listen"].(string); ok {
		d.Listen = listen
	}
	if upstreams, ok := raw["upstreams"].([]interface{}); ok {
		for _, u := range upstreams {
			if s, ok := u.(string); ok {
				d.Upstreams = append(d.Upstreams, s)
			}
		}
	} else if upstream, ok := raw["upstream"].(string); ok && upstream != "" {
		d.Upstreams = []string{upstream}
	}
	return nil
}

// @sk-task dns-upstreams-list#T1.1: MarshalYAML — output only upstreams (AC-002)
func (d DNSProxyCfg) MarshalYAML() (interface{}, error) {
	m := map[string]interface{}{"listen": d.Listen}
	if len(d.Upstreams) > 0 {
		m["upstreams"] = d.Upstreams
	}
	return m, nil
}

// @sk-task dns-upstreams-list#T1.1: UnmarshalYAML — accept both upstream and upstreams (AC-002)
func (d *DNSProxyCfg) UnmarshalYAML(value *yaml.Node) error {
	var raw map[string]interface{}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	if listen, ok := raw["listen"].(string); ok {
		d.Listen = listen
	}
	if upstreams, ok := raw["upstreams"].([]interface{}); ok {
		for _, u := range upstreams {
			if s, ok := u.(string); ok {
				d.Upstreams = append(d.Upstreams, s)
			}
		}
	} else if upstream, ok := raw["upstream"].(string); ok && upstream != "" {
		d.Upstreams = []string{upstream}
	}
	return nil
}

type ObfuscationCfg struct {
	Enabled bool        `json:"enabled" mapstructure:"enabled"`
	UTLS    *UTLSCfg    `json:"utls,omitempty" mapstructure:"utls"`
	Padding *PaddingCfg `json:"padding,omitempty" mapstructure:"padding"`
}

type UTLSCfg struct {
	Enabled  bool `json:"enabled" mapstructure:"enabled"`
	Fallback bool `json:"fallback" mapstructure:"fallback"`
}

type PaddingCfg struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	Size    int  `json:"size" mapstructure:"size"`
}

// @sk-task production-gap#T1.1: explicit client TLS trust surface (AC-001)
// @sk-task whitelist-obfuscation#T1.1: add SNI list (AC-004)
type ClientTLSCfg struct {
	CAFile     string   `json:"ca_file" mapstructure:"ca_file"`
	ServerName string   `json:"server_name" mapstructure:"server_name"`
	VerifyMode string   `json:"verify_mode" mapstructure:"verify_mode"`
	SNI        []string `json:"sni,omitempty" mapstructure:"sni"`
}

type ProxyAuthCfg struct {
	Username string `json:"username" mapstructure:"username"`
	Password string `json:"password" mapstructure:"password"`
}

// @sk-task geoip-geosite-integration#T1.2: SourceRule union type (AC-001)
type SourceRule struct {
	GeoIP   *string `json:"geoip,omitempty" yaml:"geoip,omitempty" mapstructure:"geoip,omitempty"`
	GeoSite *string `json:"geosite,omitempty" yaml:"geosite,omitempty" mapstructure:"geosite,omitempty"`
	CIDR    *string `json:"cidr,omitempty" yaml:"cidr,omitempty" mapstructure:"cidr,omitempty"`
	URL     *string `json:"url,omitempty" yaml:"url,omitempty" mapstructure:"url,omitempty"`
}

// @sk-task geoip-geosite-integration#T1.2: source rule type (AC-001)
func (s SourceRule) Type() string {
	switch {
	case s.GeoIP != nil:
		return "geoip"
	case s.GeoSite != nil:
		return "geosite"
	case s.CIDR != nil:
		return "cidr"
	case s.URL != nil:
		return "url"
	default:
		return "invalid"
	}
}

// @sk-task geoip-geosite-integration#T1.2: source rule value (AC-001)
func (s SourceRule) Value() string {
	switch {
	case s.GeoIP != nil:
		return *s.GeoIP
	case s.GeoSite != nil:
		return *s.GeoSite
	case s.CIDR != nil:
		return *s.CIDR
	case s.URL != nil:
		return *s.URL
	default:
		return ""
	}
}

// @sk-task geoip-geosite-integration#T1.2: source rule validation (AC-001)
func (s SourceRule) Valid() bool {
	n := 0
	if s.GeoIP != nil {
		n++
	}
	if s.GeoSite != nil {
		n++
	}
	if s.CIDR != nil {
		n++
	}
	if s.URL != nil {
		n++
	}
	return n == 1
}

// @sk-task dns-response-tracker#T1.2: DNSCacheCfg struct (AC-003)
type DNSCacheCfg struct {
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
	TTL     int  `json:"ttl,omitempty" yaml:"ttl,omitempty" mapstructure:"ttl,omitempty"`
}

// UnmarshalJSON accepts both true (boolean) and {"enabled":true,"ttl":60}
func (d *DNSCacheCfg) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	// try bool first (backward compat)
	if data[0] == 't' || data[0] == 'f' {
		var b bool
		if err := json.Unmarshal(data, &b); err != nil {
			return err
		}
		d.Enabled = b
		d.TTL = 60
		return nil
	}
	// fallback to object
	type alias DNSCacheCfg
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	d.Enabled = a.Enabled
	d.TTL = a.TTL
	return nil
}

// UnmarshalYAML accepts both true and {"enabled":true,"ttl":60}
func (d *DNSCacheCfg) UnmarshalYAML(value *yaml.Node) error {
	// try bool first
	var b bool
	if err := value.Decode(&b); err == nil {
		d.Enabled = b
		d.TTL = 60
		return nil
	}
	// fallback to struct
	type alias DNSCacheCfg
	var a alias
	if err := value.Decode(&a); err != nil {
		return err
	}
	d.Enabled = a.Enabled
	d.TTL = a.TTL
	return nil
}

// @sk-task routing-split-tunnel#T1.1: routing config struct (AC-009)
// @sk-task geoip-geosite-integration#T1.2: source fields (AC-001)
type RoutingCfg struct {
	DefaultRoute   string   `json:"default_route" mapstructure:"default_route"`
	IncludeRanges  []string `json:"include_ranges" mapstructure:"include_ranges"`
	ExcludeRanges  []string `json:"exclude_ranges" mapstructure:"exclude_ranges"`
	IncludeIPs     []string `json:"include_ips" mapstructure:"include_ips"`
	ExcludeIPs     []string `json:"exclude_ips" mapstructure:"exclude_ips"`
	IncludeDomains []string `json:"include_domains" mapstructure:"include_domains"`
	ExcludeDomains []string `json:"exclude_domains" mapstructure:"exclude_domains"`

	GeoIPPath      string       `json:"geoip_path,omitempty" yaml:"geoip_path,omitempty" mapstructure:"geoip_path,omitempty"`
	GeoSitePath    string       `json:"geosite_path,omitempty" yaml:"geosite_path,omitempty" mapstructure:"geosite_path,omitempty"`
	GeoIPURL       string       `json:"geoip_url,omitempty" yaml:"geoip_url,omitempty" mapstructure:"geoip_url,omitempty"`
	GeoSiteURL     string       `json:"geosite_url,omitempty" yaml:"geosite_url,omitempty" mapstructure:"geosite_url,omitempty"`
	SourceTTL      int          `json:"source_ttl_hours,omitempty" yaml:"source_ttl_hours,omitempty" mapstructure:"source_ttl_hours,omitempty"`
	IncludeSources []SourceRule `json:"include_sources,omitempty" yaml:"include_sources,omitempty" mapstructure:"include_sources,omitempty"`
	ExcludeSources []SourceRule `json:"exclude_sources,omitempty" yaml:"exclude_sources,omitempty" mapstructure:"exclude_sources,omitempty"`
	DNSCache       *DNSCacheCfg `json:"dns_cache,omitempty" yaml:"dns_cache,omitempty" mapstructure:"dns_cache,omitempty"`
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
// @sk-task whitelist-obfuscation#T1.2: obfuscation backward compat decoder (AC-001)
func LoadClientConfig(path string) (*ClientConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("KVN_CLIENT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	// backward compat: obfuscation: true → {enabled: true}
	if raw := v.Get("obfuscation"); raw != nil {
		if _, ok := raw.(bool); ok {
			v.Set("obfuscation", map[string]interface{}{"enabled": raw})
		}
	}

	// @sk-task dns-upstreams-list#T1.1: normalize deprecated upstream → upstreams via viper (AC-002)
	if upstream := v.GetString("dns_proxy.upstream"); upstream != "" {
		if v.IsSet("dns_proxy.upstreams") {
			log.Printf("[config] WARNING: both dns_proxy.upstream and dns_proxy.upstreams specified; upstreams takes priority")
		} else {
			v.Set("dns_proxy.upstreams", []string{upstream})
		}
	}

	cfg := &ClientConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config %s: %w", path, err)
	}
	SetClientDefaults(cfg)
	if len(cfg.TLS.SNI) > 0 && cfg.TLS.VerifyMode == "verify" {
		log.Printf("[config] WARNING: tls.sni=%v with verify_mode=verify will fail — SNI domain doesn't match server certificate. Set verify_mode=insecure when using custom SNI.", cfg.TLS.SNI)
	}
	if cfg.Crypto.Enabled && cfg.Crypto.Key == "" {
		cfg.Crypto.Enabled = false
	}

	// @sk-task client-relay-mode#T1.1: relay config defaults and validation (AC-003)
	// @sk-task quic-relay-mode#T1.1: RelayQuicCfg defaults and validation (AC-003)
	if cfg.Mode == "relay" {
		if cfg.Relay == nil {
			return nil, fmt.Errorf("mode is 'relay' but relay config block is missing")
		}
		if cfg.Relay.Listen == "" {
			return nil, fmt.Errorf("relay.listen is required when mode is 'relay'")
		}
		if cfg.Relay.MaxConnections <= 0 {
			cfg.Relay.MaxConnections = 100
		}
		if len(cfg.Relay.WSPaths) == 0 {
			cfg.Relay.WSPaths = []string{"/tunnel"}
		}
		if cfg.Relay.Quic != nil {
			if cfg.Relay.Quic.KeepAlive <= 0 {
				cfg.Relay.Quic.KeepAlive = 7
			}
			if cfg.Relay.Quic.IdleTimeout <= 0 {
				return nil, fmt.Errorf("relay.quic.idle_timeout must be > 0")
			}
		}
	} else if cfg.Relay != nil {
		log.Println("[config] WARNING: relay config block present but mode is not 'relay', ignoring")
	}

	// @sk-task production-readiness-gap#T1: warn when secrets come from config file (AC-001)
	secretKeys := []string{"auth.token", "crypto.key"}
	if cfg.ProxyAuth != nil {
		secretKeys = append(secretKeys, "proxy_auth.username", "proxy_auth.password")
	}
	if warnSecretInFile("KVN_CLIENT", secretKeys) {
		log.Println("[config] WARNING: secrets (auth.token, crypto.key) loaded from config file. Use environment variables KVN_CLIENT_* for production.")
	}
	return cfg, nil
}

// @sk-task kvn-web-config-update#T3.1: exported defaults for web UI (AC-005, AC-006)
func SetClientDefaults(cfg *ClientConfig) {
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
	if cfg.MaxMessageSize <= 0 {
		cfg.MaxMessageSize = 10 * 1024 * 1024
	}
	if cfg.TunnelTimeout <= 0 {
		cfg.TunnelTimeout = 30
	}
	if cfg.ProxyMaxConcurrency <= 0 {
		cfg.ProxyMaxConcurrency = 1000
	}
	if cfg.ProxyConnections <= 0 {
		cfg.ProxyConnections = 10
	}
	if cfg.DNSProxy.Listen == "" {
		cfg.DNSProxy.Listen = "127.0.0.54:53"
	}
	if len(cfg.DNSProxy.Upstreams) == 0 {
		cfg.DNSProxy.Upstreams = append([]string{}, DefaultDNSUpstreams...)
	}
}

// @sk-task kvn-web#T1.1: SaveClientConfig writes config to YAML (AC-005)
// @sk-task quic-obfuscation#T3.3: normalize routing defaults on save (AC-001)
func SaveClientConfig(path string, cfg *ClientConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
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
	return os.WriteFile(path, data, 0o600)
}

// @sk-task relay-terminator#T1.1: RelayConfig for cmd/relay (AC-001)
type RelayConfig struct {
	Mode          string          `json:"mode" mapstructure:"mode"`
	Relay         RelayTermCfg    `json:"relay" mapstructure:"relay"`
	Server        string          `json:"server" mapstructure:"server"`
	Transport     string          `json:"transport" mapstructure:"transport"`
	Obfuscation   *ObfuscationCfg `json:"obfuscation,omitempty" mapstructure:"obfuscation"`
	Crypto        CryptoCfg       `json:"crypto" mapstructure:"crypto"`
	TLS           ClientTLSCfg    `json:"tls" mapstructure:"tls"`
	Auth          ServerAuth      `json:"auth" mapstructure:"auth"`
	UpstreamToken string          `json:"upstream_token" mapstructure:"upstream_token"`
	Log           LogConfig       `json:"log" mapstructure:"log"`
}

// @sk-task relay-terminator#T1.1: relay terminator config section (AC-001)
type RelayTermCfg struct {
	Mode           string           `json:"mode" mapstructure:"mode"`
	Listen         string           `json:"listen" mapstructure:"listen"`
	WSPaths        []string         `json:"ws_paths,omitempty" mapstructure:"ws_paths"`
	MaxConnections int              `json:"max_connections" mapstructure:"max_connections"`
	TLS            *RelayTLSCfg     `json:"tls,omitempty" mapstructure:"tls"`
	Quic           *RelayQuicCfg    `json:"quic,omitempty" mapstructure:"quic"`
	Routing        *RelayRoutingCfg `json:"routing,omitempty" mapstructure:"routing"`
	Network        *NetworkCfg      `json:"network,omitempty" mapstructure:"network"`
}

// @sk-task dns-upstreams-list#T1.1: RelayDNSCfg with Upstreams []string + backward compat (AC-007)
// @sk-task relay-terminator#T6.1: DNS config for relay routing (RQ-008, RQ-011)
type RelayDNSCfg struct {
	Upstream    string   `json:"upstream,omitempty" mapstructure:"upstream"` // DEPRECATED: use Upstreams
	Upstreams   []string `json:"upstreams,omitempty" mapstructure:"upstreams"`
	CacheTTL    int      `json:"cache_ttl" mapstructure:"cache_ttl"`
	Transparent bool     `json:"transparent" mapstructure:"transparent"`
}

// @sk-task relay-terminator#T6.1: routing config for relay terminator (RQ-008, RQ-011)
// @sk-task geoip-geosite-integration#T3.3: direct_sources with geoip/geosite/cidr/url (AC-002, AC-003, AC-004)
type RelayRoutingCfg struct {
	DirectRanges  []string     `json:"direct_ranges" mapstructure:"direct_ranges"`
	DirectDomains []string     `json:"direct_domains" mapstructure:"direct_domains"`
	DirectSources []SourceRule `json:"direct_sources,omitempty" mapstructure:"direct_sources"`
	GeoIPPath     string       `json:"geoip_path,omitempty" mapstructure:"geoip_path"`
	GeoSitePath   string       `json:"geosite_path,omitempty" mapstructure:"geosite_path"`
	GeoIPURL      string       `json:"geoip_url,omitempty" mapstructure:"geoip_url"`
	GeoSiteURL    string       `json:"geosite_url,omitempty" mapstructure:"geosite_url"`
	SourceTTL     int          `json:"source_ttl_hours,omitempty" mapstructure:"source_ttl_hours"`
	DNS           *RelayDNSCfg `json:"dns,omitempty" mapstructure:"dns"`
}

// @sk-task relay-terminator#T1.1: load relay config (AC-001)
func LoadRelayConfig(path string) (*RelayConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("KVN_RELAY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &RelayConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config %s: %w", path, err)
	}

	if cfg.Mode == "" {
		cfg.Mode = "relay"
	}
	if cfg.Relay.MaxConnections <= 0 {
		cfg.Relay.MaxConnections = 100
	}
	if len(cfg.Relay.WSPaths) == 0 {
		cfg.Relay.WSPaths = []string{"/tunnel"}
	}
	if cfg.Relay.Quic != nil {
		if cfg.Relay.Quic.KeepAlive <= 0 {
			cfg.Relay.Quic.KeepAlive = 7
		}
		if cfg.Relay.Quic.IdleTimeout <= 0 {
			return nil, fmt.Errorf("relay.quic.idle_timeout must be > 0")
		}
	}
	if cfg.TLS.VerifyMode == "" {
		cfg.TLS.VerifyMode = "insecure"
	}
	if cfg.Relay.Routing != nil && cfg.Relay.Routing.DNS != nil {
		// @sk-task dns-upstreams-list#T1.1: migrate Relay Upstream→Upstreams (AC-007)
		if len(cfg.Relay.Routing.DNS.Upstreams) == 0 {
			if cfg.Relay.Routing.DNS.Upstream != "" {
				cfg.Relay.Routing.DNS.Upstreams = []string{cfg.Relay.Routing.DNS.Upstream}
			} else {
				cfg.Relay.Routing.DNS.Upstreams = append([]string{}, DefaultDNSUpstreams...)
			}
		}
		if cfg.Relay.Routing.DNS.CacheTTL <= 0 {
			cfg.Relay.Routing.DNS.CacheTTL = 60
		}
	}
	if w := warnSecretInFile("KVN_RELAY", []string{"crypto.key"}); w {
		log.Println("[config] WARNING: secrets (crypto.key) loaded from config file. Use environment variable KVN_RELAY_CRYPTO_KEY for production.")
	}
	return cfg, nil
}

// @sk-task dns-upstreams-list#T1.1: centralized default DNS upstreams (AC-003)
var DefaultDNSUpstreams = []string{"1.1.1.1:53", "8.8.8.8:53"}

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
