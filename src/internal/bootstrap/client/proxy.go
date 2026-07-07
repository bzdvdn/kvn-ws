package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/bzdvdn/kvn-ws/src/internal/dns"
	"github.com/bzdvdn/kvn-ws/src/internal/dnsproxy"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/proxy"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
)

// @sk-task win-proxy-multi-conn#T2.1: N parallel transport connections, proxySlot type (AC-001)
type proxySlot struct {
	stream transport.StreamConn
	mgr    *proxy.Manager
}

// @sk-task arch-refactoring#T3.1: use common dialStream (AC-004)
// @sk-task win-proxy-multi-conn#T4.1: reconnect loop with config-driven connection count (AC-003)
func (c *Client) runProxyMode(ctx context.Context) {
	// @sk-task dns-cache-cleanup: clean stale resolv.conf from previous killed session
	dnsproxy.CleanupStaleDNS(c.cfg.DNSProxy.Listen)

	minBackoff, maxBackoff := parseBackoff(c.cfg.Reconnect)

	numConns := c.cfg.ProxyConnections
	if numConns <= 0 {
		numConns = 10
	}

	backoff := minBackoff
	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		attempt++
		c.logger.Info("connecting", zap.Int("attempt", attempt), zap.Duration("backoff", backoff), zap.Int("conns", numConns))

		slots := c.dialProxySlots(ctx, numConns)
		if slots == nil {
			c.logger.Warn("dial failed", zap.Duration("retry_in", backoff))
			sleepWithContext(ctx, backoff)
			backoff = nextBackoff(backoff, minBackoff, maxBackoff)
			continue
		}

		c.runProxySessionMulti(ctx, slots, c.cfg.Transparent)
		backoff = nextBackoff(backoff, minBackoff, maxBackoff)
	}
}

// @sk-task win-proxy-multi-conn#T2.3: dial N connections + handshake for each (AC-001)
func (c *Client) dialProxySlots(ctx context.Context, numConns int) []*proxySlot {
	slots := make([]*proxySlot, numConns)
	// Dial all connections sequentially so each gets a full handshake.
	// If any fails we close the ones we opened and return nil.
	for i := 0; i < numConns; i++ {
		stream, err := dialStream(ctx, c.cfg, c.logger)
		if err != nil {
			for j := 0; j < i; j++ {
				_ = slots[j].stream.Close()
			}
			return nil
		}
		if c.metricCollector != nil {
			stream = &transport.CountingStreamConn{
				StreamConn: stream,
				AddTX:      c.metricCollector.AddTX,
				AddRX:      c.metricCollector.AddRX,
			}
		}
		if !c.doHandshake(ctx, stream) {
			_ = stream.Close()
			for j := 0; j < i; j++ {
				_ = slots[j].stream.Close()
			}
			return nil
		}
		slots[i] = &proxySlot{stream: stream}
	}
	return slots
}

// doHandshake performs the client↔server hello exchange on a single transport stream.
// Returns false on failure.
// @sk-task win-proxy-multi-conn#T2.2: full ClientHello↔ServerHello handshake (AC-001)
func (c *Client) doHandshake(ctx context.Context, stream transport.StreamConn) bool {
	helloFrame, err := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Ipv6:         c.cfg.IPv6,
		Token:        c.cfg.Auth.Token,
		Mtu:          c.cfg.MTU,
	})
	if err != nil {
		c.logger.Warn("encode client hello", zap.Error(err))
		return false
	}
	helloData, err := helloFrame.Encode()
	if err != nil {
		c.logger.Warn("encode hello frame", zap.Error(err))
		return false
	}
	if err := stream.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		c.logger.Warn("send hello", zap.Error(err))
		return false
	}
	framing.ReturnBuffer(helloData)

	respData, err := stream.ReadMessage()
	if err != nil {
		c.logger.Warn("read server hello", zap.Error(err))
		return false
	}
	var respFrame framing.Frame
	if err := respFrame.Decode(respData); err != nil {
		c.logger.Warn("decode server hello", zap.Error(err))
		return false
	}
	switch respFrame.Type {
	case framing.FrameTypeAuth:
		authErr, _ := handshake.DecodeAuthError(&respFrame)
		c.logger.Warn("auth rejected", zap.String("reason", authErr.Reason))
		return false
	case framing.FrameTypeHello:
		serverHello, err := handshake.DecodeServerHello(&respFrame)
		if err != nil {
			c.logger.Warn("decode server hello", zap.Error(err))
			return false
		}
		c.logger.Info("handshake complete",
			zap.String("session", serverHello.SessionId),
			zap.String("assigned_ip", serverHello.AssignedIp.String()),
		)
		return true
	default:
		c.logger.Warn("unexpected handshake response", zap.Int("type", int(respFrame.Type)))
		return false
	}
}

