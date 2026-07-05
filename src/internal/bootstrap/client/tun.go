package client

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	"github.com/bzdvdn/kvn-ws/src/internal/dns"
	"github.com/bzdvdn/kvn-ws/src/internal/dnsproxy"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
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

func (c *Client) runSession(ctx context.Context, tunDev tun.TunDevice, stream tunnel.StreamConn) {
	defer func() { _ = stream.Close() }()
	// @sk-task dns-response-tracker#T3.5: CleanupExcludeRoutes — remove kernel routes on disconnect
	defer tunDev.CleanupExcludeRoutes()

	helloFrame, err := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Ipv6:         c.cfg.IPv6,
		Token:        c.cfg.Auth.Token,
		Mtu:          c.cfg.MTU,
	})
	if err != nil {
		c.logger.Error("encode client hello", zap.Error(err))
		return
	}
	helloData, err := helloFrame.Encode()
	if err != nil {
		c.logger.Error("encode hello frame", zap.Error(err))
		return
	}
	if err := stream.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		c.logger.Error("send hello", zap.Error(err))
		return
	}
	framing.ReturnBuffer(helloData)

	respData, err := stream.ReadMessage()
	if err != nil {
		c.logger.Error("read server hello", zap.Error(err))
		return
	}
	var respFrame framing.Frame
	if err := respFrame.Decode(respData); err != nil {
		c.logger.Error("decode response frame", zap.Error(err))
		return
	}

	var sessionCipher *crypto.SessionCipher
	var sessionID string
	var phyGateway net.IP
	var phyIface string
	havePhyRoute := false

	switch respFrame.Type {
	case framing.FrameTypeAuth:
		authErr, _ := handshake.DecodeAuthError(&respFrame)
		c.logger.Fatal("auth rejected", zap.String("reason", authErr.Reason))
	case framing.FrameTypeHello:
		serverHello, err := handshake.DecodeServerHello(&respFrame)
		sessionID = serverHello.SessionId
		if err != nil {
			c.logger.Error("decode server hello", zap.Error(err))
			return
		}
		c.logger.Info("handshake complete",
			zap.String("session", serverHello.SessionId),
			zap.String("ip", serverHello.AssignedIp.String()),
		)
		mask := &net.IPNet{
			IP:   serverHello.AssignedIp,
			Mask: net.CIDRMask(tun.CIDRMaskV4Bits, tun.CIDRMaskV4Total),
		}
		if err := tunDev.SetIP(serverHello.AssignedIp, mask); err != nil {
			c.logger.Error("set tun ip", zap.Error(err))
			return
		}
		if serverHello.AssignedIpv6 != nil {
			c.logger.Info("assigned IPv6", zap.String("ip6", serverHello.AssignedIpv6.String()))
			v6Mask := &net.IPNet{
				IP:   serverHello.AssignedIpv6,
				Mask: net.CIDRMask(tun.CIDRMaskV6Bits, tun.CIDRMaskV6Total),
			}
			if err := tunDev.SetIP(serverHello.AssignedIpv6, v6Mask); err != nil {
				c.logger.Error("set tun ipv6", zap.Error(err))
				return
			}
		}
		if c.cfg.MTU > 0 {
			if err := tunDev.SetMTU(c.cfg.MTU); err != nil {
				c.logger.Warn("set tun mtu", zap.Int("mtu", c.cfg.MTU), zap.Error(err))
			}
		}
		if err := tunDev.DisableGSO(); err != nil {
			c.logger.Warn("disable gso", zap.Error(err))
		} else {
			c.logger.Info("gso/gro disabled on tun")
		}
		if len(c.masterKey) > 0 && len(serverHello.CryptoSalt) > 0 {
			sessionCipher, err = crypto.NewSessionCipher(c.masterKey, serverHello.CryptoSalt, serverHello.SessionId)
			if err != nil {
				c.logger.Error("session cipher init", zap.Error(err))
				return
			}
			c.logger.Info("app-layer encryption active")
		} else if len(c.masterKey) > 0 && len(serverHello.CryptoSalt) == 0 {
			c.logger.Warn("server did not send crypto salt, connection will be unencrypted")
		}

		var pErr error
		phyGateway, phyIface, pErr = tun.SaveDefaultRoute()
		if pErr != nil {
			c.logger.Warn("save default route (bypass routes won't be added)", zap.Error(pErr))
		} else {
			havePhyRoute = true
		}
		var excludeCIDRs []string
		if pErr == nil {
			u, uErr := url.Parse(c.cfg.Server)
			if uErr == nil {
				host := u.Hostname()
				if ip := net.ParseIP(host); ip != nil {
					bits := 32
					if ip.To4() == nil {
						bits = 128
					}
					excludeCIDRs = append(excludeCIDRs, host+"/"+strconv.Itoa(bits))
				} else {
					if resolved := resolveServerIP(host); resolved != nil {
						bits := 32
						if resolved.To4() == nil {
							bits = 128
						}
						excludeCIDRs = append(excludeCIDRs, resolved.String()+"/"+strconv.Itoa(bits))
					}
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
			var routeCleanups []func()
			for _, cidr := range excludeCIDRs {
				if err := tunDev.AddExcludeRoute(cidr, phyGateway, phyIface); err != nil {
					c.logger.Warn("add exclude route", zap.String("cidr", cidr), zap.Error(err))
				} else {
					ec := cidr
					routeCleanups = append(routeCleanups, func() {
						if err := tunDev.RemoveExcludeRoute(ec, phyGateway, phyIface); err != nil {
							c.logger.Warn("remove exclude route", zap.String("cidr", ec), zap.Error(err))
						}
					})
					c.logger.Debug("exclude route added", zap.String("cidr", cidr))
				}
			}
			defer func() {
				for _, cleanup := range routeCleanups {
					cleanup()
				}
			}()
		}

		gateway := serverHello.GatewayIp
		if gateway == nil {
			gateway = computeGateway(serverHello.AssignedIp, mask.Mask)
		}
		if err := tunDev.SetGateway(gateway); err != nil {
			c.logger.Warn("set default route", zap.Error(err))
		} else {
			defer func() {
				if err := tunDev.RemoveGateway(gateway); err != nil {
					c.logger.Warn("remove default route", zap.Error(err))
				}
			}()
			c.logger.Info("default route added", zap.String("gateway", gateway.String()))
		}
	default:
		c.logger.Error("unexpected response type", zap.Int("type", int(respFrame.Type)))
		return
	}

	var routeSet *routing.RuleSet
	var tunRouter *routing.TunRouter
	if c.cfg.Routing != nil && c.cfg.Mode != "proxy" {
		var err error
		routeSet, err = routing.NewRuleSet(c.cfg.Routing, c.logger)
		if err != nil {
			c.logger.Warn("tun router init, forwarding all traffic through tunnel", zap.Error(err))
		} else {
			tunnelSend := func(packet []byte) error {
				payload := packet
				if sessionCipher != nil {
					encrypted, encErr := sessionCipher.Encrypt(payload)
					if encErr != nil {
						return encErr
					}
					payload = encrypted
				}
				f := framing.Frame{
					Type:    framing.FrameTypeData,
					Flags:   framing.FrameFlagNone,
					Payload: payload,
				}
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
			tunRouter = routing.NewTunRouter(routeSet, tunDev.Read, tunWrite, tunnelSend, c.logger)
			c.logger.Info("split-tunnel routing enabled",
				zap.String("default", c.cfg.Routing.DefaultRoute),
				zap.Int("include_ranges", len(c.cfg.Routing.IncludeRanges)),
				zap.Int("exclude_ranges", len(c.cfg.Routing.ExcludeRanges)),
			)
		}
	}

	// @sk-task dns-response-tracker#T3.2: DNS proxy + Tracker for suffix domain tracking (AC-003)
	var dnsSrv *dnsproxy.Server
	var dnsCtx context.Context
	var dnsCancel context.CancelFunc
	var resolvBackup *dnsproxy.ResolvConfBackup
	if c.cfg.Routing != nil && c.cfg.Routing.DNSCache != nil && c.cfg.Routing.DNSCache.Enabled && routeSet != nil {
		hasSuffix := false
		for _, d := range c.cfg.Routing.ExcludeDomains {
			if strings.HasPrefix(d, ".") {
				hasSuffix = true
				break
			}
		}
		if !hasSuffix {
			for _, d := range c.cfg.Routing.IncludeDomains {
				if strings.HasPrefix(d, ".") {
					hasSuffix = true
					break
				}
			}
		}
		if hasSuffix {
			tracker := dns.NewTracker(time.Duration(c.cfg.Routing.DNSCache.TTL) * time.Second)
			routeSet.SetTracker(tracker)
			resolvBackup, _ = dnsproxy.BackupResolvConf()
			// @sk-task dns-upstreams-list#T3.1: pass Upstreams slice (AC-001)
			dnsSrv = dnsproxy.New(c.cfg.DNSProxy.Listen, c.cfg.DNSProxy.Upstreams...)
			dnsSrv.SetTracker(tracker)
			// @sk-task dns-response-tracker#T3.5: SetRouteFunc for TUN mode (was missing — all DNS went TCP upstream)
			dnsSrv.SetRouteFunc(func(domain string) bool {
				return routeSet.MatchDomain(domain) == routing.RouteDirect
			})
			// @sk-task dns-response-tracker#T3.5: loopback filter + upstream fallback (systemd-resolved DNS loop fix)
			if resolvBackup != nil {
				// Filter out loopback resolvers (systemd-resolved) to avoid DNS loop
				allNss := resolvBackup.Nameservers()
				var resolvers []string
				for _, ns := range allNss {
					host, _, err := net.SplitHostPort(ns)
					if err != nil {
						host = ns
					}
					ip := net.ParseIP(host)
					if ip != nil && ip.IsLoopback() {
						continue
					}
					resolvers = append(resolvers, ns)
				}
				// If all resolvers were loopback, use the first upstream DNS instead
				if len(resolvers) == 0 && len(c.cfg.DNSProxy.Upstreams) > 0 {
					resolvers = append(resolvers, c.cfg.DNSProxy.Upstreams[0])
				}
				dnsSrv.SetOrigResolvers(resolvers)
				// Add exclude routes for public resolvers so DNS proxy bypasses TUN.
				// Private resolvers (corporate DNS behind openfortivpn) route through ppp0 naturally.
				if havePhyRoute {
					for _, ns := range resolvers {
						host, _, err := net.SplitHostPort(ns)
						if err != nil {
							host = ns
						}
						ip := net.ParseIP(host)
						if ip == nil || ip.IsPrivate() || ip.IsLoopback() {
							continue
						}
						bits := 32
						if ip.To4() == nil {
							bits = 128
						}
						cidr := host + "/" + strconv.Itoa(bits)
						if err := tunDev.AddExcludeRoute(cidr, phyGateway, phyIface); err != nil {
							c.logger.Warn("add dns resolver exclude route", zap.String("cidr", cidr), zap.Error(err))
						}
					}
				}
			}
			// @sk-task dns-response-tracker#T3.5: SetDirectRouteFunc + private/loopback skip (corporate IPs behind ppp0)
			if havePhyRoute {
				dnsSrv.SetDirectRouteFunc(func(ips []netip.Addr) {
					for _, ip := range ips {
						if ip.IsPrivate() || ip.IsLoopback() {
							continue
						}
						cidr := ip.String() + "/32"
						if err := tunDev.AddExcludeRoute(cidr, phyGateway, phyIface); err != nil {
							c.logger.Warn("add dns direct route", zap.String("cidr", cidr), zap.Error(err))
						}
					}
				})
			}
			dnsCtx, dnsCancel = context.WithCancel(ctx)
			dnsReady := make(chan error, 1)
			go func() {
				dnsReady <- dnsSrv.Run(dnsCtx)
			}()
			select {
			case err := <-dnsReady:
				c.logger.Warn("dns cache: proxy failed to start, restoring resolv.conf", zap.Error(err))
				dnsCancel()
				dnsSrv = nil
				if resolvBackup != nil {
					_ = resolvBackup.Restore()
					resolvBackup = nil
				}
			case <-time.After(100 * time.Millisecond):
				if resolvBackup != nil {
					_ = dnsproxy.OverrideResolvConf(c.cfg.DNSProxy.Listen)
				}
			}
			defer func() {
				if dnsCancel != nil {
					dnsCancel()
				}
				if dnsSrv != nil {
					_ = dnsSrv.Shutdown()
					dnsSrv = nil
				}
				if resolvBackup != nil {
					if err := resolvBackup.Restore(); err != nil {
						c.logger.Warn("dns cache: failed to restore resolv.conf", zap.Error(err))
					}
				}
			}()
		}
	}

	tunSess := tunnel.NewSession(tunDev, stream, nil, sessionID, "", nil, nil, nil, c.logger, sessionCipher, nil,
		time.Duration(c.cfg.TunnelTimeout)*time.Second, c.cfg.ProxyMaxConcurrency, nil, nil, nil)
	if tunRouter != nil {
		tunSess.SetTunRouter(tunRouter)
	}
	if err := tunSess.Run(ctx); err != nil {
		c.logger.Info("session ended", zap.Error(err))
	}
}
