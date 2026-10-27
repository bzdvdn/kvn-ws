package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a reusable HTTP/2 client. One Client multiplexes all TCP and UDP
// flows over a small number of TLS connections.
type Client struct {
	http     *http.Client
	endpoint string
	token    string
}

// NewClient creates a shared v2 HTTP/2 client for endpoint.
func NewClient(endpoint, token string, tlsCfg *tls.Config) (*Client, error) {
	if tlsCfg == nil {
		return nil, errors.New("tls config is required")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid v2 endpoint %q", endpoint)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = Path
	}
	cfg := tlsCfg.Clone()
	if len(cfg.NextProtos) == 0 {
		cfg.NextProtos = []string{"h2"}
	}
	tr := &http.Transport{
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
	return &Client{
		http:     &http.Client{Transport: tr},
		endpoint: u.String(),
		token:    token,
	}, nil
}

// DialTCP opens a TCP stream to target.
func (c *Client) DialTCP(ctx context.Context, target string) (*Stream, error) {
	resp, pw, err := c.open(ctx, Path, target)
	if err != nil {
		return nil, err
	}
	return &Stream{body: resp.Body, writer: pw}, nil
}

// DialUDP opens a best-effort UDP flow to target.
func (c *Client) DialUDP(ctx context.Context, target string) (*DatagramStream, error) {
	resp, pw, err := c.open(ctx, UDPPath, target)
	if err != nil {
		return nil, err
	}
	return &DatagramStream{body: resp.Body, writer: pw}, nil
}

func (c *Client) open(ctx context.Context, path, target string) (*http.Response, *io.PipeWriter, error) {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, nil, err
	}
	u.Path = path
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), pr)
	if err != nil {
		_ = pw.Close()
		return nil, nil, err
	}
	req.ContentLength = -1
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set(TargetHeader, target)
	resp, err := c.http.Do(req)
	if err != nil {
		_ = pw.Close()
		return nil, nil, fmt.Errorf("v2 %s dial: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/octet-stream") {
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
