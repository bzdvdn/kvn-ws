package client

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	"github.com/bzdvdn/kvn-ws/src/internal/dns"
	"github.com/bzdvdn/kvn-ws/src/internal/dnsproxy"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	quictp "github.com/bzdvdn/kvn-ws/src/internal/transport/quic"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
	"github.com/bzdvdn/kvn-ws/src/internal/tunnel"
)

// @sk-task arch-refactoring#T3.1: use common dialStream (AC-004)
// @sk-task domain-routing: resolve server hostname for exclude route
func resolveServerIP(host string) net.IP {
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return nil
	}
	if ip := net.ParseIP(addrs[0]); ip != nil {
		return ip
	}
	return nil
}

func (c *Client) reconnectLoop(ctx context.Context, tunDev tun.TunDevice) {
	// @sk-task dns-cache-cleanup: clean stale resolv.conf and exclude routes from previous killed session
	dnsproxy.CleanupStaleDNS(c.cfg.DNSProxy.Listen)
	if u, uErr := url.Parse(c.cfg.Server); uErr == nil {
		if host := u.Hostname(); host != "" {
			tun.CleanupStaleExcludeRoutes(host)
		}
	}

	minBackoff, maxBackoff := parseBackoff(c.cfg.Reconnect)

	backoff := minBackoff
	attempt := 0

	for {
		select {
		case <-ctx.Done():
			removeKillSwitch(c.cfg, c.logger)
			return
		default:
		}

		attempt++
		c.logger.Info("connecting", zap.Int("attempt", attempt), zap.Duration("backoff", backoff))

		stream, err := dialStream(ctx, c.cfg, c.logger)
		if err != nil {
			c.logger.Warn("dial failed", zap.Error(err), zap.Duration("retry_in", backoff))
			applyKillSwitch(c.cfg, c.logger)
			sleepWithContext(ctx, backoff)
			backoff = nextBackoff(backoff, minBackoff, maxBackoff)
			continue
		}
		if c.metricCollector != nil {
			stream = &transport.CountingStreamConn{
				StreamConn: stream,
				AddTX:      c.metricCollector.AddTX,
				AddRX:      c.metricCollector.AddRX,
			}
		}

		removeKillSwitch(c.cfg, c.logger)
		backoff = minBackoff

		c.runSession(ctx, tunDev, stream)
	}
}

// tunConfig carries the results of TUN configuration shared across setup steps.
type tunConfig struct {
	sessionCipher *crypto.SessionCipher
	v4Mask        *net.IPNet
	phyGateway    net.IP
	phyIface      string
	havePhyRoute  bool
}

// runSession drives one tunnel session: handshake, TUN config, routing,
// DNS proxy, secondary channel and the forwarding loop.
func (c *Client) runSession(ctx context.Context, tunDev tun.TunDevice, stream tunnel.StreamConn) {
	defer func() { _ = stream.Close() }()
	// @sk-task dns-response-tracker#T3.5: CleanupExcludeRoutes — remove kernel routes on disconnect
	defer tunDev.CleanupExcludeRoutes()

	// Session-scoped context: cancelled when this session ends (even though the
	// outer reconnect context stays alive), so secondary re-bind attempts stop.
	sessionCtx, sessionCancel := context.WithCancel(ctx)
	defer sessionCancel()

	serverHello, err := c.handshakeSession(stream)
	if err != nil {
		c.logger.Error("handshake", zap.Error(err))
		return
	}

	tcfg, err := c.configureTun(tunDev, serverHello)
	if err != nil {
		c.logger.Error("configure tun", zap.Error(err))
		return
	}
	if cleanup := c.addBypassRoutes(tunDev, serverHello, tcfg); cleanup != nil {
		defer cleanup()
	}

	routeSet, tunRouter := c.buildTunRouter(stream, tcfg.sessionCipher, tunDev)
	if cleanup := c.setupDNSProxy(ctx, routeSet, tunDev, tcfg); cleanup != nil {
		defer cleanup()
	}

	tunSess := tunnel.NewSession(tunDev, stream, nil, serverHello.SessionId, "", nil, nil, nil, c.logger, tcfg.sessionCipher, nil,
		time.Duration(c.cfg.TunnelTimeout)*time.Second, c.cfg.ProxyMaxConcurrency, nil, nil, nil)
	// @sk-task quic-datagrams#T3.4: enable datagram path when the server confirmed it (AC-001)
	datagramsOK := c.cfg.UDPDatagramsEnabled() && quictp.DatagramCapable(serverHello.Transport)
	tunSess.SetDatagrams(datagramsOK)
	if datagramsOK {
		c.logger.Info("datagram mode enabled",
			zap.String("session", serverHello.SessionId),
			zap.String("transport", serverHello.Transport),
		)
	} else {
		c.logger.Debug("datagram mode disabled", zap.String("transport", serverHello.Transport))
	}
	if tunRouter != nil {
		tunSess.SetTunRouter(tunRouter)
	}
	if cleanup := c.bindSecondaryChannel(sessionCtx, tunSess, serverHello.SessionId); cleanup != nil {
		defer cleanup()
	}
	if err := tunSess.Run(sessionCtx); err != nil {
		c.logger.Info("session ended", zap.Error(err))
	}
}

