package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/bzdvdn/kvn-ws/src/internal/tunpool"

	"github.com/spf13/viper"
)

// @sk-task kvn-v2-transport#T1.1: v2 server config schema (AC-001, AC-007)
type TUNConfig struct {
	Subnet      string `mapstructure:"subnet"`
	RangeStart  string `mapstructure:"range_start"`
	RangeEnd    string `mapstructure:"range_end"`
	Gateway     string `mapstructure:"gateway"`
	LeaseTTLSec int    `mapstructure:"lease_ttl_sec"`
	LeaseFile   string `mapstructure:"lease_file"`
}

type V2ServerConfig struct {
	Listen     string      `mapstructure:"listen"`
	TLS        TLSCfg      `mapstructure:"tls"`
	Auth       ServerAuth  `mapstructure:"auth"`
	Fallback   FallbackCfg `mapstructure:"fallback"`
	MaxStreams int         `mapstructure:"max_streams"`
	QuicListen string      `mapstructure:"quic_listen"`
	TUN        TUNConfig   `mapstructure:"tun"`
}

// @sk-task kvn-v2-transport#T1.1: v2 client multi-server entries (AC-005, AC-007)
type V2ServerEntry struct {
	Name     string `json:"name" mapstructure:"name"`
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
	Token    string `json:"token" mapstructure:"token"`
}

// UTLSConfig controls the browser TLS fingerprint used by the v2 client.
type UTLSConfig struct {
	Enabled  *bool    `json:"enabled,omitempty" yaml:"enabled,omitempty" mapstructure:"enabled"`
	Profiles []string `json:"profiles,omitempty" yaml:"profiles,omitempty" mapstructure:"profiles"`
	Profile  string   `json:"profile,omitempty" yaml:"profile,omitempty" mapstructure:"profile"`
	Fallback *bool    `json:"fallback,omitempty" yaml:"fallback,omitempty" mapstructure:"fallback"`
}

// V2StealthConfig controls the browser-like v2 HTTP surface and padding.
type V2StealthConfig struct {
	Enabled     *bool  `json:"enabled,omitempty" yaml:"enabled,omitempty" mapstructure:"enabled"`
	Path        string `json:"path,omitempty" yaml:"path,omitempty" mapstructure:"path"`
	Padding     *bool  `json:"padding,omitempty" yaml:"padding,omitempty" mapstructure:"padding"`
	PaddingMode string `json:"padding_mode,omitempty" yaml:"padding_mode,omitempty" mapstructure:"padding_mode"`
	PaddingSize int    `json:"padding_size,omitempty" yaml:"padding_size,omitempty" mapstructure:"padding_size"`
}

type V2ClientConfig struct {
	ActiveServer  string          `json:"active_server" mapstructure:"active_server"`
	ClientID      string          `json:"client_id,omitempty" mapstructure:"client_id"`
	Servers       []V2ServerEntry `json:"servers" mapstructure:"servers"`
	MTU           int             `json:"mtu" mapstructure:"mtu"`
	Mode          string          `json:"mode" mapstructure:"mode"`
	QuicEnabled   bool            `json:"quic_enabled,omitempty" mapstructure:"quic_enabled"`
	QuicEndpoint  string          `json:"quic_endpoint,omitempty" mapstructure:"quic_endpoint"`
	IPv6          bool            `json:"ipv6" mapstructure:"ipv6"`
	AutoReconnect *bool           `json:"auto_reconnect,omitempty" mapstructure:"auto_reconnect"`
	ProxyListen   string          `json:"proxy_listen,omitempty" mapstructure:"proxy_listen"`
	ProxyAuth     *ProxyAuthCfg   `json:"proxy_auth,omitempty" mapstructure:"proxy_auth"`
	// @sk-task kvn-v2-transport#T3.2: advanced settings stay first-class (AC-005)
	Routing   *RoutingCfg     `json:"routing,omitempty" mapstructure:"routing"`
	DNSProxy  DNSProxyCfg     `json:"dns_proxy,omitempty" mapstructure:"dns_proxy"`
	TLS       ClientTLSCfg    `json:"tls,omitempty" mapstructure:"tls"`
	Reconnect *ReconnectCfg   `json:"reconnect,omitempty" mapstructure:"reconnect"`
	Log       LogConfig       `json:"log,omitempty" mapstructure:"log"`
	UTLS      UTLSConfig      `json:"utls,omitempty" mapstructure:"utls"`
	Stealth   V2StealthConfig `json:"stealth,omitempty" mapstructure:"stealth"`
}

