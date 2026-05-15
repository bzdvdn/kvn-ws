package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/netip"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/control"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/proxy"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// @sk-task foundation#T1.1: Go module init (AC-001)
// @sk-task foundation#T3.2: client main with graceful shutdown (AC-010)
// @sk-task core-tunnel-mvp#T4.1: client forwarding loops (AC-007, AC-008)
// @sk-task core-tunnel-mvp#T4.2: graceful shutdown (AC-010)
// @sk-task production-hardening#T4.2: reconnect + kill-switch (AC-001, AC-003)
// @sk-task production-hardening#T4.3: pflag CLI (AC-011)
func main() {
	cfgPath := pflag.String("config", "configs/client.yaml", "path to config file")
	pflag.Parse()

	cfg, err := config.LoadClientConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	logger, _, err := logger.New(cfg.Log.Level) //nolint: forbidigo
	if err != nil {
		log.Fatalf("logger: %v", err)
	}

	logger.Info("starting client", zap.String("server", cfg.Server))
	defer logger.Info("client stopped")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if cfg.Mode == "proxy" {
		// @sk-task local-proxy-mode#T1.1: proxy mode entry (AC-003)
		runProxyMode(ctx, cfg, logger)
		return
	}

	tunDev := tun.NewTunDevice()
	if err := tunDev.Open(); err != nil {
		logger.Fatal("open tun", zap.Error(err))
	}
	defer func() { _ = tunDev.Close() }()

	reconnectLoop(ctx, tunDev, cfg, logger)
}

// @sk-task local-proxy-mode#T1.1: proxy mode entry (AC-003)
// @sk-task local-proxy-mode#T2.1: SOCKS5 listener in proxy mode (AC-001)
// @sk-task local-proxy-mode#T2.2: stream manager initialization (AC-001)
// @sk-task production-gap#T1.1: proxy mode uses explicit client TLS trust settings (AC-001)
func runProxyMode(ctx context.Context, cfg *config.ClientConfig, logger *zap.Logger) {
	tlsCfg, err := tls.NewClientTLSConfigFromSettings(tls.ClientTLSSettings{
		CAFile:     cfg.TLS.CAFile,
		ServerName: cfg.TLS.ServerName,
		VerifyMode: cfg.TLS.VerifyMode,
	})
	if err != nil {
		logger.Fatal("proxy tls config", zap.Error(err))
	}

	wsConn, err := websocket.Dial(cfg.Server, tlsCfg, logger, websocket.WSConfig{
		Compression: cfg.Compression,
		Multiplex:   cfg.Multiplex,
		MTU:         cfg.MTU,
	})
	if err != nil {
		logger.Fatal("proxy dial", zap.Error(err))
	}
	defer func() { _ = wsConn.Close() }()

	streamMgr := proxy.NewManager(wsConn)

	// @sk-task local-proxy-mode#T3.3: CIDR/domain exclusion in proxy mode (AC-007)
	var routeSet *routing.RuleSet
	if cfg.Routing != nil {
		rs, err := routing.NewRuleSet(cfg.Routing, logger)
		if err != nil {
			logger.Warn("routing init, using default", zap.Error(err))
		} else {
			routeSet = rs
		}
	}

	pl := proxy.NewListener(*cfg, func(client net.Conn, dst string) {
		// Check exclusion rules
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
				logger.Debug("proxy direct", zap.String("dst", dst))
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
		logger.Fatal("proxy start", zap.Error(err))
	}
	defer func() { _ = pl.Close() }()

	logger.Info("proxy mode", zap.String("listen", pl.Addr().String()))

	go func() {
		if err := pl.AcceptLoop(); err != nil {
			logger.Warn("proxy accept loop ended", zap.Error(err))
		}
	}()

	// Read incoming Proxy frames from WS and route to local streams
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			data, err := wsConn.ReadMessage()
			if err != nil {
				return
			}
			var f framing.Frame
			if err := f.Decode(data); err != nil {
				continue
			}
			if f.Type == framing.FrameTypeProxy {
				streamMgr.HandleIncomingFrame(&f)
			}
			f.Release()
		}
	}()

	<-ctx.Done()
	logger.Info("proxy mode stopped")
}

