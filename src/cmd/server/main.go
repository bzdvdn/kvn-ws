package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/auth"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/session"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	tlspkg "github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// @sk-task foundation#T1.1: Go module init (AC-001)
// @sk-task foundation#T3.2: server main with graceful shutdown (AC-010)
// @sk-task core-tunnel-mvp#T4.1: server forwarding loops (AC-007, AC-008)
// @sk-task core-tunnel-mvp#T4.2: graceful shutdown (AC-010)
func main() {
	cfgPath := flag.String("config", "configs/server.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.LoadServerConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	logger, err := logger.New(cfg.Logging.Level)
	if err != nil {
		log.Fatalf("logger: %v", err)
	}

	logger.Info("starting server", zap.String("listen", cfg.Listen))
	defer logger.Info("server stopped")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	pool, err := session.NewIPPool(session.PoolCfg{
		Subnet:     cfg.Network.PoolIPv4.Subnet,
		Gateway:    cfg.Network.PoolIPv4.Gateway,
		RangeStart: cfg.Network.PoolIPv4.RangeStart,
		RangeEnd:   cfg.Network.PoolIPv4.RangeEnd,
	})
	if err != nil {
		logger.Fatal("create ip pool", zap.Error(err))
	}
	sm := session.NewSessionManager(pool)
	_ = sm

	tunDev := tun.NewTunDevice()
	if err := tunDev.Open(); err != nil {
		logger.Fatal("open tun", zap.Error(err))
	}
	defer tunDev.Close()

	gatewayIP := netParseIP(cfg.Network.PoolIPv4.Gateway)
	_, subnet, _ := netParseCIDR(cfg.Network.PoolIPv4.Subnet)
	if err := tunDev.SetIP(gatewayIP, subnet); err != nil {
		logger.Fatal("set tun ip", zap.Error(err))
	}

	tlsCfg, err := tlspkg.NewServerTLSConfig(cfg.TLS.Cert, cfg.TLS.Key)
	if err != nil {
		logger.Fatal("tls config", zap.Error(err))
	}

	tlsListener, err := tls.Listen("tcp", cfg.Listen, tlsCfg)
	if err != nil {
		logger.Fatal("tls listen", zap.Error(err))
	}
	defer tlsListener.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/tunnel", func(w http.ResponseWriter, r *http.Request) {
		handleTunnel(w, r, tunDev, sm, cfg.Auth.Tokens, logger)
	})

	httpServer := &http.Server{
		Handler: mux,
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		logger.Info("listening", zap.String("addr", cfg.Listen))
		if err := httpServer.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	eg.Go(func() error {
		<-ctx.Done()
		logger.Info("shutting down http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), tunnelShutdownTimeout)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	})

	if err := eg.Wait(); err != nil {
		logger.Info("server stopped", zap.Error(err))
	}

	logger.Info("shutting down")
}

func handleTunnel(w http.ResponseWriter, r *http.Request, tunDev tun.TunDevice, sm *session.SessionManager, validTokens []string, logger *zap.Logger) {
	wsConn, err := websocket.Accept(w, r)
	if err != nil {
		logger.Error("ws upgrade", zap.Error(err))
		return
	}

	data, err := wsConn.ReadMessage()
	if err != nil {
		logger.Error("read client hello", zap.Error(err))
		wsConn.Close()
		return
	}
	var frame framing.Frame
	if err := frame.Decode(data); err != nil {
		logger.Error("decode client hello frame", zap.Error(err))
		wsConn.Close()
		return
	}
	clientHello, err := handshake.DecodeClientHello(&frame)
	if err != nil {
		logger.Error("decode client hello", zap.Error(err))
		wsConn.Close()
		return
	}

	if !auth.ValidateToken(clientHello.Token, validTokens) {
		logger.Warn("auth failed")
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: "invalid token"})
		authData, _ := authFrame.Encode()
		wsConn.WriteMessage(authData)
		wsConn.Close()
		return
	}

	sess, assignedIP, err := sm.Create(clientHello.Token)
	if err != nil {
		logger.Error("session create", zap.Error(err))
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: "pool exhausted"})
		authData, _ := authFrame.Encode()
		wsConn.WriteMessage(authData)
		wsConn.Close()
		return
	}

	serverHello, err := handshake.EncodeServerHello(&handshake.ServerHello{
		SessionID:  sess.ID,
		AssignedIP: assignedIP,
	})
	if err != nil {
		logger.Error("encode server hello", zap.Error(err))
		wsConn.Close()
		return
	}
	helloData, err := serverHello.Encode()
	if err != nil {
		logger.Error("encode hello frame", zap.Error(err))
		wsConn.Close()
		return
	}
	if err := wsConn.WriteMessage(helloData); err != nil {
		logger.Error("send server hello", zap.Error(err))
		wsConn.Close()
		return
	}

	logger.Info("session created",
		zap.String("session", sess.ID),
		zap.String("ip", assignedIP.String()),
	)

	eg, ctx := errgroup.WithContext(context.Background())
	eg.Go(func() error {
		return serverWSToTun(ctx, tunDev, wsConn, logger)
	})
	eg.Go(func() error {
		return serverTunToWS(ctx, tunDev, wsConn, sess.ID, logger)
	})

	if err := eg.Wait(); err != nil {
		logger.Info("session ended",
			zap.String("session", sess.ID),
			zap.String("ip", assignedIP.String()),
			zap.Error(err),
		)
	}

	sm.Remove(sess.ID)
	wsConn.Close()
}

func serverWSToTun(ctx context.Context, dev tun.TunDevice, conn *websocket.WSConn, logger *zap.Logger) error {
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

func serverTunToWS(ctx context.Context, dev tun.TunDevice, conn *websocket.WSConn, sessionID string, logger *zap.Logger) error {
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

var tunnelShutdownTimeout = 5 * time.Second

func netParseIP(s string) net.IP {
	return net.ParseIP(s)
}

func netParseCIDR(s string) (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(s)
}