// @sk-task kvn-v2-transport#T1.1: safe v2 defaults; stealth is always on (AC-003)
func DefaultV2ServerConfig() V2ServerConfig {
	return V2ServerConfig{
		Listen:     "0.0.0.0:443",
		Fallback:   FallbackCfg{Title: "Welcome"},
		MaxStreams: 4096,
	}
}

func (c *V2ServerConfig) ApplyDefaults() {
	d := DefaultV2ServerConfig()
	if c.Listen == "" {
		c.Listen = d.Listen
	}
	if c.MaxStreams <= 0 {
		c.MaxStreams = d.MaxStreams
	}
	if c.Fallback.Title == "" && c.Fallback.Upstream == "" {
		c.Fallback.Title = d.Fallback.Title
	}
}

func (c *V2ServerConfig) Validate() error {
	if c.Listen == "" {
		return fmt.Errorf("v2 server listen is required")
	}
	if c.TLS.Cert == "" || c.TLS.Key == "" {
		return fmt.Errorf("v2 server tls.cert and tls.key are required")
	}
	if len(c.Auth.Tokens) == 0 {
		return fmt.Errorf("v2 server auth.tokens is required")
	}
	if c.Fallback.Upstream != "" {
		u, err := url.Parse(c.Fallback.Upstream)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("v2 server fallback.upstream must be an http(s) URL")
		}
	}
	if err := validateTUN(c.TUN); err != nil {
		return err
	}
	return nil
}

func validateTUN(t TUNConfig) error {
	_, err := tunpool.New(tunpool.Config{
		Subnet: t.Subnet, RangeStart: t.RangeStart, RangeEnd: t.RangeEnd,
		Gateway: t.Gateway, LeaseTTLSec: t.LeaseTTLSec, LeaseFile: t.LeaseFile,
	})
	return err
}

// @sk-task kvn-v2-transport#T1.1: v2 client defaults and validation (AC-005, AC-007)
func DefaultV2ClientConfig() V2ClientConfig {
	enabled := true
	fallback := true
	return V2ClientConfig{
		MTU:  1200,
		Mode: "tun",
		UTLS: UTLSConfig{
			Enabled:  &enabled,
			Profiles: []string{"chrome", "firefox", "safari"},
			Profile:  "chrome",
			Fallback: &fallback,
		},
		Stealth: V2StealthConfig{
			Enabled:     &enabled,
			Padding:     &enabled,
			PaddingMode: "adaptive",
			PaddingSize: 64,
		},
	}
}

func (c *V2ClientConfig) ApplyDefaults() {
	d := DefaultV2ClientConfig()
	if c.MTU <= 0 {
		c.MTU = d.MTU
	}
	if c.Mode == "" {
		c.Mode = d.Mode
	}
	if c.ActiveServer == "" && len(c.Servers) > 0 {
		c.ActiveServer = c.Servers[0].Name
	}
	if c.AutoReconnect == nil {
		v := true
		c.AutoReconnect = &v
	}
	if c.ProxyListen == "" {
		c.ProxyListen = "127.0.0.1:2310"
	}
	if c.Routing == nil {
		c.Routing = &RoutingCfg{DefaultRoute: "server", ExcludeRanges: append([]string{}, DefaultExcludeRanges...)}
	}
	if c.DNSProxy.Listen == "" {
		c.DNSProxy.Listen = "127.0.0.153:53"
	}
	if len(c.DNSProxy.Upstreams) == 0 {
		c.DNSProxy.Upstreams = append([]string{}, DefaultDNSUpstreams...)
	}
	if c.TLS.VerifyMode == "" {
		c.TLS.VerifyMode = "verify"
	}
	if c.ClientID == "" {
		c.ClientID = NewClientID()
	}
	c.UTLS.ApplyDefaults()
	c.Stealth.ApplyDefaults()
}