// @sk-task production-hardening#T4.2: reconnect loop with backoff (AC-001)
// @sk-task production-gap#T1.1: reconnect loop enforces explicit client TLS trust settings (AC-001)
func reconnectLoop(ctx context.Context, tunDev tun.TunDevice, cfg *config.ClientConfig, logger *zap.Logger) {
	minBackoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	if cfg.Reconnect != nil {
		if cfg.Reconnect.MinBackoffSec > 0 {
			minBackoff = time.Duration(cfg.Reconnect.MinBackoffSec) * time.Second
		}
		if cfg.Reconnect.MaxBackoffSec > 0 {
			maxBackoff = time.Duration(cfg.Reconnect.MaxBackoffSec) * time.Second
		}
	}

	tlsCfg, err := tls.NewClientTLSConfigFromSettings(tls.ClientTLSSettings{
		CAFile:     cfg.TLS.CAFile,
		ServerName: cfg.TLS.ServerName,
		VerifyMode: cfg.TLS.VerifyMode,
	})
	if err != nil {
		logger.Fatal("client tls config", zap.Error(err))
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
		logger.Info("connecting", zap.Int("attempt", attempt), zap.Duration("backoff", backoff))

		// @sk-task performance-and-polish#T1.1: pass Compression, Multiplex, MTU to Dial (AC-004, AC-006, AC-007)
		wsCfg := websocket.WSConfig{
			Compression: cfg.Compression,
			Multiplex:   cfg.Multiplex,
			MTU:         cfg.MTU,
		}
		wsConn, err := websocket.Dial(cfg.Server, tlsCfg, logger, wsCfg)
		if err != nil {
			logger.Warn("dial failed", zap.Error(err), zap.Duration("retry_in", backoff))
			applyKillSwitch(cfg, logger)
			sleepWithContext(ctx, backoff)
			backoff = nextBackoff(backoff, minBackoff, maxBackoff)
			continue
		}

		wsConn.SetKeepalive(control.DefaultPingInterval, control.DefaultPongTimeout)

		removeKillSwitch(cfg, logger)
		backoff = minBackoff

		runSession(ctx, tunDev, wsConn, cfg, logger)
	}
}

func runSession(ctx context.Context, tunDev tun.TunDevice, wsConn *websocket.WSConn, cfg *config.ClientConfig, logger *zap.Logger) {
	defer func() { _ = wsConn.Close() }()

	helloFrame, err := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		IPv6:         cfg.IPv6,
		Token:        cfg.Auth.Token,
		MTU:          cfg.MTU,
	})
	if err != nil {
		logger.Error("encode client hello", zap.Error(err))
		return
	}
	helloData, err := helloFrame.Encode()
	if err != nil {
		logger.Error("encode hello frame", zap.Error(err))
		return
	}
	if err := wsConn.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		logger.Error("send hello", zap.Error(err))
		return
	}
	framing.ReturnBuffer(helloData)

	respData, err := wsConn.ReadMessage()
	if err != nil {
		logger.Error("read server hello", zap.Error(err))
		return
	}
	var respFrame framing.Frame
	if err := respFrame.Decode(respData); err != nil {
		logger.Error("decode response frame", zap.Error(err))
		return
	}

	switch respFrame.Type {
	case framing.FrameTypeAuth:
		authErr, _ := handshake.DecodeAuthError(&respFrame)
		logger.Fatal("auth rejected", zap.String("reason", authErr.Reason))
	case framing.FrameTypeHello:
		serverHello, err := handshake.DecodeServerHello(&respFrame)
		if err != nil {
			logger.Error("decode server hello", zap.Error(err))
			return
		}
		logger.Info("handshake complete",
			zap.String("session", serverHello.SessionID),
			zap.String("ip", serverHello.AssignedIP.String()),
		)
		mask := &net.IPNet{
			IP:   serverHello.AssignedIP,
			Mask: net.CIDRMask(24, 32),
		}
		if err := tunDev.SetIP(serverHello.AssignedIP, mask); err != nil {
			logger.Error("set tun ip", zap.Error(err))
			return
		}
		// @sk-task ipv6-dual-stack#T2.2: set assigned IPv6 on TUN (AC-001)
		if serverHello.AssignedIPv6 != nil {
			logger.Info("assigned IPv6", zap.String("ip6", serverHello.AssignedIPv6.String()))
			v6Mask := &net.IPNet{
				IP:   serverHello.AssignedIPv6,
				Mask: net.CIDRMask(112, 128),
			}
			if err := tunDev.SetIP(serverHello.AssignedIPv6, v6Mask); err != nil {
				logger.Error("set tun ipv6", zap.Error(err))
				return
			}
		}
		if cfg.MTU > 0 {
			if err := tunDev.SetMTU(cfg.MTU); err != nil {
				logger.Warn("set tun mtu", zap.Int("mtu", cfg.MTU), zap.Error(err))
			}
		}
	default:
		logger.Error("unexpected response type", zap.Int("type", int(respFrame.Type)))
		return
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return tunToWS(ctx, tunDev, wsConn, logger)
	})
	eg.Go(func() error {
		return wsToTun(ctx, tunDev, wsConn, logger)
	})

	if err := eg.Wait(); err != nil {
		logger.Info("forwarding stopped", zap.Error(err))
	}
}

