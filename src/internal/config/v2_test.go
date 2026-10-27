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
