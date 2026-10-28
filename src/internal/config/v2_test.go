package config

import "testing"

// @sk-test kvn-v2-transport#T1.1: v2 server/client defaults and validation (AC-003, AC-005)
func TestV2ConfigDefaults(t *testing.T) {
	s := DefaultV2ServerConfig()
	if s.Listen == "" || s.MaxStreams <= 0 || s.Fallback.Title == "" {
		t.Fatalf("bad v2 server defaults: %+v", s)
	}
	c := DefaultV2ClientConfig()
	if c.MTU <= 0 || c.Mode == "" {
		t.Fatalf("bad v2 client defaults: %+v", c)
	}
	c.ApplyDefaults()
	if c.UTLS.Enabled == nil || !*c.UTLS.Enabled || c.UTLS.Profile != "chrome" || len(c.UTLS.Profiles) < 3 {
		t.Fatalf("bad utls defaults: %+v", c.UTLS)
	}
	if c.Stealth.Enabled == nil || !*c.Stealth.Enabled || c.Stealth.PaddingMode != "adaptive" {
		t.Fatalf("bad stealth defaults: %+v", c.Stealth)
	}
}

func TestV2UTLSAndStealthValidation(t *testing.T) {
	c := V2ClientConfig{Servers: []V2ServerEntry{{Name: "a", Endpoint: "https://example.com", Token: "t"}}}
	c.ApplyDefaults()
	c.UTLS.Profile = "opera"
	if err := c.Validate(); err == nil {
		t.Fatal("unsupported utls profile accepted")
	}
	c.UTLS.Profile = "chrome"
	c.Stealth.Path = "api/v2/stream"
	if err := c.Validate(); err == nil {
		t.Fatal("relative stealth path accepted")
	}
}

func TestV2ClientValidation(t *testing.T) {
	c := V2ClientConfig{Servers: []V2ServerEntry{{Name: "a", Endpoint: "https://example.com/api/v2/stream", Token: "t"}}}
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	c.Servers[0].Endpoint = "http://example.com"
	if err := c.Validate(); err == nil {
		t.Fatal("http endpoint accepted")
	}
}

// @sk-test kvn-v2-transport#T5.2: v1 and v2 configs coexist independently (AC-007)
func TestV1V2ConfigCoexist(t *testing.T) {
	v1 := &ClientConfig{Server: "wss://v1.example.com/tunnel", Auth: AuthCfg{Token: "v1"}, Protocol: "v1"}
	SetClientDefaults(v1)
	if v1.Protocol != "v1" {
		t.Fatalf("v1 protocol = %q", v1.Protocol)
	}
	v2 := &V2ClientConfig{
		Servers: []V2ServerEntry{{Name: "v2", Endpoint: "https://v2.example.com", Token: "v2"}},
	}
	v2.ApplyDefaults()
	if err := v2.Validate(); err != nil {
		t.Fatalf("v2 config: %v", err)
	}
	if v2.Servers[0].Endpoint == v1.Server {
		t.Fatal("v1/v2 endpoints unexpectedly coupled")
	}
}

func TestV2ServerTUNValidation(t *testing.T) {
	valid := V2ServerConfig{
		Listen: "0.0.0.0:443",
		TLS:    TLSCfg{Cert: "c", Key: "k"},
		Auth:   ServerAuth{Tokens: []TokenCfg{{Name: "t", Secret: "s"}}},
		TUN:    TUNConfig{Subnet: "10.66.0.0/24", RangeStart: "10.66.0.2", RangeEnd: "10.66.0.3", Gateway: "10.66.0.1", LeaseTTLSec: 60},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.TUN.Gateway = "10.77.0.1"
	if err := valid.Validate(); err == nil {
		t.Fatal("gateway outside subnet accepted")
	}
	valid.TUN = TUNConfig{Subnet: "10.66.0.0/24", RangeStart: "10.66.0.2", RangeEnd: "10.66.0.3", Gateway: "10.66.0.1"}
	if err := valid.Validate(); err == nil {
		t.Fatal("zero lease TTL accepted")
	}
}

func TestEnsureClientIDPersists(t *testing.T) {
	path := t.TempDir() + "/client.yaml"
	a := EnsureClientID(path, "")
	b := EnsureClientID(path, "")
	if a == "" || a != b {
		t.Fatalf("client ids = %q/%q", a, b)
	}
	if got := EnsureClientID(path, "explicit"); got != "explicit" {
		t.Fatalf("explicit id = %q", got)
	}
}