// @sk-task production-hardening#T4.2: kill-switch enable (AC-003)
// @sk-task ipv6-dual-stack#T3.3: add IPv6 kill-switch table (AC-005)
func applyKillSwitch(cfg *config.ClientConfig, logger *zap.Logger) {
	if cfg.KillSwitch == nil || !cfg.KillSwitch.Enabled {
		return
	}
	cmds := [][]string{
		{"add", "table", "ip", "kvn-kill"},
		{"add", "chain", "ip", "kvn-kill", "prerouting", "{ type filter hook prerouting priority 0; }"},
		{"add", "rule", "ip", "kvn-kill", "prerouting", "reject"},
	}
	for _, args := range cmds {
		if err := exec.Command("nft", args...).Run(); err != nil {
			logger.Warn("kill-switch: nft command failed", zap.Strings("args", args), zap.Error(err))
			return
		}
	}
	if cfg.IPv6 {
		cmds6 := [][]string{
			{"add", "table", "ip6", "kvn-kill"},
			{"add", "chain", "ip6", "kvn-kill", "prerouting", "{ type filter hook prerouting priority 0; }"},
			{"add", "rule", "ip6", "kvn-kill", "prerouting", "reject"},
		}
		for _, args := range cmds6 {
			if err := exec.Command("nft", args...).Run(); err != nil {
				logger.Warn("kill-switch: ipv6 nft command failed", zap.Strings("args", args), zap.Error(err))
				return
			}
		}
	}
	logger.Info("kill-switch enabled: all traffic blocked")
}

// @sk-task production-hardening#T4.2: kill-switch disable (AC-003)
// @sk-task ipv6-dual-stack#T3.3: remove IPv6 kill-switch table (AC-005)
func removeKillSwitch(cfg *config.ClientConfig, logger *zap.Logger) {
	if cfg.KillSwitch == nil || !cfg.KillSwitch.Enabled {
		return
	}
	if err := exec.Command("nft", "delete", "table", "ip", "kvn-kill").Run(); err != nil {
		logger.Warn("kill-switch: nftables delete failed", zap.Error(err))
	}
	if cfg.IPv6 {
		if err := exec.Command("nft", "delete", "table", "ip6", "kvn-kill").Run(); err != nil {
			logger.Warn("kill-switch: ipv6 nftables delete failed", zap.Error(err))
		}
	}
}

func nextBackoff(current, min, max time.Duration) time.Duration {
	next := current * 2
	jitter := time.Duration(rand.Int63n(int64(time.Second))) - time.Second/2
	next += jitter
	if next < min {
		return min
	}
	if next > max {
		return max
	}
	return next
}

func sleepWithContext(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func tunToWS(ctx context.Context, dev tun.TunDevice, conn *websocket.WSConn, logger *zap.Logger) error {
	buf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := dev.Read(buf)
		if err != nil {
			return fmt.Errorf("tun read: %w", err)
		}
		f := framing.Frame{
			Type:    framing.FrameTypeData,
			Flags:   framing.FrameFlagNone,
			Payload: buf[:n],
		}
		data, err := f.Encode()
		if err != nil {
			return err
		}
		if err := conn.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
			framing.ReturnBuffer(data)
			return fmt.Errorf("ws write deadline: %w", err)
		}
		if err := conn.WriteMessage(data); err != nil {
			framing.ReturnBuffer(data)
			return fmt.Errorf("ws write: %w", err)
		}
		framing.ReturnBuffer(data)
	}
}

func wsToTun(ctx context.Context, dev tun.TunDevice, conn *websocket.WSConn, logger *zap.Logger) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
			return fmt.Errorf("ws read deadline: %w", err)
		}
		data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("ws read: %w", err)
		}
		var f framing.Frame
		if err := f.Decode(data); err != nil {
			return err
		}
		if f.Type == framing.FrameTypeData {
			if _, err := dev.Write(f.Payload); err != nil {
				f.Release()
				return fmt.Errorf("tun write: %w", err)
			}
			f.Release()
		}
	}
}
