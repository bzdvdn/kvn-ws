package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	pkglog "github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/metrics"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/auth"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/session"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/framing"
	tlspkg "github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
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

	logger, err := pkglog.New(cfg.Logging.Level)
	if err != nil {
		log.Fatalf("logger: %v", err)
	}

	logger.Info("starting server", zap.String("listen", cfg.Listen))
	defer logger.Info("server stopped")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// @sk-task production-hardening#T5.1: sighup reload handler (AC-009)
	cfgPtr := config.NewAtomicConfig(cfg)
	sighupCh := make(chan os.Signal, 1)
	signal.Notify(sighupCh, syscall.SIGHUP)
	cfgPathVal := *cfgPath
	go func() {
		for range sighupCh {
			logger.Info("sighup received, reloading config")
			newCfg, err := config.LoadServerConfig(cfgPathVal)
			if err != nil {
				logger.Warn("config reload failed, keeping old config", zap.Error(err))
				continue
			}
			cfgPtr.Store(newCfg)
			logger.Info("config reloaded",
				zap.Int("tokens", len(newCfg.Auth.Tokens)),
			)
		}
	}()

	pool, err := session.NewIPPool(session.PoolCfg{
		Subnet:     cfg.Network.PoolIPv4.Subnet,
		Gateway:    cfg.Network.PoolIPv4.Gateway,
		RangeStart: cfg.Network.PoolIPv4.RangeStart,
		RangeEnd:   cfg.Network.PoolIPv4.RangeEnd,
	})
	if err != nil {
		logger.Fatal("create ip pool", zap.Error(err))
	}

	if cfg.BoltDBPath != "" {
		boltStore, err := session.NewBoltStore(cfg.BoltDBPath)
		if err != nil {
			logger.Warn("bolt db init, using in-memory pool", zap.Error(err))
		} else {
			pool.SetBoltStore(boltStore)
			if err := pool.LoadFromBolt(); err != nil {
				logger.Warn("bolt db load", zap.Error(err))
			}
			logger.Info("ip pool loaded from bolt", zap.String("path", cfg.BoltDBPath))
		}
	}

	sm := session.NewSessionManager(pool)
	sm.Start(300*time.Second, 24*time.Hour, 10*time.Second)

	collectors := metrics.NewCollectors()
	ready := false

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

	rl := newRateLimiter(cfg.RateLimiting.AuthBurst, cfg.RateLimiting.AuthPerMinute)
	prl := newSessionPacketLimiter(cfg.RateLimiting.PacketsPerSec)

	mux.HandleFunc("/tunnel", func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(r.RemoteAddr) {
			pkglog.Audit(logger, zapcore.WarnLevel, "auth rate limited",
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("reason", "rate limit exceeded"),
			)
			http.Error(w, "429 too many requests", http.StatusTooManyRequests)
			return
		}
		handleTunnel(w, r, tunDev, sm, cfg.Auth.Tokens, prl, collectors, logger)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.Handle("/metrics", promhttp.Handler())

	httpServer := &http.Server{
		Handler: mux,
	}

	ready = true

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

func handleTunnel(w http.ResponseWriter, r *http.Request, tunDev tun.TunDevice, sm *session.SessionManager, validTokens []string, prl *sessionPacketLimiter, collectors *metrics.Collectors, logger *zap.Logger) {
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
		pkglog.Audit(logger, zapcore.WarnLevel, "auth failed",
			zap.String("session_id", ""),
			zap.String("reason", "invalid token"),
			zap.String("remote_addr", r.RemoteAddr),
		)
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
	collectors.ActiveSessions.Inc()

	eg, ctx := errgroup.WithContext(context.Background())
	eg.Go(func() error {
		return serverWSToTun(ctx, tunDev, wsConn, sess.ID, prl, collectors, logger)
	})
	eg.Go(func() error {
		return serverTunToWS(ctx, tunDev, wsConn, sess.ID, collectors, logger)
	})

	if err := eg.Wait(); err != nil {
		logger.Info("session ended",
			zap.String("session", sess.ID),
			zap.String("ip", assignedIP.String()),
			zap.Error(err),
		)
	}
	collectors.ActiveSessions.Dec()

	sm.Remove(sess.ID)
	wsConn.Close()
}

func serverWSToTun(ctx context.Context, dev tun.TunDevice, conn *websocket.WSConn, sessionID string, prl *sessionPacketLimiter, collectors *metrics.Collectors, logger *zap.Logger) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if prl != nil && !prl.Allow(sessionID) {
			pkglog.Audit(logger, zapcore.WarnLevel, "packet rate limited",
				zap.String("session_id", sessionID),
				zap.String("reason", "packet rate exceeded"),
			)
			continue
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
			n, err := dev.Write(f.Payload)
			if err != nil {
				return err
			}
			collectors.AddThroughput("rx", float64(n))
		}
	}
}

func serverTunToWS(ctx context.Context, dev tun.TunDevice, conn *websocket.WSConn, sessionID string, collectors *metrics.Collectors, logger *zap.Logger) error {
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

// @sk-task production-hardening#T2.2: auth rate limiter per IP (AC-004)
type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	burst    int
	perMin   int
}

func newRateLimiter(burst, perMin int) *ipRateLimiter {
	return &ipRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		burst:    burst,
		perMin:   perMin,
	}
}

func (rl *ipRateLimiter) Allow(addr string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	lim, ok := rl.limiters[addr]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(rl.perMin)/60, rl.burst)
		rl.limiters[addr] = lim
	}
	return lim.Allow()
}

// @sk-task production-hardening#T2.2: per-session packet rate limiter (AC-004)
type sessionPacketLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	perSec   int
}

func newSessionPacketLimiter(perSec int) *sessionPacketLimiter {
	return &sessionPacketLimiter{
		limiters: make(map[string]*rate.Limiter),
		perSec:   perSec,
	}
}

func (pl *sessionPacketLimiter) Allow(sessionID string) bool {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	lim, ok := pl.limiters[sessionID]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(pl.perSec), pl.perSec)
		pl.limiters[sessionID] = lim
	}
	return lim.Allow()
}

var tunnelShutdownTimeout = 5 * time.Second

func netParseIP(s string) net.IP {
	return net.ParseIP(s)
}

func netParseCIDR(s string) (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(s)
}
