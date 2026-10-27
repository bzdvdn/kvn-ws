// Package client implements the KVN v2 client. The first MVP mode is a local
// SOCKS5/HTTP CONNECT proxy so the HTTP/2 stream path can be tested before the
// TUN/gVisor data plane lands.
package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/proxy"
	tlspkg "github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	"github.com/bzdvdn/kvn-ws/src/internal/v2/transport"
)

// Client runs one selected v2 server.
type Client struct {
	cfg    *config.V2ClientConfig
	logger *zap.Logger
}

func New(cfg *config.V2ClientConfig, logger *zap.Logger) *Client {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{cfg: cfg, logger: logger}
}

// ActiveServer returns the selected server entry.
func (c *Client) ActiveServer() (config.V2ServerEntry, error) {
	for _, s := range c.cfg.Servers {
		if s.Name == c.cfg.ActiveServer {
			return s, nil
		}
	}
	return config.V2ServerEntry{}, fmt.Errorf("active server %q not found", c.cfg.ActiveServer)
}

// Run starts the local proxy and tunnels TCP via v2 streams until ctx ends.
func (c *Client) Run(ctx context.Context) error {
	if strings.ToLower(c.cfg.Mode) != "" && strings.ToLower(c.cfg.Mode) != "proxy" {
		return errors.New("v2 TUN mode is not implemented yet; use mode: proxy")
	}
	server, err := c.ActiveServer()
	if err != nil {
		return err
	}
	endpoint, err := normalizeEndpoint(server.Endpoint)
	if err != nil {
		return err
	}
	tlsCfg, err := tlspkg.NewClientTLSConfigFromSettings(tlspkg.ClientTLSSettings{
		CAFile:     c.cfg.TLS.CAFile,
		ServerName: c.cfg.TLS.ServerName,
		VerifyMode: c.cfg.TLS.VerifyMode,
	})
	if err != nil {
		return err
	}
	tlsCfg.NextProtos = []string{"h2"}

	var proxyAuth *proxy.ProxyAuth
	if c.cfg.ProxyAuth != nil {
		proxyAuth = &proxy.ProxyAuth{Username: c.cfg.ProxyAuth.Username, Password: c.cfg.ProxyAuth.Password}
	}
	listener := proxy.NewListener(c.cfg.ProxyListen, proxyAuth, func(conn net.Conn, dst string) {
		c.handleConn(ctx, conn, dst, endpoint, server.Token, tlsCfg)
	})
	if err := listener.Start(); err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	c.logger.Info("v2 proxy listening",
		zap.String("listen", listener.Addr().String()),
		zap.String("server", server.Name),
		zap.String("endpoint", endpoint),
	)
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	if err := listener.AcceptLoop(); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func (c *Client) handleConn(ctx context.Context, client net.Conn, dst, endpoint, token string, tlsCfg *tls.Config) {
	defer func() { _ = client.Close() }()
	stream, err := transport.Dial(ctx, endpoint, token, dst, tlsCfg)
	if err != nil {
		c.logger.Warn("v2 stream dial failed", zap.String("dst", dst), zap.Error(err))
		return
	}
	defer func() { _ = stream.Close() }()

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(stream, client)
		if tc, ok := client.(*net.TCPConn); ok {
			_ = tc.CloseRead()
		}
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, stream)
		if tc, ok := client.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
}

// @sk-task kvn-v2-transport#T2.0: normalize v2 endpoint to the standard stream path (AC-001)
func normalizeEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme == "wss" {
		u.Scheme = "https"
	}
	if u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("v2 endpoint must be https URL")
	}
	u.Path = transport.Path
	u.RawQuery = ""
	return u.String(), nil
}
