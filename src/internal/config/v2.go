package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

// @sk-task kvn-v2-transport#T1.1: v2 server config schema (AC-001, AC-007)
type V2ServerConfig struct {
	Listen     string      `mapstructure:"listen"`
	TLS        TLSCfg      `mapstructure:"tls"`
	Auth       ServerAuth  `mapstructure:"auth"`
	Fallback   FallbackCfg `mapstructure:"fallback"`
	MaxStreams int         `mapstructure:"max_streams"`
	QuicListen string      `mapstructure:"quic_listen"`
}

// @sk-task kvn-v2-transport#T1.1: v2 client multi-server entries (AC-005, AC-007)
type V2ServerEntry struct {
	Name     string `json:"name" mapstructure:"name"`
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
	Token    string `json:"token" mapstructure:"token"`
}

type V2ClientConfig struct {
	ActiveServer  string          `json:"active_server" mapstructure:"active_server"`
	Servers       []V2ServerEntry `json:"servers" mapstructure:"servers"`
	MTU           int             `json:"mtu" mapstructure:"mtu"`
	Mode          string          `json:"mode" mapstructure:"mode"`
	TUNAddress    string          `json:"tun_address,omitempty" mapstructure:"tun_address"`
	TUNGateway    string          `json:"tun_gateway,omitempty" mapstructure:"tun_gateway"`
	QuicEnabled   bool            `json:"quic_enabled,omitempty" mapstructure:"quic_enabled"`
	QuicEndpoint  string          `json:"quic_endpoint,omitempty" mapstructure:"quic_endpoint"`
	IPv6          bool            `json:"ipv6" mapstructure:"ipv6"`
	AutoReconnect *bool           `json:"auto_reconnect,omitempty" mapstructure:"auto_reconnect"`
	ProxyListen   string          `json:"proxy_listen,omitempty" mapstructure:"proxy_listen"`
	ProxyAuth     *ProxyAuthCfg   `json:"proxy_auth,omitempty" mapstructure:"proxy_auth"`
	// @sk-task kvn-v2-transport#T3.2: advanced settings stay first-class (AC-005)
	Routing   *RoutingCfg   `json:"routing,omitempty" mapstructure:"routing"`
	DNSProxy  DNSProxyCfg   `json:"dns_proxy,omitempty" mapstructure:"dns_proxy"`
	TLS       ClientTLSCfg  `json:"tls,omitempty" mapstructure:"tls"`
	Reconnect *ReconnectCfg `json:"reconnect,omitempty" mapstructure:"reconnect"`
	Log       LogConfig     `json:"log,omitempty" mapstructure:"log"`
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
	return nil
}

// @sk-task kvn-v2-transport#T1.1: v2 client defaults and validation (AC-005, AC-007)
func DefaultV2ClientConfig() V2ClientConfig {
	return V2ClientConfig{MTU: 1200, Mode: "tun"}
}

func (c *V2ClientConfig) ApplyDefaults() {
	d := DefaultV2ClientConfig()
	if c.MTU <= 0 {
		c.MTU = d.MTU
	}
	if c.Mode == "" {
		c.Mode = d.Mode
	}
	if c.TUNAddress == "" {
		c.TUNAddress = "10.66.0.2/24"
	}
	if c.TUNGateway == "" {
		c.TUNGateway = "10.66.0.1"
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
}

func (c *V2ClientConfig) Validate() error {
	if len(c.Servers) == 0 {
		return fmt.Errorf("v2 client servers is required")
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
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}
