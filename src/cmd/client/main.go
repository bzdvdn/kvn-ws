package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os/signal"
	"syscall"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// @sk-task foundation#T1.1: Go module init (AC-001)
// @sk-task foundation#T3.2: client main with graceful shutdown (AC-010)
// @sk-task core-tunnel-mvp#T4.1: client forwarding loops (AC-007, AC-008)
// @sk-task core-tunnel-mvp#T4.2: graceful shutdown (AC-010)
func main() {
	cfgPath := flag.String("config", "configs/client.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.LoadClientConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	logger, err := logger.New(cfg.Log.Level)
	if err != nil {
		log.Fatalf("logger: %v", err)
	}

	logger.Info("starting client", zap.String("server", cfg.Server))
	defer logger.Info("client stopped")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	tunDev := tun.NewTunDevice()
	if err := tunDev.Open(); err != nil {
		logger.Fatal("open tun", zap.Error(err))
	}
	defer tunDev.Close()

	tlsCfg := tls.NewClientTLSConfig(true)
	wsConn, err := websocket.Dial(cfg.Server, tlsCfg)
	if err != nil {
		logger.Fatal("ws dial", zap.Error(err))
	}
	defer wsConn.Close()

	helloFrame, err := handshake.EncodeClientHello(&handshake.ClientHello{
		ProtoVersion: handshake.ProtoVersion,
		Token:        cfg.Auth.Token,
	})
	if err != nil {
		logger.Fatal("encode client hello", zap.Error(err))
	}
	helloData, err := helloFrame.Encode()
	if err != nil {
		logger.Fatal("encode hello frame", zap.Error(err))
	}
	if err := wsConn.WriteMessage(helloData); err != nil {
		logger.Fatal("send hello", zap.Error(err))
	}

	respData, err := wsConn.ReadMessage()
	if err != nil {
		logger.Fatal("read server hello", zap.Error(err))
	}
	var respFrame framing.Frame
	if err := respFrame.Decode(respData); err != nil {
		logger.Fatal("decode response frame", zap.Error(err))
	}

	switch respFrame.Type {
	case framing.FrameTypeAuth:
		authErr, _ := handshake.DecodeAuthError(&respFrame)
		logger.Fatal("auth rejected", zap.String("reason", authErr.Reason))
	case framing.FrameTypeHello:
		serverHello, err := handshake.DecodeServerHello(&respFrame)
		if err != nil {
			logger.Fatal("decode server hello", zap.Error(err))
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
			logger.Fatal("set tun ip", zap.Error(err))
		}
		if cfg.MTU > 0 {
			if err := tunDev.SetMTU(cfg.MTU); err != nil {
				logger.Warn("set tun mtu", zap.Int("mtu", cfg.MTU), zap.Error(err))
			}
		}
	default:
		logger.Fatal("unexpected response type", zap.Int("type", int(respFrame.Type)))
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

	logger.Info("shutting down")
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
			return err
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
		if err := conn.WriteMessage(data); err != nil {
			return err
		}
	}
}

func wsToTun(ctx context.Context, dev tun.TunDevice, conn *websocket.WSConn, logger *zap.Logger) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var f framing.Frame
		if err := f.Decode(data); err != nil {
			return err
		}
		if f.Type == framing.FrameTypeData {
			if _, err := dev.Write(f.Payload); err != nil {
				return err
			}
		}
	}
}