// @sk-task win-proxy-multi-conn#T3.1: per-slot Manager (AC-001, AC-002)
// @sk-task win-proxy-multi-conn#T3.2: round-robin stream distribution in onConn (AC-002)
// @sk-task win-proxy-multi-conn#T3.4: DNS proxy bound to slot 0 (AC-006)
// @sk-task win-proxy-multi-conn#T3.5: QUIC keepalive on slot 0 (AC-007)
// @sk-task win-proxy-multi-conn#T3.6: teardown all slots on any error (AC-003)
func (c *Client) runProxySessionMulti(ctx context.Context, slots []*proxySlot, transparent bool) {
	// Close all streams on exit
	defer func() {
		for _, slot := range slots {
			_ = slot.stream.Close()
		}
	}()

	// Shared resources — routing, DNS tracker, DNS proxy
	var routeSet *routing.RuleSet
	if c.cfg.Routing != nil {
		rs, err := routing.NewRuleSet(c.cfg.Routing, c.logger)
		if err != nil {
			c.logger.Warn("routing init, using default", zap.Error(err))
		} else {
			routeSet = rs
		}
	}
	var dnsTracker *dns.Tracker
	if routeSet != nil && c.cfg.Routing != nil && c.cfg.Routing.DNSRouting != nil && c.cfg.Routing.DNSRouting.Enabled {
		dnsTracker = dns.NewTracker(time.Duration(c.cfg.Routing.DNSRouting.TTL) * time.Second)
		routeSet.SetTracker(dnsTracker)
	}

	var dnsCtx context.Context
	var dnsCancel context.CancelFunc
	var resolvBackup *dnsproxy.ResolvConfBackup
	if transparent {
		resolvBackup, _ = dnsproxy.BackupResolvConf()

		c.dnsSrv = dnsproxy.New(c.cfg.DNSProxy.Listen, c.cfg.DNSProxy.Upstreams...)
		if dnsTracker != nil {
			c.dnsSrv.SetTracker(dnsTracker)
		}
		if resolvBackup != nil {
			c.dnsSrv.SetOrigResolvers(resolvBackup.Nameservers())
		}
		if routeSet != nil {
			c.dnsSrv.SetRouteFunc(func(domain string) bool {
				return routeSet.MatchDomain(domain) == routing.RouteDirect
			})
		}

		dnsCtx, dnsCancel = context.WithCancel(ctx)
		dnsReady := make(chan error, 1)
		go func() {
			dnsReady <- c.dnsSrv.Run(dnsCtx)
		}()
		select {
		case err := <-dnsReady:
			c.logger.Warn("dns proxy failed to start, restoring resolv.conf", zap.Error(err))
			dnsCancel()
			c.dnsSrv = nil
			if resolvBackup != nil {
				_ = resolvBackup.Restore()
				resolvBackup = nil
			}
		case <-time.After(100 * time.Millisecond):
			if resolvBackup != nil {
				_ = dnsproxy.OverrideResolvConf(c.cfg.DNSProxy.Listen)
			}
			// Use the first slot's stream for DNS responses
			c.dnsSrv.SetStream(slots[0].stream)
		}
		defer func() {
			if c.dnsSrv != nil {
				c.dnsSrv.ClearStream()
			}
			if dnsCancel != nil {
				dnsCancel()
			}
			if c.dnsSrv != nil {
				_ = c.dnsSrv.Shutdown()
				c.dnsSrv = nil
			}
			if resolvBackup != nil {
				if err := resolvBackup.Restore(); err != nil {
					c.logger.Warn("dns proxy: failed to restore resolv.conf", zap.Error(err))
				}
			}
		}()
	}

	// Create one Manager per slot
	for i, slot := range slots {
		slot.mgr = proxy.NewManager(slot.stream, func(format string, args ...any) {
			c.logger.Warn(fmt.Sprintf(format, args...))
		})
		slots[i] = slot
	}

	var proxyAuth *proxy.ProxyAuth
	if c.cfg.ProxyAuth != nil {
		proxyAuth = &proxy.ProxyAuth{Username: c.cfg.ProxyAuth.Username, Password: c.cfg.ProxyAuth.Password}
	}
	listenAddr := c.cfg.ProxyListen
	if transparent {
		_, port, err := net.SplitHostPort(listenAddr)
		if err == nil {
			listenAddr = "0.0.0.0:" + port
		}
	}

	// Round-robin across slots
	var nextSlot atomic.Uint64
	numSlots := uint64(len(slots))

	pl := proxy.NewListener(listenAddr, proxyAuth, func(client net.Conn, dst string) {
		c.logger.Debug("proxy onconn", zap.String("dst", dst))
		if routeSet != nil {
			host, _, err := net.SplitHostPort(dst)
			if err != nil {
				host = dst
			}
			if action := routeSet.MatchDomain(host); action == routing.RouteDirect {
				c.logger.Info("proxy domain direct", zap.String("dst", dst), zap.String("ip", dst))
				go func() {
					defer func() { _ = client.Close() }()
					target, err := net.Dial("tcp", dst)
					if err != nil {
						c.logger.Warn("route direct dial failed", zap.String("dst", dst), zap.Error(err))
						return
					}
					defer func() { _ = target.Close() }()
					gctx, cancel := context.WithCancel(ctx)
					defer cancel()
					go func() {
						<-gctx.Done()
						_ = target.Close()
						_ = client.Close()
					}()
					eg, _ := errgroup.WithContext(gctx)
					eg.Go(func() error {
						_, err := io.Copy(target, client)
						return err
					})
					eg.Go(func() error {
						_, err := io.Copy(client, target)
						return err
					})
					_ = eg.Wait()
				}()
				return
			}
			ipAddr := net.ParseIP(host)
			if ipAddr == nil {
				addrs, _ := net.DefaultResolver.LookupHost(ctx, host)
				if len(addrs) > 0 {
					ipAddr = net.ParseIP(addrs[0])
				}
				if dnsTracker != nil && len(addrs) > 0 {
					var ips []netip.Addr
					for _, a := range addrs {
						if ip, err := netip.ParseAddr(a); err == nil {
							ips = append(ips, ip)
						}
					}
					if len(ips) > 0 {
						dnsTracker.Track(host, ips)
					}
				}
			}
			var nip netip.Addr
			if ipAddr != nil {
				if v4 := ipAddr.To4(); v4 != nil {
					nip, _ = netip.AddrFromSlice(v4)
				} else {
					nip, _ = netip.AddrFromSlice(ipAddr)
				}
			}
			if nip.IsValid() && routeSet.Route(nip) == routing.RouteDirect {
				c.logger.Info("proxy direct", zap.String("dst", dst), zap.String("ip", dst))
				go func() {
					defer func() { _ = client.Close() }()
					target, err := net.Dial("tcp", dst)
					if err != nil {
						c.logger.Warn("route direct dial failed", zap.String("dst", dst), zap.Error(err))
						return
					}
					defer func() { _ = target.Close() }()
					gctx, cancel := context.WithCancel(ctx)
					defer cancel()
					go func() {
						<-gctx.Done()
						_ = target.Close()
						_ = client.Close()
					}()
					eg, _ := errgroup.WithContext(gctx)
					eg.Go(func() error {
						_, err := io.Copy(target, client)
						return err
					})
					eg.Go(func() error {
						_, err := io.Copy(client, target)
						return err
					})
					_ = eg.Wait()
				}()
				return
			}
		}

		// Pick a slot by round-robin
		idx := nextSlot.Add(1) % numSlots
		slot := slots[idx]

		s := &proxy.Stream{
			ID:    proxy.NewStreamID(),
			Dst:   dst,
			Local: client,
		}
		slot.mgr.Add(s)

		go func() {
			s.ForwardToStream(slot.stream)
			slot.mgr.Remove(s.ID)
		}()
	}, c.cfg.ProxyMaxConcurrency)

	if transparent {
		pl.SetTransparent(true)
		pl.SetLogFn(func(format string, args ...any) {
			c.logger.Debug(fmt.Sprintf(format, args...))
		})
	}

	if err := pl.Start(); err != nil {
		c.logger.Warn("proxy start", zap.Error(err))
		return
	}

	c.logger.Info("proxy session started",
		zap.String("listen", pl.Addr().String()),
		zap.Int("slots", len(slots)),
	)

	eg, gctx := errgroup.WithContext(ctx)

	// Accept loop
	eg.Go(func() error {
		if err := pl.AcceptLoop(); err != nil {
			return err
		}
		return nil
	})

	// Read-loops — one per slot
	for _, slot := range slots {
		slot := slot
		eg.Go(func() error {
			return proxyReadLoop(gctx, slot.stream, slot.mgr, c)
		})
	}

	// QUIC keepalive — one per session is enough, run on slot 0
	if c.cfg.Transport == "quic" {
		eg.Go(func() error {
			pingTicker := time.NewTicker(25 * time.Second)
			defer pingTicker.Stop()
			stream := slots[0].stream
			for {
				select {
				case <-gctx.Done():
					return nil
				case <-pingTicker.C:
					f := framing.Frame{
						Type:  framing.FrameTypeProxy,
						Flags: framing.FrameFlagNone,
					}
					_ = stream.SetWriteDeadline(time.Now().Add(10 * time.Second))
					data, err := f.Encode()
					if err != nil {
						c.logger.Warn("keepalive encode error", zap.Error(err))
						_ = stream.SetWriteDeadline(time.Time{})
						continue
					}
					if err := stream.WriteMessage(data); err != nil {
						framing.ReturnBuffer(data)
						_ = stream.SetWriteDeadline(time.Time{})
						c.logger.Warn("keepalive: connection lost", zap.Error(err))
						return fmt.Errorf("keepalive: %w", err)
					}
					framing.ReturnBuffer(data)
					_ = stream.SetWriteDeadline(time.Time{})
				}
			}
		})
	}

	<-gctx.Done()
	c.logger.Debug("proxy session stopping")
	_ = pl.Close()
	for _, slot := range slots {
		_ = slot.stream.Close()
	}

	if err := eg.Wait(); err != nil {
		c.logger.Debug("proxy session stopped", zap.Error(err))
	}
}

// @sk-task win-proxy-multi-conn#T3.3: per-slot read-loop (AC-001)
func proxyReadLoop(ctx context.Context, stream transport.StreamConn, mgr *proxy.Manager, c *Client) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := stream.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			return err
		}
		data, err := stream.ReadMessage()
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			c.logger.Warn("stream read error in proxy mode", zap.Error(err))
			return err
		}
		var f framing.Frame
		if err := f.Decode(data); err != nil {
			c.logger.Warn("frame decode error", zap.Error(err))
			continue
		}
		switch f.Type {
		case framing.FrameTypeProxy:
			c.logger.Debug("proxy frame received", zap.Int("payload_len", len(f.Payload)))
			mgr.HandleIncomingFrame(&f)
		case framing.FrameTypeDNS:
			if c.dnsSrv != nil {
				c.logger.Debug("dns frame received", zap.Int("payload_len", len(f.Payload)))
				payload := f.Payload
				if len(payload) >= 4 {
					streamID := binary.BigEndian.Uint32(payload[0:4])
					c.dnsSrv.HandleDNSResponse(streamID, payload[4:])
				}
			}
		default:
			c.logger.Debug("skipping non-proxy frame", zap.Int("type", int(f.Type)))
		}
		f.Release()
	}
}
