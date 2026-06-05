package client

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/control"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	quictp "github.com/bzdvdn/kvn-ws/src/internal/transport/quic"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
	"github.com/bzdvdn/kvn-ws/src/internal/tunnel"
)

func (c *Client) reconnectLoop(ctx context.Context, tunDev tun.TunDevice) {
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

	tlsCfg, err := tls.NewClientTLSConfigFromSettings(tls.ClientTLSSettings{
		CAFile:     c.cfg.TLS.CAFile,
		ServerName: c.cfg.TLS.ServerName,
		VerifyMode: c.cfg.TLS.VerifyMode,
	})
	if err != nil {
		c.logger.Fatal("client tls config", zap.Error(err))
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

		var stream tunnel.StreamConn
		transport := c.cfg.Transport
		if transport == "" {
			transport = "tcp"
		}

		if transport == "quic" {
			c.logger.Info("dialing with QUIC transport")
			quicConn, err := quictp.Dial(c.cfg.Server, tlsCfg, nil)
			if err != nil {
				c.logger.Warn("QUIC dial failed, falling back to TCP", zap.Error(err))
				transport = "tcp"
			} else {
				stream = quicConn
			}
		}

		if transport != "quic" {
			wsCfg := websocket.WSConfig{
				Compression: c.cfg.Compression,
				Multiplex:   c.cfg.Multiplex,
				MTU:         c.cfg.MTU,
			}
			wsConn, err := websocket.Dial(c.cfg.Server, tlsCfg, c.logger, wsCfg)
			if err != nil {
				c.logger.Warn("dial failed", zap.Error(err), zap.Duration("retry_in", backoff))
				applyKillSwitch(c.cfg, c.logger)
				sleepWithContext(ctx, backoff)
				backoff = nextBackoff(backoff, minBackoff, maxBackoff)
				continue
			}
			wsConn.SetKeepalive(control.DefaultPingInterval, control.DefaultPongTimeout)
			stream = wsConn
		}

		removeKillSwitch(c.cfg, c.logger)
		backoff = minBackoff

		c.runSession(ctx, tunDev, stream)
	}
}

func (c *Client) runSession(ctx context.Context, tunDev tun.TunDevice, stream tunnel.StreamConn) {
	defer func() { _ = stream.Close() }()

	helloFrame, err := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		IPv6:         c.cfg.IPv6,
		Token:        c.cfg.Auth.Token,
		MTU:          c.cfg.MTU,
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

	switch respFrame.Type {
	case framing.FrameTypeAuth:
		authErr, _ := handshake.DecodeAuthError(&respFrame)
		c.logger.Fatal("auth rejected", zap.String("reason", authErr.Reason))
	case framing.FrameTypeHello:
		serverHello, err := handshake.DecodeServerHello(&respFrame)
		sessionID = serverHello.SessionID
		if err != nil {
			c.logger.Error("decode server hello", zap.Error(err))
			return
		}
		c.logger.Info("handshake complete",
			zap.String("session", serverHello.SessionID),
			zap.String("ip", serverHello.AssignedIP.String()),
		)
		mask := &net.IPNet{
			IP:   serverHello.AssignedIP,
			Mask: net.CIDRMask(24, 32),
		}
		if err := tunDev.SetIP(serverHello.AssignedIP, mask); err != nil {
			c.logger.Error("set tun ip", zap.Error(err))
			return
		}
		if serverHello.AssignedIPv6 != nil {
			c.logger.Info("assigned IPv6", zap.String("ip6", serverHello.AssignedIPv6.String()))
			v6Mask := &net.IPNet{
				IP:   serverHello.AssignedIPv6,
				Mask: net.CIDRMask(112, 128),
			}
			if err := tunDev.SetIP(serverHello.AssignedIPv6, v6Mask); err != nil {
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
			sessionCipher, err = crypto.NewSessionCipher(c.masterKey, serverHello.CryptoSalt, serverHello.SessionID)
			if err != nil {
				c.logger.Error("session cipher init", zap.Error(err))
				return
			}
			c.logger.Info("app-layer encryption active")
		} else if len(c.masterKey) > 0 && len(serverHello.CryptoSalt) == 0 {
			c.logger.Warn("server did not send crypto salt, connection will be unencrypted")
		}

		phyGateway, phyIface, pErr := tun.SaveDefaultRoute()
		if pErr != nil {
			c.logger.Warn("save default route (bypass routes won't be added)", zap.Error(pErr))
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
				}
			}
			if c.cfg.Routing != nil {
				for _, r := range c.cfg.Routing.ExcludeRanges {
					if r != "0.0.0.0/0" && r != "::/0" {
						excludeCIDRs = append(excludeCIDRs, r)
					}
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

		gateway := serverHello.GatewayIP
		if gateway == nil {
			gateway = computeGateway(serverHello.AssignedIP, mask.Mask)
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

	var tunRouter *routing.TunRouter
	if c.cfg.Routing != nil && c.cfg.Mode != "proxy" {
		rs, err := routing.NewRuleSet(c.cfg.Routing, c.logger)
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
			tunRouter = routing.NewTunRouter(rs, tunDev.Read, tunWrite, tunnelSend, c.logger)
			c.logger.Info("split-tunnel routing enabled",
				zap.String("default", c.cfg.Routing.DefaultRoute),
				zap.Int("include_ranges", len(c.cfg.Routing.IncludeRanges)),
				zap.Int("exclude_ranges", len(c.cfg.Routing.ExcludeRanges)),
			)
		}
	}

	tunSess := tunnel.NewSession(tunDev, stream, nil, sessionID, "", nil, nil, nil, c.logger, sessionCipher, nil)
	if tunRouter != nil {
		tunSess.SetTunRouter(tunRouter)
	}
	tunSess.SetInterruptibleRead(true)

	if err := tunSess.Run(ctx); err != nil {
		c.logger.Info("session ended", zap.Error(err))
	}
}
