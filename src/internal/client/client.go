// Package client implements the KVN v2 client. The first MVP mode is a local
// SOCKS5/HTTP CONNECT proxy so the HTTP/2 stream path can be tested before the
// TUN/gVisor data plane lands.
package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/dns"
	"github.com/bzdvdn/kvn-ws/src/internal/dnsproxy"
	"github.com/bzdvdn/kvn-ws/src/internal/proxy"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/tcpstack"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	tlspkg "github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
)

// Client runs one selected v2 server.
type Client struct {
	cfg    *config.V2ClientConfig
	logger *zap.Logger
	tunMu  sync.Mutex
	tunDev tun.TunDevice

	rxBytes    atomic.Int64
	txBytes    atomic.Int64
	latencyMS  atomic.Uint64
	reconnects atomic.Int64
	startedAt  time.Time
}

func New(cfg *config.V2ClientConfig, logger *zap.Logger) *Client {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{cfg: cfg, logger: logger}
}

// @sk-task kvn-v2-transport#T3.3: release TUN during webui disconnect/reconnect (AC-005)
func (c *Client) StopTun() {
	c.tunMu.Lock()
	dev := c.tunDev
	c.tunDev = nil
	c.tunMu.Unlock()
	if dev != nil {
		_ = dev.Close()
	}
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

// Run starts the selected v2 mode until ctx ends.
func (c *Client) Run(ctx context.Context) error {
	switch strings.ToLower(c.cfg.Mode) {
	case "", "proxy":
		return c.runProxy(ctx)
	case "tun":
		return c.runTUN(ctx)
	default:
		return fmt.Errorf("unsupported v2 mode %q", c.cfg.Mode)
	}
}

func (c *Client) connection() (config.V2ServerEntry, string, *tls.Config, error) {
	server, err := c.ActiveServer()
	if err != nil {
		return config.V2ServerEntry{}, "", nil, err
	}
	endpoint, err := normalizeEndpoint(server.Endpoint)
	if err != nil {
		return config.V2ServerEntry{}, "", nil, err
	}
	tlsCfg, err := tlspkg.NewClientTLSConfigFromSettings(tlspkg.ClientTLSSettings{
		CAFile:     c.cfg.TLS.CAFile,
		ServerName: c.cfg.TLS.ServerName,
		VerifyMode: c.cfg.TLS.VerifyMode,
	})
	if err != nil {
		return config.V2ServerEntry{}, "", nil, err
	}
	tlsCfg.NextProtos = []string{"h2"}
	return server, endpoint, tlsCfg, nil
}

// @sk-task kvn-v2-transport#T2.0: proxy mode for non-TUN environments (AC-001)
func (c *Client) runProxy(ctx context.Context) error {
	server, endpoint, tlsCfg, err := c.connection()
	if err != nil {
		return err
	}
	httpClient, err := transport.NewClientWithOptions(endpoint, server.Token, c.transportOptions(tlsCfg))
	if err != nil {
		return err
	}
	var proxyAuth *proxy.ProxyAuth
	if c.cfg.ProxyAuth != nil {
		proxyAuth = &proxy.ProxyAuth{Username: c.cfg.ProxyAuth.Username, Password: c.cfg.ProxyAuth.Password}
	}
	listener := proxy.NewListener(c.cfg.ProxyListen, proxyAuth, func(conn net.Conn, dst string) {
		c.handleConn(ctx, conn, dst, httpClient)
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

// ResolveTUN fetches the server-assigned TUN session without starting TUN.
func (c *Client) ResolveTUN(ctx context.Context) (transport.TUNSession, error) {
	server, endpoint, tlsCfg, err := c.connection()
	if err != nil {
		return transport.TUNSession{}, err
	}
	httpClient, err := transport.NewClientWithOptions(endpoint, server.Token, c.transportOptions(tlsCfg))
	if err != nil {
		return transport.TUNSession{}, err
	}
	sessionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return httpClient.Session(sessionCtx, c.cfg.ClientID)
}

// RunTUNDevice runs the v2 data plane on a caller-owned TUN device.
// Android VpnService owns IP/routes/DNS; this function only pumps packets and
// uses the server-assigned TUN address for the gVisor stack.
func (c *Client) RunTUNDevice(ctx context.Context, dev tun.TunDevice) error {
	server, endpoint, tlsCfg, err := c.connection()
	if err != nil {
		return err
	}
	httpClient, err := transport.NewClientWithOptions(endpoint, server.Token, c.transportOptions(tlsCfg))
	if err != nil {
		return err
	}
	sessionStart := time.Now()
	sessionCtx, sessionCancel := context.WithTimeout(ctx, 2*time.Second)
	session, err := httpClient.Session(sessionCtx, c.cfg.ClientID)
	sessionCancel()
	if err != nil {
		return fmt.Errorf("tun assignment unavailable: %w", err)
	}
	c.latencyMS.Store(uint64(time.Since(sessionStart).Milliseconds()))
	c.startedAt = time.Now()
	go c.latencyLoop(ctx, endpoint)
	c.logger.Debug("v2 session resolved", zap.String("tun_address", session.TUNAddress), zap.String("tun_gateway", session.TUNGateway), zap.Int64("latency_ms", int64(c.latencyMS.Load())))
	ip, _, err := net.ParseCIDR(session.TUNAddress)
	if err != nil {
		return fmt.Errorf("tun assignment address: %w", err)
	}
	dev = c.counted(dev)
	if net.ParseIP(session.TUNGateway) == nil {
		return fmt.Errorf("invalid tun assignment gateway %q", session.TUNGateway)
	}
	var routeSet *routing.RuleSet
	if c.cfg.Routing != nil {
		routeSet, _ = routing.NewRuleSet(c.cfg.Routing, c.logger)
	}
	direct := newDirectCache()
	dialer := &net.Dialer{}
	stack, err := tcpstack.New(ip, c.cfg.MTU, func(dctx context.Context, target string) (io.ReadWriteCloser, error) {
		if direct.HasTarget(target) {
			return dialer.DialContext(dctx, "tcp", target)
		}
		return httpClient.DialTCP(ctx, target)
	}, c.logger)
	if err != nil {
		return fmt.Errorf("v2 tcp stack: %w", err)
	}
	var quicClient *transport.QUICClient
	if c.cfg.QuicEnabled {
		qep := c.cfg.QuicEndpoint
		if qep == "" {
			qep = defaultQUICEndpoint(endpoint)
		}
		qc, qerr := transport.NewQUICClient(ctx, qep, server.Token, tlsCfg, c.logger)
		if qerr != nil {
			c.logger.Warn("v2 QUIC datagrams unavailable, using HTTP/2 UDP fallback", zap.Error(qerr))
		} else {
			quicClient = qc
			defer func() { _ = qc.Close() }()
		}
	}
	dialUDP := packetDialer(func(dctx context.Context, target string) (packetFlow, error) {
		if direct.HasTarget(target) {
			return newDirectUDPFlow(target)
		}
		if quicClient != nil {
			if f, ferr := quicClient.DialFlow(dctx, target); ferr == nil {
				return f, nil
			}
		}
		return httpClient.DialUDP(dctx, target)
	})
	udpMgr := newUDPManager(ctx, dev, dialUDP, c.logger)
	go stack.Run(ctx, func(p []byte) error { _, werr := dev.Write(p); return werr })
	c.logger.Info("v2 mobile TUN started", zap.String("server", server.Name), zap.String("tun_ip", session.TUNAddress))
	buf := make([]byte, 65535)
	for {
		n, rerr := dev.Read(buf)
		if rerr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return rerr
		}
		packet := make([]byte, n)
		copy(packet, buf[:n])
		if isICMPv4(packet) {
			go c.handlePing(ctx, httpClient, packet, dev)
			continue
		}
		if isUDPv4(packet) {
			if c.handleDirectDNS(packet, dev, routeSet, direct) {
				continue
			}
			udpMgr.Handle(packet)
			continue
		}
		stack.Inject(packet)
	}
}

// @sk-task kvn-v2-transport#T2.3: TUN mode with gVisor TCP split and v2 streams (AC-001)
func (c *Client) runTUN(ctx context.Context) error {
	server, endpoint, tlsCfg, err := c.connection()
	if err != nil {
		return err
	}
	httpClient, err := transport.NewClientWithOptions(endpoint, server.Token, c.transportOptions(tlsCfg))
	if err != nil {
		return err
	}
	sessionStart := time.Now()
	sessionCtx, sessionCancel := context.WithTimeout(ctx, 2*time.Second)
	session, err := httpClient.Session(sessionCtx, c.cfg.ClientID)
	sessionCancel()
	if err != nil {
		return fmt.Errorf("tun assignment unavailable: %w", err)
	}
	c.latencyMS.Store(uint64(time.Since(sessionStart).Milliseconds()))
	c.startedAt = time.Now()
	go c.latencyLoop(ctx, endpoint)
	c.logger.Debug("v2 session resolved", zap.String("tun_address", session.TUNAddress), zap.String("tun_gateway", session.TUNGateway), zap.Int64("latency_ms", int64(c.latencyMS.Load())))
	dev := tun.NewTunDevice()
	if err := dev.Open(); err != nil {
		return fmt.Errorf("open v2 tun: %w", err)
	}
	dev = c.counted(dev)
	c.tunMu.Lock()
	c.tunDev = dev
	c.tunMu.Unlock()
	defer func() {
		c.tunMu.Lock()
		if c.tunDev == dev {
			c.tunDev = nil
		}
		c.tunMu.Unlock()
		_ = dev.Close()
	}()

	ip, subnet, err := net.ParseCIDR(session.TUNAddress)
	if err != nil {
		return fmt.Errorf("tun assignment address: %w", err)
	}
	if err := dev.SetIP(ip, subnet); err != nil {
		return fmt.Errorf("set tun ip: %w", err)
	}
	if err := dev.SetMTU(c.cfg.MTU); err != nil {
		return fmt.Errorf("set tun mtu: %w", err)
	}
	// @sk-task kvn-v2-transport#T5.1: disable GSO/GRO, otherwise wireguard tun
	// returns ErrTooManySegments on Read.
	if err := dev.DisableGSO(); err != nil {
		c.logger.Warn("v2 tun gso/gro disable failed", zap.Error(err))
	}
	phyGW, phyIface, err := tun.SaveDefaultRoute()
	if err != nil {
		return fmt.Errorf("save physical route: %w", err)
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	serverIPs, _ := net.LookupIP(u.Hostname())
	for _, sip := range serverIPs {
		if v4 := sip.To4(); v4 != nil {
			_ = dev.AddExcludeRoute(v4.String()+"/32", phyGW, phyIface)
		}
	}
	// Apply CIDR/IP excludes directly at the OS route level for split tunnel.
	if c.cfg.Routing != nil {
		for _, cidr := range c.cfg.Routing.ExcludeRanges {
			_ = dev.AddExcludeRoute(cidr, phyGW, phyIface)
		}
		for _, rawIP := range c.cfg.Routing.ExcludeIPs {
			if ip := net.ParseIP(rawIP).To4(); ip != nil {
				_ = dev.AddExcludeRoute(ip.String()+"/32", phyGW, phyIface)
			}
		}
	}
	dnsProxyListen := c.cfg.DNSProxy.Listen
	if dnsProxyListen == "" {
		dnsProxyListen = "127.0.0.153:53"
	}
	upstreams := c.cfg.DNSProxy.Upstreams
	if len(upstreams) == 0 {
		upstreams = []string{"1.1.1.1:53", "8.8.8.8:53"}
	}
	for _, upstream := range upstreams {
		host, _, splitErr := net.SplitHostPort(upstream)
		if splitErr != nil {
			continue
		}
		if ns := net.ParseIP(host).To4(); ns != nil {
			_ = dev.AddExcludeRoute(ns.String()+"/32", phyGW, phyIface)
		}
	}
	gateway := net.ParseIP(session.TUNGateway)
	if gateway == nil {
		return fmt.Errorf("invalid tun assignment gateway %q", session.TUNGateway)
	}
	if err := dev.SetGateway(gateway); err != nil {
		return fmt.Errorf("set tun gateway: %w", err)
	}
	defer func() { _ = dev.RemoveGateway(gateway) }()
	defer dev.CleanupExcludeRoutes()

	dnsCtx, dnsCancel := context.WithCancel(ctx)
	defer dnsCancel()
	dnsSrv := dnsproxy.New(dnsProxyListen, upstreams...)
	dnsSrv.SetOrigResolvers(resolvServers())
	var routeSet *routing.RuleSet
	if c.cfg.Routing != nil {
		routeSet, _ = routing.NewRuleSet(c.cfg.Routing, c.logger)
	}
	direct := newDirectCache()
	if routeSet != nil {
		ttl := 60
		if c.cfg.Routing.DNSRouting != nil && c.cfg.Routing.DNSRouting.TTL > 0 {
			ttl = c.cfg.Routing.DNSRouting.TTL
		}
		tracker := dns.NewTracker(time.Duration(ttl) * time.Second)
		routeSet.SetTracker(tracker)
		dnsSrv.SetTracker(tracker)
		dnsSrv.SetRouteFunc(func(domain string) bool {
			return routeSet.MatchDomain(domain) == routing.RouteDirect
		})
		dnsSrv.SetDirectRouteFunc(func(ips []netip.Addr) {
			for _, a := range ips {
				if a.IsPrivate() || a.IsLoopback() {
					continue
				}
				_ = dev.AddExcludeRoute(a.String()+"/32", phyGW, phyIface)
			}
		})
	}
	go func() { _ = dnsSrv.Run(dnsCtx) }()
	defer func() { _ = dnsSrv.Shutdown() }()
	if backup, berr := dnsproxy.BackupResolvConf(); berr == nil && backup != nil {
		if oerr := dnsproxy.OverrideResolvConf(dnsProxyListen); oerr != nil {
			c.logger.Warn("v2 dns override failed", zap.Error(oerr))
		} else {
			defer func() { _ = backup.Restore() }()
		}
	}

	stack, err := tcpstack.New(ip, c.cfg.MTU, func(_ context.Context, target string) (io.ReadWriteCloser, error) {
		return httpClient.DialTCP(ctx, target)
	}, c.logger)
	if err != nil {
		return fmt.Errorf("v2 tcp stack: %w", err)
	}
	var quicClient *transport.QUICClient
	if c.cfg.QuicEnabled {
		qep := c.cfg.QuicEndpoint
		if qep == "" {
			qep = defaultQUICEndpoint(endpoint)
		}
		qc, qerr := transport.NewQUICClient(ctx, qep, server.Token, tlsCfg, c.logger)
		if qerr != nil {
			c.logger.Warn("v2 QUIC datagrams unavailable, using HTTP/2 UDP fallback", zap.String("endpoint", qep), zap.Error(qerr))
		} else {
			quicClient = qc
			defer func() { _ = qc.Close() }()
			c.logger.Info("v2 QUIC datagram channel ready", zap.String("endpoint", qep))
		}
	}
	dialUDP := packetDialer(func(dctx context.Context, target string) (packetFlow, error) {
		if direct.HasTarget(target) {
			return newDirectUDPFlow(target)
		}
		if quicClient != nil {
			if f, ferr := quicClient.DialFlow(dctx, target); ferr == nil {
				return f, nil
			}
		}
		return httpClient.DialUDP(dctx, target)
	})
	udpMgr := newUDPManager(ctx, dev, dialUDP, c.logger)
	c.logger.Info("v2 TUN started",
		zap.String("server", server.Name),
		zap.String("tun_ip", session.TUNAddress),
		zap.String("dns_proxy", dnsProxyListen),
	)
	go func() {
		<-ctx.Done()
		_ = dev.Close()
	}()
	go stack.Run(ctx, func(p []byte) error { _, werr := dev.Write(p); return werr })

	buf := make([]byte, 65535)
	for {
		n, rerr := dev.Read(buf)
		if rerr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return rerr
		}
		packet := make([]byte, n)
		copy(packet, buf[:n])
		if isICMPv4(packet) {
			go c.handlePing(ctx, httpClient, packet, dev)
			continue
		}
		if isUDPv4(packet) {
			if c.handleDirectDNS(packet, dev, routeSet, direct) {
				continue
			}
			udpMgr.Handle(packet)
			continue
		}
		stack.Inject(packet)
	}
}

// @sk-task kvn-v2-transport#T4.3: derive the QUIC UDP endpoint from the HTTPS endpoint (AC-008)
func defaultQUICEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return endpoint
	}
	if u.Port() != "" {
		return u.Host
	}
	port := "443"
	if u.Scheme == "https" {
		port = "443"
	}
	return net.JoinHostPort(u.Hostname(), port)
}

// @sk-task kvn-v2-transport#T4.1: route IPv4 UDP packets to the best-effort manager (AC-008)
func isUDPv4(packet []byte) bool {
	return len(packet) >= 20 && packet[0]>>4 == 4 && packet[9] == 17
}

func resolvServers() []string {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && fields[0] == "nameserver" {
			if ip := net.ParseIP(fields[1]); ip != nil {
				out = append(out, net.JoinHostPort(ip.String(), "53"))
			}
		}
	}
	return out
}