// handshakeSession exchanges the ClientHello and returns the ServerHello.
func (c *Client) handshakeSession(stream tunnel.StreamConn) (*handshake.ServerHello, error) {
	// @sk-task quic-datagrams#T2.1: request datagram capability on the QUIC transport (AC-003)
	transport := c.cfg.Transport
	if transport == "quic" && c.cfg.UDPDatagramsEnabled() {
		transport = quictp.DatagramTransport
	}
	helloFrame, err := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Ipv6:         c.cfg.IPv6,
		Token:        c.cfg.Auth.Token,
		Mtu:          c.cfg.MTU,
		Transport:    transport,
	})
	if err != nil {
		return nil, fmt.Errorf("encode client hello: %w", err)
	}
	helloData, err := helloFrame.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode hello frame: %w", err)
	}
	if err := stream.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		return nil, fmt.Errorf("send hello: %w", err)
	}
	framing.ReturnBuffer(helloData)

	respData, err := stream.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read server hello: %w", err)
	}
	var respFrame framing.Frame
	if err := respFrame.Decode(respData); err != nil {
		return nil, fmt.Errorf("decode response frame: %w", err)
	}
	switch respFrame.Type {
	case framing.FrameTypeAuth:
		authErr, _ := handshake.DecodeAuthError(&respFrame)
		c.logger.Fatal("auth rejected", zap.String("reason", authErr.Reason))
		return nil, fmt.Errorf("auth rejected")
	case framing.FrameTypeHello:
		serverHello, err := handshake.DecodeServerHello(&respFrame)
		if err != nil {
			return nil, fmt.Errorf("decode server hello: %w", err)
		}
		c.logger.Info("handshake complete",
			zap.String("session", serverHello.SessionId),
			zap.String("ip", serverHello.AssignedIp.String()),
		)
		return serverHello, nil
	default:
		return nil, fmt.Errorf("unexpected response type: %d", int(respFrame.Type))
	}
}

// @sk-task game-latency#T1.3: effective client MTU = min(config, advertised); 0 means unset (AC-003)
func effectiveMTU(cfgMTU, advertisedMTU int) int {
	mtu := cfgMTU
	if advertisedMTU > 0 && (mtu == 0 || advertisedMTU < mtu) {
		mtu = advertisedMTU
	}
	return mtu
}

// configureTun applies the assigned addresses, MTU, GSO and session cipher,
// returning the session cipher plus physical-route info for bypass routes.
func (c *Client) configureTun(tunDev tun.TunDevice, serverHello *handshake.ServerHello) (*tunConfig, error) {
	mask := &net.IPNet{
		IP:   serverHello.AssignedIp,
		Mask: net.CIDRMask(tun.CIDRMaskV4Bits, tun.CIDRMaskV4Total),
	}
	if err := tunDev.SetIP(serverHello.AssignedIp, mask); err != nil {
		return nil, fmt.Errorf("set tun ip: %w", err)
	}
	if serverHello.AssignedIpv6 != nil {
		c.logger.Info("assigned IPv6", zap.String("ip6", serverHello.AssignedIpv6.String()))
		v6Mask := &net.IPNet{
			IP:   serverHello.AssignedIpv6,
			Mask: net.CIDRMask(tun.CIDRMaskV6Bits, tun.CIDRMaskV6Total),
		}
		if err := tunDev.SetIP(serverHello.AssignedIpv6, v6Mask); err != nil {
			return nil, fmt.Errorf("set tun ipv6: %w", err)
		}
	}
	// @sk-task game-latency#T1.3: clamp client MTU by the server-advertised MTU (AC-003)
	if mtu := effectiveMTU(c.cfg.MTU, serverHello.Mtu); mtu > 0 {
		if err := tunDev.SetMTU(mtu); err != nil {
			c.logger.Warn("set tun mtu", zap.Int("mtu", mtu), zap.Error(err))
		}
	}
	if err := tunDev.DisableGSO(); err != nil {
		c.logger.Warn("disable gso", zap.Error(err))
	} else {
		c.logger.Info("gso/gro disabled on tun")
	}
	tcfg := &tunConfig{v4Mask: mask}
	if len(c.masterKey) > 0 && len(serverHello.CryptoSalt) > 0 {
		var err error
		tcfg.sessionCipher, err = crypto.NewSessionCipher(c.masterKey, serverHello.CryptoSalt, serverHello.SessionId)
		if err != nil {
			return nil, fmt.Errorf("session cipher init: %w", err)
		}
		c.logger.Info("app-layer encryption active")
	} else if len(c.masterKey) > 0 && len(serverHello.CryptoSalt) == 0 {
		c.logger.Warn("server did not send crypto salt, connection will be unencrypted")
	}
	phyGateway, phyIface, err := tun.SaveDefaultRoute()
	if err != nil {
		c.logger.Warn("save default route (bypass routes won't be added)", zap.Error(err))
		return tcfg, nil
	}
	tcfg.phyGateway, tcfg.phyIface, tcfg.havePhyRoute = phyGateway, phyIface, true
	return tcfg, nil
}

