package client

import (
	"context"
	"io"
	"net"
	"net/netip"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/bzdvdn/kvn-ws/src/internal/protocol/control"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/proxy"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
)

func (c *Client) runProxyMode(ctx context.Context) {
	tlsCfg, err := tls.NewClientTLSConfigFromSettings(tls.ClientTLSSettings{
		CAFile:     c.cfg.TLS.CAFile,
		ServerName: c.cfg.TLS.ServerName,
		VerifyMode: c.cfg.TLS.VerifyMode,
	})
	if err != nil {
		c.logger.Fatal("proxy tls config", zap.Error(err))
	}

	minBackoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	if c.cfg.Reconnect != nil {
		if c.cfg.Reconnect.MinBackoffSec > 0 {
			minBackoff = time.Duration(c.cfg.Reconnect.MinBackoffSec) * time.Second
		}
		if c.cfg.Reconnect.MaxBackoffSec > 0 {
			maxBackoff = time.Duration(c.cfg.Reconnect.MaxBackoffSec) * time.Second
		}
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
		c.logger.Info("connecting", zap.Int("attempt", attempt), zap.Duration("backoff", backoff))

		wsConn, err := websocket.Dial(c.cfg.Server, tlsCfg, c.logger, websocket.WSConfig{
			Compression: c.cfg.Compression,
			Multiplex:   c.cfg.Multiplex,
			MTU:         c.cfg.MTU,
		})
		if err != nil {
			c.logger.Warn("dial failed", zap.Error(err), zap.Duration("retry_in", backoff))
			sleepWithContext(ctx, backoff)
			backoff = nextBackoff(backoff, minBackoff, maxBackoff)
			continue
		}

		c.runProxySession(ctx, wsConn)
		backoff = minBackoff
	}
}

func (c *Client) runProxySession(ctx context.Context, wsConn *websocket.WSConn) {
	defer func() { _ = wsConn.Close() }()

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
		if err := wsConn.WriteMessage(helloData); err != nil {
			framing.ReturnBuffer(helloData)
			c.logger.Warn("send hello", zap.Error(err))
			return
		}
		framing.ReturnBuffer(helloData)

		respData, err := wsConn.ReadMessage()
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
	wsConn.SetKeepalive(control.DefaultPingInterval, control.DefaultPongTimeout)

	streamMgr := proxy.NewManager(wsConn)

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
			ipAddr := net.ParseIP(host)
			if ipAddr == nil {
				addrs, _ := net.DefaultResolver.LookupHost(context.Background(), host)
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
				c.logger.Debug("proxy direct", zap.String("dst", dst))
				go func() {
					defer func() { _ = client.Close() }()
					target, err := net.Dial("tcp", dst)
					if err != nil {
						return
					}
					defer func() { _ = target.Close() }()
					errc := make(chan error, 2)
					go func() { _, err := io.Copy(target, client); errc <- err }()
					go func() { _, err := io.Copy(client, target); errc <- err }()
					<-errc
				}()
				return
			}
		}

		stream := &proxy.Stream{
			ID:    proxy.NewStreamID(),
			Dst:   dst,
			Local: client,
		}
		streamMgr.Add(stream)

		go stream.ForwardToWS(wsConn)
	})

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
			data, err := wsConn.ReadMessage()
			if err != nil {
				c.logger.Warn("ws read error in proxy mode", zap.Error(err))
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

	<-gctx.Done()
	c.logger.Debug("proxy session stopping")
	_ = pl.Close()
	_ = wsConn.Close()

	if err := eg.Wait(); err != nil {
		c.logger.Debug("proxy session stopped", zap.Error(err))
	}
}