func (c *Client) handleConn(ctx context.Context, client net.Conn, dst string, httpClient *transport.Client) {
	defer func() { _ = client.Close() }()
	c.logger.Debug("v2 proxy stream", zap.String("target", dst))
	stream, err := httpClient.DialTCP(ctx, dst)
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
	u.Path = transport.DefaultPath
	u.RawQuery = ""
	return u.String(), nil
}

func (c *Client) transportOptions(tlsCfg *tls.Config) transport.ClientOptions {
	opts := transport.DefaultClientOptions(tlsCfg)
	opts.ClientID = c.cfg.ClientID
	if c.cfg.UTLS.Enabled != nil && *c.cfg.UTLS.Enabled {
		opts.UTLSProfile = c.cfg.UTLS.Profile
		opts.UTLSFallback = c.cfg.UTLS.Fallback == nil || *c.cfg.UTLS.Fallback
	} else {
		opts.UTLSProfile = ""
	}
	if c.cfg.Stealth.Path != "" {
		opts.StreamPath = c.cfg.Stealth.Path
		opts.UDPPath = c.cfg.Stealth.Path
	}
	if c.cfg.Stealth.Padding != nil {
		opts.Padding = *c.cfg.Stealth.Padding
	}
	opts.PaddingSize = c.cfg.Stealth.PaddingSize
	return opts
}
