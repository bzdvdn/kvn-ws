package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/http2"

	"github.com/bzdvdn/kvn-ws/src/internal/tlsprofile"
)

// Client is a reusable HTTP/2 client. One Client multiplexes all TCP and UDP
// flows over a small number of TLS connections.
type Client struct {
	http         *http.Client
	endpoint     string
	token        string
	streamPath   string
	udpPath      string
	targetHeader string
	clientID     string
	padding      bool
	paddingSize  int
}

// NewClient creates a shared v2 HTTP/2 client with legacy-compatible defaults.
func NewClient(endpoint, token string, tlsCfg *tls.Config) (*Client, error) {
	opts := DefaultClientOptions(tlsCfg)
	return NewClientWithOptions(endpoint, token, &opts)
}

// NewClientWithOptions creates a shared v2 HTTP/2 client for endpoint.
func NewClientWithOptions(endpoint, token string, opts *ClientOptions) (*Client, error) {
	if opts.TLS == nil {
		return nil, errors.New("tls config is required")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid v2 endpoint %q", endpoint)
	}
	streamPath := opts.StreamPath
	if streamPath == "" {
		streamPath = DefaultPath
	}
	udpPath := opts.UDPPath
	if udpPath == "" {
		udpPath = DefaultUDPPath
	}
	// Preserve an explicit legacy endpoint path for tests/diagnostics.
	switch u.Path {
	case Path:
		streamPath = Path
		udpPath = UDPPath
	case UDPPath:
		udpPath = UDPPath
	}
	u.Path = ""
	cfg := opts.TLS.Clone()
	if len(cfg.NextProtos) == 0 {
		cfg.NextProtos = []string{"h2"}
	}
	var transport http.RoundTripper
	if opts.UTLSProfile != "" {
		dialer, err := tlsprofile.New(tlsprofile.Config{
			Profile:  opts.UTLSProfile,
			Fallback: opts.UTLSFallback,
			TLS:      cfg,
		})
		if err != nil {
			return nil, err
		}
		transport = &http2.Transport{
			TLSClientConfig: cfg,
			DialTLSContext:  dialer.DialTLSContext,
		}
	} else {
		transport = &http.Transport{
			TLSClientConfig:       cfg,
			ForceAttemptHTTP2:     true,
			DisableCompression:    true,
			MaxIdleConns:          64,
			MaxIdleConnsPerHost:   16,
			MaxConnsPerHost:       4,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		}
	}
	return &Client{
		http:         &http.Client{Transport: transport},
		endpoint:     u.String(),
		token:        token,
		streamPath:   streamPath,
		udpPath:      udpPath,
		targetHeader: opts.TargetHeader,
		clientID:     opts.ClientID,
		padding:      opts.Padding,
		paddingSize:  opts.PaddingSize,
	}, nil
}

// Session fetches the server-assigned TUN address/gateway.
func (c *Client) Session(ctx context.Context, clientID string) (TUNSession, error) {
	if clientID == "" {
		return TUNSession{}, errors.New("client_id is required")
	}
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return TUNSession{}, err
	}
	u.Path = DefaultPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return TUNSession{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Request-ID", clientID)
	resp, err := c.http.Do(req)
	if err != nil {
		return TUNSession{}, fmt.Errorf("session request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return TUNSession{}, fmt.Errorf("session status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var session TUNSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return TUNSession{}, fmt.Errorf("session decode: %w", err)
	}
	if err := session.Validate(); err != nil {
		return TUNSession{}, err
	}
	return session, nil
}

// DialTCP opens a TCP stream to target.
func (c *Client) DialTCP(ctx context.Context, target string) (*Stream, error) {
	resp, pw, err := c.open(ctx, http.MethodPost, c.streamPath, target, "application/octet-stream", c.padding)
	if err != nil {
		return nil, err
	}
	return &Stream{body: resp.Body, writer: pw, padding: c.padding, padSize: c.paddingSize}, nil
}

// DialICMP sends one ICMP echo request and returns the response body.
// The payload is buffered before Do, so the client does not wait for response
// headers before sending the ICMP body (avoids an extra client/server RTT).
func (c *Client) DialICMP(ctx context.Context, target string, payload []byte) (io.ReadCloser, error) {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	u.Path = DefaultPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.ContentLength = int64(len(payload))
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/icmp")
	if c.clientID != "" {
		req.Header.Set("X-Request-ID", c.clientID)
	}
	if c.targetHeader != "" {
		req.Header.Set(c.targetHeader, target)
	} else {
		req.Host = target
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("v2 icmp dial: %w", err)
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/icmp") {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("v2 icmp status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}

// DialUDP opens a best-effort UDP flow to target.
func (c *Client) DialUDP(ctx context.Context, target string) (*DatagramStream, error) {
	resp, pw, err := c.open(ctx, http.MethodPut, c.udpPath, target, "application/octet-stream", c.padding)
	if err != nil {
		return nil, err
	}
	return &DatagramStream{body: resp.Body, writer: pw, padding: c.padding, padSize: c.paddingSize}, nil
}

func (c *Client) open(ctx context.Context, method, path, target, contentType string, pad bool) (*http.Response, *io.PipeWriter, error) {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, nil, err
	}
	u.Path = path
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), pr)
	if err != nil {
		_ = pw.Close()
		return nil, nil, err
	}
	req.ContentLength = -1
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.clientID != "" {
		req.Header.Set("X-Request-ID", c.clientID)
	}
	if pad {
		contentType = fmt.Sprintf("%s; pad=%d", contentType, c.paddingSize)
	}
	req.Header.Set("Content-Type", contentType)
	if c.targetHeader != "" {
		req.Header.Set(c.targetHeader, target)
	} else {
		req.Host = target
	}
	resp, err := c.http.Do(req)
	if err != nil {
		_ = pw.Close()
		return nil, nil, fmt.Errorf("v2 %s dial: %w", path, err)
	}
	contentTypeOK := strings.HasPrefix(resp.Header.Get("Content-Type"), "application/octet-stream") ||
		strings.HasPrefix(resp.Header.Get("Content-Type"), "application/icmp")
	if resp.StatusCode != http.StatusOK || !contentTypeOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		_ = pw.Close()
		return nil, nil, fmt.Errorf("v2 %s status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, pw, nil
}

// Dial is a convenience wrapper for tests and diagnostics.
func Dial(ctx context.Context, endpoint, token, target string, tlsCfg *tls.Config) (*Stream, error) {
	c, err := NewClient(endpoint, token, tlsCfg)
	if err != nil {
		return nil, err
	}
	return c.DialTCP(ctx, target)
}

// DialUDP is a convenience wrapper for tests and diagnostics.
func DialUDP(ctx context.Context, endpoint, token, target string, tlsCfg *tls.Config) (*DatagramStream, error) {
	c, err := NewClient(endpoint, token, tlsCfg)
	if err != nil {
		return nil, err
	}
	return c.DialUDP(ctx, target)
}
