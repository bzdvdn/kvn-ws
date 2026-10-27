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
