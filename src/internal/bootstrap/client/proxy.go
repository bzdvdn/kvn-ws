package client

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/proxy"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
)

// @sk-task arch-refactoring#T3.1: use common dialStream (AC-004)
func (c *Client) runProxyMode(ctx context.Context) {
	minBackoff, maxBackoff := parseBackoff(c.cfg.Reconnect)

	backoff := minBackoff
	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		attempt++
		c.logger.Info("connecting", zap.Int("attempt", attempt), zap.Duration("backoff", backoff))

		stream, err := dialStream(ctx, c.cfg, c.logger)
		if err != nil {
			c.logger.Warn("dial failed", zap.Error(err), zap.Duration("retry_in", backoff))
			sleepWithContext(ctx, backoff)
			backoff = nextBackoff(backoff, minBackoff, maxBackoff)
			continue
		}

		c.runProxySession(ctx, stream)
		backoff = nextBackoff(backoff, minBackoff, maxBackoff)
	}
}

// @sk-task quic-proxy-mode#T2.2: runProxySession takes proxy.StreamConn (AC-001, AC-002, AC-003)
func (c *Client) runProxySession(ctx context.Context, stream proxy.StreamConn) {
	defer func() { _ = stream.Close() }()

	{
		helloFrame, err := handshake.EncodeClientHello(&handshake.ClientHello{
			ProtoVersion: handshake.ProtoVersion,
			IPv6:         c.cfg.IPv6,
			Token:        c.cfg.Auth.Token,
			MTU:          c.cfg.MTU,
		})
		if err != nil {
			c.logger.Warn("encode client hello", zap.Error(err))
			return
		}
		helloData, err := helloFrame.Encode()
		if err != nil {
			c.logger.Warn("encode hello frame", zap.Error(err))
			return
		}
		if err := stream.WriteMessage(helloData); err != nil {
			framing.ReturnBuffer(helloData)
			c.logger.Warn("send hello", zap.Error(err))
			return
		}
		framing.ReturnBuffer(helloData)

		respData, err := stream.ReadMessage()
		if err != nil {
			c.logger.Warn("read server hello", zap.Error(err))
			return
		}
		var respFrame framing.Frame
		if err := respFrame.Decode(respData); err != nil {
			c.logger.Warn("decode server hello", zap.Error(err))
			return
		}
		switch respFrame.Type {
		case framing.FrameTypeAuth:
			authErr, _ := handshake.DecodeAuthError(&respFrame)
			c.logger.Warn("auth rejected", zap.String("reason", authErr.Reason))
			return
		case framing.FrameTypeHello:
			serverHello, err := handshake.DecodeServerHello(&respFrame)
			if err != nil {
				c.logger.Warn("decode server hello", zap.Error(err))
				return
			}
			c.logger.Info("handshake complete",
				zap.String("session", serverHello.SessionID),
				zap.String("assigned_ip", serverHello.AssignedIP.String()),
			)
		default:
			c.logger.Warn("unexpected handshake response", zap.Int("type", int(respFrame.Type)))
			return
		}
	}

	streamMgr := proxy.NewManager(stream)

	var routeSet *routing.RuleSet
	if c.cfg.Routing != nil {
		rs, err := routing.NewRuleSet(c.cfg.Routing, c.logger)
		if err != nil {
			c.logger.Warn("routing init, using default", zap.Error(err))
		} else {
			routeSet = rs
		}
	}

	var proxyAuth *proxy.ProxyAuth
	if c.cfg.ProxyAuth != nil {
		proxyAuth = &proxy.ProxyAuth{Username: c.cfg.ProxyAuth.Username, Password: c.cfg.ProxyAuth.Password}
	}
	pl := proxy.NewListener(c.cfg.ProxyListen, proxyAuth, func(client net.Conn, dst string) {
		if routeSet != nil {
			host, _, err := net.SplitHostPort(dst)
			if err != nil {
				host = dst
			}
			// @sk-task dns-routing#T6.1: check suffix domains first (AC-001, AC-002)
			// @sk-task fix-critical-leaks#T3.3: RouteDirect lifecycle via errgroup (AC-003)
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
						target.Close()
						client.Close()
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
					eg.Wait()
				}()
				return
			}
			ipAddr := net.ParseIP(host)
			if ipAddr == nil {
				// @sk-task fix-critical-leaks#T2.2: DNS ctx propagation (AC-005)
			addrs, _ := net.DefaultResolver.LookupHost(ctx, host)
				if len(addrs) > 0 {
					ipAddr = net.ParseIP(addrs[0])
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
				// @sk-task fix-critical-leaks#T3.3: RouteDirect lifecycle via errgroup (AC-003)
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
						target.Close()
						client.Close()
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
					eg.Wait()
				}()
				return
			}
		}

		s := &proxy.Stream{
			ID:    proxy.NewStreamID(),
			Dst:   dst,
			Local: client,
		}
		streamMgr.Add(s)

		go func() {
			s.ForwardToStream(stream)
			streamMgr.Remove(s.ID)
		}()
	}, c.cfg.ProxyMaxConcurrency)

	if err := pl.Start(); err != nil {
		c.logger.Warn("proxy start", zap.Error(err))
		return
	}

	c.logger.Info("proxy session started", zap.String("listen", pl.Addr().String()))

	eg, gctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		if err := pl.AcceptLoop(); err != nil {
			return err
		}
		return nil
	})

	eg.Go(func() error {
		for {
			select {
			case <-gctx.Done():
				return gctx.Err()
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
			if f.Type == framing.FrameTypeProxy {
				c.logger.Debug("proxy frame received", zap.Int("payload_len", len(f.Payload)))
				streamMgr.HandleIncomingFrame(&f)
			} else {
				c.logger.Debug("skipping non-proxy frame", zap.Int("type", int(f.Type)))
			}
			f.Release()
		}
	})

	// @sk-task fix-ping-drops#T3.1: QUIC keepalive — send periodic proxy frames to prevent server stream read deadline expiry
	if c.cfg.Transport == "quic" {
		eg.Go(func() error {
			pingTicker := time.NewTicker(25 * time.Second)
			defer pingTicker.Stop()
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
						c.logger.Warn("keepalive write error", zap.Error(err))
						framing.ReturnBuffer(data)
						_ = stream.SetWriteDeadline(time.Time{})
						continue
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
	_ = stream.Close()

	if err := eg.Wait(); err != nil {
		c.logger.Debug("proxy session stopped", zap.Error(err))
	}
}