// addBypassRoutes installs server exclude routes and the default gateway,
// returning a cleanup for both.
func (c *Client) addBypassRoutes(tunDev tun.TunDevice, serverHello *handshake.ServerHello, tcfg *tunConfig) func() {
	var cleanups []func()
	if tcfg.havePhyRoute {
		var excludeCIDRs []string
		if u, uErr := url.Parse(c.cfg.Server); uErr == nil {
			host := u.Hostname()
			if ip := net.ParseIP(host); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				excludeCIDRs = append(excludeCIDRs, host+"/"+strconv.Itoa(bits))
			} else if resolved := resolveServerIP(host); resolved != nil {
				bits := 32
				if resolved.To4() == nil {
					bits = 128
				}
				excludeCIDRs = append(excludeCIDRs, resolved.String()+"/"+strconv.Itoa(bits))
			}
		}
		if c.cfg.Routing != nil {
			for _, r := range c.cfg.Routing.ExcludeRanges {
				if r != "0.0.0.0/0" && r != "::/0" {
					excludeCIDRs = append(excludeCIDRs, r)
				}
			}
			for _, ip := range c.cfg.Routing.ExcludeIPs {
				parsed := net.ParseIP(ip)
				if parsed == nil {
					continue
				}
				bits := 32
				if parsed.To4() == nil {
					bits = 128
				}
				excludeCIDRs = append(excludeCIDRs, ip+"/"+strconv.Itoa(bits))
			}
		}
		for _, cidr := range excludeCIDRs {
			if err := tunDev.AddExcludeRoute(cidr, tcfg.phyGateway, tcfg.phyIface); err != nil {
				c.logger.Warn("add exclude route", zap.String("cidr", cidr), zap.Error(err))
			} else {
				ec := cidr
				cleanups = append(cleanups, func() {
					if err := tunDev.RemoveExcludeRoute(ec, tcfg.phyGateway, tcfg.phyIface); err != nil {
						c.logger.Warn("remove exclude route", zap.String("cidr", ec), zap.Error(err))
					}
				})
				c.logger.Debug("exclude route added", zap.String("cidr", cidr))
			}
		}
	}
	gateway := serverHello.GatewayIp
	if gateway == nil {
		gateway = computeGateway(serverHello.AssignedIp, tcfg.v4Mask.Mask)
	}
	if err := tunDev.SetGateway(gateway); err != nil {
		c.logger.Warn("set default route", zap.Error(err))
	} else {
		cleanups = append(cleanups, func() {
			if err := tunDev.RemoveGateway(gateway); err != nil {
				c.logger.Warn("remove default route", zap.Error(err))
			}
		})
		c.logger.Info("default route added", zap.String("gateway", gateway.String()))
	}
	return func() {
		for _, cl := range cleanups {
			cl()
		}
	}
}

