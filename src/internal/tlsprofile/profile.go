// Package tlsprofile provides browser-like uTLS ClientHello profiles for the
// v2 client. It is deliberately independent from the v1 transport package.
package tlsprofile

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"

	utls "github.com/refraction-networking/utls"
)

const (
	Chrome  = "chrome"
	Firefox = "firefox"
	Safari  = "safari"
)

var supported = map[string]utls.ClientHelloID{
	Chrome:  utls.HelloChrome_Auto,
	Firefox: utls.HelloFirefox_Auto,
	Safari:  utls.HelloSafari_Auto,
}

// DefaultProfiles is the profile set advertised by the v2 client.
func DefaultProfiles() []string {
	return []string{Chrome, Firefox, Safari}
}

// NormalizeProfile returns a canonical profile name.
func NormalizeProfile(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if _, ok := supported[name]; !ok {
		return "", fmt.Errorf("unsupported utls profile %q", name)
	}
	return name, nil
}

// ProfileID maps a public profile name to a uTLS ClientHello.
func ProfileID(name string) (utls.ClientHelloID, error) {
	name, err := NormalizeProfile(name)
	if err != nil {
		return utls.ClientHelloID{}, err
	}
	return supported[name], nil
}

// Config controls the uTLS dialer.
type Config struct {
	Profile  string
	Fallback bool
	TLS      *tls.Config
	Dialer   *net.Dialer
}

// Dialer establishes TLS connections with a browser-like ClientHello.
type Dialer struct {
	profile  string
	fallback bool
	tls      *tls.Config
	dialer   *net.Dialer
}

// New creates a uTLS dialer.
func New(cfg Config) (*Dialer, error) {
	if cfg.TLS == nil {
		return nil, errors.New("tls config is required")
	}
	profile, err := NormalizeProfile(cfg.Profile)
	if err != nil {
		return nil, err
	}
	tlsCfg := cfg.TLS.Clone()
	if len(tlsCfg.NextProtos) == 0 {
		tlsCfg.NextProtos = []string{"h2"}
	}
	d := cfg.Dialer
	if d == nil {
		d = &net.Dialer{}
	}
	return &Dialer{profile: profile, fallback: cfg.Fallback, tls: tlsCfg, dialer: d}, nil
}

// DialContext dials a TLS connection using the selected browser profile.
func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	raw, err := d.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("tcp dial: %w", err)
	}
	cfg := &utls.Config{
		ServerName:         serverName(d.tls.ServerName, addr),
		InsecureSkipVerify: d.tls.InsecureSkipVerify, // #nosec G402 — inherited from explicit client config
		RootCAs:            d.tls.RootCAs,
		MinVersion:         d.tls.MinVersion,
		NextProtos:         d.tls.NextProtos,
	}
	uconn := utls.UClient(raw, cfg, supported[d.profile])
	if err := uconn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		if d.fallback {
			return d.dialFallback(ctx, network, addr)
		}
		return nil, fmt.Errorf("utls handshake: %w", err)
	}
	return uconn, nil
}

// DialTLSContext implements the shape needed by http2.Transport.
func (d *Dialer) DialTLSContext(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
	return d.DialContext(ctx, network, addr)
}

func (d *Dialer) dialFallback(ctx context.Context, network, addr string) (net.Conn, error) {
	raw, err := d.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("tcp fallback dial: %w", err)
	}
	cfg := d.tls.Clone()
	if cfg.ServerName == "" {
		cfg.ServerName = serverName("", addr)
	}
	conn := tls.Client(raw, cfg)
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("tls fallback handshake: %w", err)
	}
	return conn, nil
}

func serverName(configured, addr string) string {
	if configured != "" {
		return configured
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