// ApplyDefaults fills the browser/stealth profile defaults.
func (c *UTLSConfig) ApplyDefaults() {
	enabled := true
	fallback := true
	if c.Enabled == nil {
		c.Enabled = &enabled
	}
	if c.Fallback == nil {
		c.Fallback = &fallback
	}
	if len(c.Profiles) == 0 {
		c.Profiles = []string{"chrome", "firefox", "safari"}
	}
	if c.Profile == "" {
		c.Profile = "chrome"
	}
}

// ApplyDefaults fills the v2 HTTP surface defaults.
func (c *V2StealthConfig) ApplyDefaults() {
	enabled := true
	if c.Enabled == nil {
		c.Enabled = &enabled
	}
	if c.Padding == nil {
		c.Padding = &enabled
	}
	if c.PaddingMode == "" {
		c.PaddingMode = "adaptive"
	}
	if c.PaddingSize <= 0 {
		c.PaddingSize = 64
	}
}

func NewClientID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func (c *V2ClientConfig) Validate() error {
	if len(c.Servers) == 0 {
		return fmt.Errorf("v2 client servers is required")
	}
	if c.ClientID == "" {
		return fmt.Errorf("v2 client client_id is required")
	}
	names := make(map[string]struct{}, len(c.Servers))
	for i, s := range c.Servers {
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("v2 client servers[%d].name is required", i)
		}
		if _, ok := names[s.Name]; ok {
			return fmt.Errorf("v2 client duplicate server name %q", s.Name)
		}
		names[s.Name] = struct{}{}
		u, err := url.Parse(s.Endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "wss") {
			return fmt.Errorf("v2 client servers[%d].endpoint must be https/wss URL", i)
		}
		if s.Token == "" {
			return fmt.Errorf("v2 client servers[%d].token is required", i)
		}
	}
	if _, ok := names[c.ActiveServer]; !ok {
		return fmt.Errorf("v2 client active_server %q not found", c.ActiveServer)
	}
	allowedProfiles := map[string]struct{}{"chrome": {}, "firefox": {}, "safari": {}}
	for _, profile := range c.UTLS.Profiles {
		if _, ok := allowedProfiles[profile]; !ok {
			return fmt.Errorf("v2 client unsupported utls profile %q", profile)
		}
	}
	if _, ok := allowedProfiles[c.UTLS.Profile]; !ok {
		return fmt.Errorf("v2 client unsupported utls profile %q", c.UTLS.Profile)
	}
	if c.Stealth.Path != "" && !strings.HasPrefix(c.Stealth.Path, "/") {
		return fmt.Errorf("v2 client stealth.path must start with /")
	}
	return nil
}

// @sk-task kvn-v2-transport#T1.1: load v2 server config (AC-001)
func LoadV2ServerConfig(path string) (*V2ServerConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read v2 server config %s: %w", path, err)
	}
	cfg := &V2ServerConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal v2 server config: %w", err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// @sk-task v2-tun-assignment#T2.1: persist generated client id outside portable QR/JSON
func EnsureClientID(path string, configured string) string {
	if configured != "" {
		return configured
	}
	sidecar := path + ".client_id"
	if data, err := os.ReadFile(sidecar); err == nil { // #nosec G304 -- sidecar is next to the trusted config
		if id := strings.TrimSpace(string(data)); id != "" {
			return id
		}
	}
	id := NewClientID()
	if id != "" {
		_ = os.WriteFile(sidecar, []byte(id+"\n"), 0o600)
	}
	return id
}

// @sk-task kvn-v2-transport#T1.1: load v2 client config (AC-005)
func LoadV2ClientConfig(path string) (*V2ClientConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read v2 client config %s: %w", path, err)
	}
	cfg := &V2ClientConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal v2 client config: %w", err)
	}
	cfg.ClientID = EnsureClientID(path, cfg.ClientID)
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}