// buildTunRouter builds the split-tunnel rule set and router (nil in proxy mode).
func (c *Client) buildTunRouter(stream tunnel.StreamConn, sessionCipher *crypto.SessionCipher, tunDev tun.TunDevice) (*routing.RuleSet, *routing.TunRouter) {
	if c.cfg.Routing == nil || c.cfg.Mode == "proxy" {
		return nil, nil
	}
	routeSet, err := routing.NewRuleSet(c.cfg.Routing, c.logger)
	if err != nil {
		c.logger.Warn("tun router init, forwarding all traffic through tunnel", zap.Error(err))
		return nil, nil
	}
	tunnelSend := func(packet []byte) error {
		payload := packet
		if sessionCipher != nil {
			encrypted, encErr := sessionCipher.Encrypt(payload)
			if encErr != nil {
				return encErr
			}
			payload = encrypted
		}
		f := framing.Frame{Type: framing.FrameTypeData, Flags: framing.FrameFlagNone, Payload: payload}
		data, encErr := f.Encode()
		if encErr != nil {
			return encErr
		}
		defer framing.ReturnBuffer(data)
		if err := stream.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
			return err
		}
		return stream.WriteMessage(data)
	}
	tunWrite := func(pkt []byte) (int, error) { return tunDev.Write(pkt) }
	tunRouter := routing.NewTunRouter(routeSet, tunDev.Read, tunWrite, tunnelSend, c.logger)
	c.logger.Info("split-tunnel routing enabled",
		zap.String("default", c.cfg.Routing.DefaultRoute),
		zap.Int("include_ranges", len(c.cfg.Routing.IncludeRanges)),
		zap.Int("exclude_ranges", len(c.cfg.Routing.ExcludeRanges)),
	)
	return routeSet, tunRouter
}

// setupDNSProxy starts the DNS proxy with domain tracking and returns its cleanup.
func (c *Client) setupDNSProxy(ctx context.Context, routeSet *routing.RuleSet, tunDev tun.TunDevice, tcfg *tunConfig) func() {
	rc := c.cfg.Routing
	if rc == nil || rc.DNSRouting == nil || !rc.DNSRouting.Enabled || routeSet == nil ||
		(len(rc.ExcludeDomains) == 0 && len(rc.IncludeDomains) == 0) {
		return nil
	}
	dnsBackup, dnsResolvers := setupDNS()

	tracker := dns.NewTracker(time.Duration(rc.DNSRouting.TTL) * time.Second)
	routeSet.SetTracker(tracker)
	dnsSrv := dnsproxy.New(c.cfg.DNSProxy.Listen, c.cfg.DNSProxy.Upstreams...)
	dnsSrv.SetTracker(tracker)
	dnsSrv.SetRouteFunc(func(domain string) bool {
		return routeSet.MatchDomain(domain) == routing.RouteDirect
	})
	if dnsBackup != nil {
		if len(dnsResolvers) == 0 && len(c.cfg.DNSProxy.Upstreams) > 0 {
			dnsSrv.SetOrigResolvers([]string{c.cfg.DNSProxy.Upstreams[0]})
		} else {
			dnsSrv.SetOrigResolvers(dnsResolvers)
		}
	} else if len(c.cfg.DNSProxy.Upstreams) > 0 {
		dnsSrv.SetOrigResolvers([]string{c.cfg.DNSProxy.Upstreams[0]})
	}
	if tcfg.havePhyRoute {
		dnsSrv.SetDirectRouteFunc(func(ips []netip.Addr) {
			for _, ip := range ips {
				if ip.IsPrivate() || ip.IsLoopback() {
					continue
				}
				cidr := ip.String() + "/32"
				if err := tunDev.AddExcludeRoute(cidr, tcfg.phyGateway, tcfg.phyIface); err != nil {
					c.logger.Warn("add dns direct route", zap.String("cidr", cidr), zap.Error(err))
				}
			}
		})
	}
	dnsCtx, dnsCancel := context.WithCancel(ctx)
	dnsReady := make(chan error, 1)
	go func() {
		dnsReady <- dnsSrv.Run(dnsCtx)
	}()
	select {
	case err := <-dnsReady:
		c.logger.Warn("dns cache: proxy failed to start, restoring dns", zap.Error(err))
		dnsCancel()
		dnsSrv = nil
		if dnsBackup != nil {
			restoreDNS(dnsBackup)
			dnsBackup = nil
		}
	case <-time.After(100 * time.Millisecond):
		dnsUpstreams := dnsResolvers
		if len(dnsUpstreams) == 0 && len(c.cfg.DNSProxy.Upstreams) > 0 {
			dnsUpstreams = c.cfg.DNSProxy.Upstreams
		}
		applyDNS(dnsBackup, tunDev, c.cfg.DNSProxy.Listen, tcfg.phyGateway, tcfg.phyIface, dnsUpstreams)
	}
	return func() {
		if dnsCancel != nil {
			dnsCancel()
		}
		if dnsSrv != nil {
			_ = dnsSrv.Shutdown()
		}
		if dnsBackup != nil {
			restoreDNS(dnsBackup)
		}
	}
}

// bindSecondaryChannel dials and binds the secondary WS channel when multi_channel is on.
func (c *Client) bindSecondaryChannel(ctx context.Context, tunSess *tunnel.Session, sessionID string) func() {
	if !c.cfg.MultiChannel || sessionID == "" {
		return nil
	}
	secondaryConn, batchOK, secErr := dialSecondaryChannel(ctx, c.cfg, c.logger, sessionID)
	if secErr != nil {
		c.logger.Warn("secondary channel unavailable, continuing on primary", zap.Error(secErr))
		return nil
	}
	tunSess.SetSecondary(secondaryConn)
	// @sk-task secondary-batching#T3.1: enable batching only after server confirmation (AC-001, AC-004)
	tunSess.SetBatchMode(batchOK)
	c.logger.Info("secondary channel bound",
		zap.String("session", sessionID),
		zap.String("server", c.cfg.Server),
		zap.Bool("batch", batchOK),
	)
	// @sk-task dual-ws-channel follow-up: re-establish the secondary channel if
	// it drops mid-session instead of silently losing UDP media (AC-004).
	tunSess.SetOnSecondaryLost(func() {
		backoff := time.Second
		for attempt := 1; attempt <= 5; attempt++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			conn, ok, derr := dialSecondaryChannel(ctx, c.cfg, c.logger, sessionID)
			if derr == nil {
				tunSess.SetSecondary(conn)
				tunSess.SetBatchMode(ok)
				c.logger.Info("secondary channel re-bound",
					zap.String("session", sessionID),
					zap.Int("attempt", attempt),
					zap.Bool("batch", ok),
				)
				return
			}
			c.logger.Warn("secondary re-bind failed, continuing on primary",
				zap.String("session", sessionID),
				zap.Int("attempt", attempt),
				zap.Error(derr),
			)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
		}
	})
	return func() { _ = secondaryConn.Close() }
}

// @sk-task dual-ws-channel#T3.2: dial secondary stream and complete secondary handshake (AC-001, AC-004)
// @sk-task secondary-batching#T3.1: declare batching capability and report server confirmation (AC-001)
func dialSecondaryChannel(ctx context.Context, cfg *config.ClientConfig, logger *zap.Logger, sessionID string) (conn tunnel.StreamConn, batchOK bool, err error) {
	// @sk-task game-latency#T3.3: secondary (UDP) channel requests unpadded framing (AC-001)
	stream, err := dialStreamWith(ctx, cfg, logger, realtimeUnpadded(cfg))
	if err != nil {
		return nil, false, fmt.Errorf("dial secondary: %w", err)
	}
	helloFrame, err := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Ipv6:         cfg.IPv6,
		Token:        cfg.Auth.Token,
		Mtu:          cfg.MTU,
		Channel:      "secondary",
		SessionId:    sessionID,
		BatchSupport: true,
	})
	if err != nil {
		_ = stream.Close()
		return nil, false, fmt.Errorf("encode secondary hello: %w", err)
	}
	helloData, err := helloFrame.Encode()
	if err != nil {
		_ = stream.Close()
		return nil, false, fmt.Errorf("encode secondary hello frame: %w", err)
	}
	if err := stream.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		_ = stream.Close()
		return nil, false, fmt.Errorf("send secondary hello: %w", err)
	}
	framing.ReturnBuffer(helloData)

	respData, err := stream.ReadMessage()
	if err != nil {
		_ = stream.Close()
		return nil, false, fmt.Errorf("read secondary server hello: %w", err)
	}
	var respFrame framing.Frame
	if err := respFrame.Decode(respData); err != nil {
		_ = stream.Close()
		return nil, false, fmt.Errorf("decode secondary response: %w", err)
	}
	switch respFrame.Type {
	case framing.FrameTypeAuth:
		authErr, _ := handshake.DecodeAuthError(&respFrame)
		_ = stream.Close()
		return nil, false, fmt.Errorf("secondary auth rejected: %s", authErr.Reason)
	case framing.FrameTypeHello:
		serverHello, err := handshake.DecodeServerHello(&respFrame)
		if err != nil {
			_ = stream.Close()
			return nil, false, fmt.Errorf("decode secondary server hello: %w", err)
		}
		if serverHello.SessionId != sessionID {
			_ = stream.Close()
			return nil, false, fmt.Errorf("secondary session id mismatch: %s != %s", serverHello.SessionId, sessionID)
		}
		return stream, serverHello.BatchSupport, nil
	default:
		_ = stream.Close()
		return nil, false, fmt.Errorf("unexpected secondary response type: %d", int(respFrame.Type))
	}
}
