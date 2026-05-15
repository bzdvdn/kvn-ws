// @sk-task security-acl#T3: CIDR ACL middleware integration
// @sk-task security-acl#T7: Bandwidth limiter integration
// @sk-task security-acl#T10: Admin API integration
// @sk-task docs-and-release#T5.1: fix session ID hex encoding (AC-008)
// @sk-task tun-data-path#T6.1: SetIP call fixed (AC-003)
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bzdvdn/kvn-ws/src/internal/acl"
	"github.com/bzdvdn/kvn-ws/src/internal/admin"
	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	pkglog "github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/metrics"
	"github.com/bzdvdn/kvn-ws/src/internal/nat"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/auth"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/handshake"
	"github.com/bzdvdn/kvn-ws/src/internal/proxy"
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

	logger, logLevel, err := pkglog.New(cfg.Logging.Level)
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
	var reloadMu sync.Mutex
	go func() {
		for {
			select {
			case <-ctx.Done():
				signal.Stop(sighupCh)
				return
			case <-sighupCh:
			}
			reloadMu.Lock()
			logger.Info("sighup received, reloading config")
			newCfg, err := config.LoadServerConfig(cfgPathVal)
			if err != nil {
				logger.Warn("config reload failed, keeping old config", zap.Error(err))
				reloadMu.Unlock()
				continue
			}
			cfgPtr.Store(newCfg)
			// @sk-task post-hardening#T3.5: update log level on SIGHUP (AC-011)
			if newCfg.Logging.Level != "" {
				var lvl zapcore.Level
				if err := lvl.UnmarshalText([]byte(newCfg.Logging.Level)); err == nil {
					logLevel.SetLevel(lvl)
				}
			}
			logger.Info("config reloaded",
				zap.Int("tokens", len(newCfg.Auth.Tokens)),
				zap.String("log_level", logLevel.Level().String()),
			)
			reloadMu.Unlock()
		}
	}()

	// @sk-task security-acl#T3: CIDR matcher setup
	cidrMatcher, err := acl.NewCIDRMatcher(cfg.ACL.AllowCIDRs, cfg.ACL.DenyCIDRs)
	if err != nil {
		logger.Fatal("cidr matcher", zap.Error(err))
	}
	if len(cfg.ACL.DenyCIDRs) > 0 {
		logger.Info("cidr deny list", zap.Strings("cidrs", cfg.ACL.DenyCIDRs))
	}
	if len(cfg.ACL.AllowCIDRs) > 0 {
		logger.Info("cidr allow list", zap.Strings("cidrs", cfg.ACL.AllowCIDRs))
	}

	pool, err := session.NewIPPool(session.PoolCfg{
		Subnet:     cfg.Network.PoolIPv4.Subnet,
		Gateway:    cfg.Network.PoolIPv4.Gateway,
		RangeStart: cfg.Network.PoolIPv4.RangeStart,
		RangeEnd:   cfg.Network.PoolIPv4.RangeEnd,
	}, logger)
	if err != nil {
		logger.Fatal("create ip pool", zap.Error(err))
	}

	var boltStore, boltStore6 *session.BoltStore
	defer func() {
		if boltStore != nil {
			_ = boltStore.Close()
		}
		if boltStore6 != nil {
			_ = boltStore6.Close()
		}
	}()

	if cfg.BoltDBPath != "" {
		boltStore, err = session.NewBoltStore(cfg.BoltDBPath, logger)
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

	sm := session.NewSessionManager(pool, logger)

	// @sk-task ipv6-dual-stack#T2.3: init IPv6 pool and bolt store (AC-004)
	if cfg.Network.PoolIPv6.Subnet != "" {
		pool6, err := session.NewIPPool6(session.PoolCfg{
			Subnet:  cfg.Network.PoolIPv6.Subnet,
			Gateway: cfg.Network.PoolIPv6.Gateway,
		}, logger)
		if err != nil {
			logger.Warn("create ipv6 pool, running ipv4-only", zap.Error(err))
		} else {
			if cfg.BoltDBPath != "" {
				boltStore6, err = session.NewBoltStore6(cfg.BoltDBPath, logger)
				if err != nil {
					logger.Warn("bolt db6 init", zap.Error(err))
				} else {
					pool6.SetBoltStore(boltStore6)
					if err := pool6.LoadFromBolt(); err != nil {
						logger.Warn("bolt db6 load", zap.Error(err))
					}
				}
			}
			sm.SetPool6(pool6)
			logger.Info("ipv6 pool initialized", zap.String("subnet", cfg.Network.PoolIPv6.Subnet))
		}
	} else {
		logger.Info("no ipv6 pool configured, running ipv4-only")
	}

	// @sk-task production-readiness-hardening#T2.4: defer sm.Stop for reclaimLoop cleanup (AC-004)
	idleTimeout := time.Duration(cfg.Session.IdleTimeoutSec) * time.Second
	if idleTimeout <= 0 {
		idleTimeout = 300 * time.Second
	}
	sessionTTL := 24 * time.Hour
	reclaimInterval := 10 * time.Second
	if cfg.Session.Expiry != nil {
		if cfg.Session.Expiry.SessionTTLSec > 0 {
			sessionTTL = time.Duration(cfg.Session.Expiry.SessionTTLSec) * time.Second
		}
		if cfg.Session.Expiry.ReclaimInterval > 0 {
			reclaimInterval = time.Duration(cfg.Session.Expiry.ReclaimInterval) * time.Second
		}
	}
	sm.Start(idleTimeout, sessionTTL, reclaimInterval)
	defer sm.Stop()

	collectors := metrics.NewCollectors()
	ready := false
	startTime := time.Now()

	tunDev := tun.NewTunDevice()
	if err := tunDev.Open(); err != nil {
		logger.Fatal("open tun", zap.Error(err))
	}
	defer func() { _ = tunDev.Close() }()

	gatewayIP := netParseIP(cfg.Network.PoolIPv4.Gateway)
	_, subnet, _ := netParseCIDR(cfg.Network.PoolIPv4.Subnet)
	if err := tunDev.SetIP(gatewayIP, subnet); err != nil {
		logger.Fatal("set tun ip", zap.Error(err))
	}

	// @sk-task ipv6-dual-stack#T2.2: set IPv6 gateway on TUN (AC-001)
	if cfg.Network.PoolIPv6.Subnet != "" {
		gatewayIPv6 := netParseIP(cfg.Network.PoolIPv6.Gateway)
		_, v6Subnet, _ := netParseCIDR(cfg.Network.PoolIPv6.Subnet)
		if err := tunDev.SetIP(gatewayIPv6, v6Subnet); err != nil {
			logger.Warn("set tun ipv6", zap.Error(err))
		}
	}

	// @sk-task ipv6-dual-stack#T2.3: setup IPv6 NAT masquerade (AC-003)
	natMgr := nat.NewNFTManager()
	if err := natMgr.Setup(); err != nil {
		logger.Warn("nat setup", zap.Error(err))
	}
	if cfg.Network.PoolIPv6.Subnet != "" {
		if err := natMgr.Setup6(); err != nil {
			logger.Warn("ipv6 nat setup", zap.Error(err))
		}
	}
	defer func() {
		_ = natMgr.Teardown()
		if cfg.Network.PoolIPv6.Subnet != "" {
			_ = natMgr.Teardown6()
		}
	}()

	tlsCfg, err := tlspkg.NewServerTLSConfig(cfg.TLS.Cert, cfg.TLS.Key, cfg.TLS.ClientCAFile, cfg.TLS.ClientAuth)
	if err != nil {
		logger.Fatal("tls config", zap.Error(err))
	}
	if cfg.TLS.ClientCAFile != "" {
		logger.Info("mtls enabled", zap.String("client_ca", cfg.TLS.ClientCAFile), zap.String("client_auth", cfg.TLS.ClientAuth))
	}

	tlsListener, err := tls.Listen("tcp", cfg.Listen, tlsCfg)
	if err != nil {
		logger.Fatal("tls listen", zap.Error(err))
	}
	defer func() { _ = tlsListener.Close() }()

	// @sk-task security-acl#T4: origin checker
	originChecker := websocket.NewOriginChecker(cfg.Origin.Whitelist, cfg.Origin.AllowEmpty)

	// @sk-task security-acl#T6: bandwidth manager
	bwCfg := make(map[string]int)
	for _, tc := range cfg.Auth.Tokens {
		bwCfg[tc.Name] = tc.BandwidthBPS
	}
	bwMgr := session.NewTokenBandwidthManager(bwCfg)

	// @sk-task app-crypto#T4: parse master key (AC-006)
	var masterKey []byte
	if cfg.Crypto.Enabled {
		masterKey, err = crypto.ParseMasterKey(cfg.Crypto.Key)
		if err != nil {
			logger.Fatal("crypto key", zap.Error(err))
		}
		logger.Info("app-layer encryption enabled")
	} else {
		logger.Info("app-layer encryption disabled")
	}

	mux := http.NewServeMux()

	rl := newRateLimiter(cfg.RateLimiting.AuthBurst, cfg.RateLimiting.AuthPerMinute)
	prl := newSessionPacketLimiter(cfg.RateLimiting.PacketsPerSec)
	rl.startCleanup(ctx)
	prl.startCleanup(ctx)

	// @sk-task security-acl#T3: CIDR ACL middleware
	tunnelHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(r.RemoteAddr) {
			pkglog.Audit(logger, zapcore.WarnLevel, "auth rate limited",
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("reason", "rate limit exceeded"),
			)
			http.Error(w, "429 too many requests", http.StatusTooManyRequests)
			return
		}
		wsCfg := websocket.WSConfig{
			Compression: cfg.Compression,
			Multiplex:   cfg.Multiplex,
			MTU:         cfg.MTU,
		}
		// @sk-task performance-and-polish#T1.1: pass Compression, Multiplex, MTU to Accept (AC-004, AC-006, AC-007)
		handleTunnel(w, r, tunDev, sm, cfg.Auth.Tokens, prl, bwMgr, originChecker, wsCfg, collectors, logger, masterKey)
	})

	cidrMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			ip := net.ParseIP(host)
			if ip != nil && !cidrMatcher.Allowed(ip) {
				pkglog.Audit(logger, zapcore.WarnLevel, "connection denied by CIDR ACL",
					zap.String("remote_addr", r.RemoteAddr),
					zap.String("ip", host),
				)
				http.Error(w, "403 forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	mux.Handle("/tunnel", cidrMiddleware(tunnelHandler))

	// @sk-task production-readiness-hardening#T3.6: health check with dependency verification (AC-012)
	// @sk-task production-readiness-gap#T2: liveness + readiness probes (AC-001)
	mux.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "alive",
			"uptime_s": time.Since(startTime).Seconds(),
		})
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
			return
		}
		sessions := sm.List()
		health := map[string]interface{}{
			"status":          "ok",
			"uptime_s":        time.Since(startTime).Seconds(),
			"tun":             "ok",
			"nat":             "ok",
			"active_sessions": len(sessions),
		}
		if boltStore != nil {
			health["bolt"] = "ok"
		} else {
			health["bolt"] = "disabled"
		}
		_ = json.NewEncoder(w).Encode(health)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
			return
		}
		sessions := sm.List()
		health := map[string]interface{}{
			"status":          "ok",
			"uptime_s":        time.Since(startTime).Seconds(),
			"tun":             "ok",
			"nat":             "ok",
			"active_sessions": len(sessions),
		}
		if boltStore != nil {
			health["bolt"] = "ok"
		} else {
			health["bolt"] = "disabled"
		}
		_ = json.NewEncoder(w).Encode(health)
	})

	// @sk-task production-gap#T3.1: protect metrics with the shared operational token gate (AC-005)
	// @sk-task post-hardening#T2.3: rate limit /metrics (AC-007)
	mrl := newRateLimiter(100, 100)
	mrl.startCleanup(ctx)
	metricsHandler := admin.TokenMiddleware(cfg.Admin.Token)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !mrl.Allow(r.RemoteAddr) {
			http.Error(w, "429 too many requests", http.StatusTooManyRequests)
			return
		}
		promhttp.Handler().ServeHTTP(w, r)
	}))
	mux.Handle("/metrics", metricsHandler)

	// @sk-task production-readiness-hardening#T2.2: ReadHeaderTimeout prevents slow-loris (AC-002)
	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 20 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
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

	// @sk-task security-acl#T10: Admin API server
	if cfg.Admin.Enabled {
		adminCfg := admin.AdminCfg{
			Enabled: cfg.Admin.Enabled,
			Listen:  cfg.Admin.Listen,
			Token:   cfg.Admin.Token,
		}
		adminSrv := admin.NewAdminServer(adminCfg, sm)

		if cfg.Admin.Token == "" {
			logger.Warn("admin api token not set, disabling admin api")
		} else {
			addr := cfg.Admin.Listen
			if !strings.HasPrefix(addr, "localhost:") && !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "unix:") {
				logger.Warn("admin api listening on non-loopback interface", zap.String("addr", addr))
			}
			logger.Info("admin api enabled", zap.String("listen", addr))

			eg.Go(func() error {
				if err := adminSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					return fmt.Errorf("admin api: %w", err)
				}
				return nil
			})
			eg.Go(func() error {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), tunnelShutdownTimeout)
				defer cancel()
				return adminSrv.Shutdown(shutdownCtx)
			})
		}
	}

	if err := eg.Wait(); err != nil {
		logger.Info("server stopped", zap.Error(err))
	}

	logger.Info("shutting down")
}

func handleTunnel(w http.ResponseWriter, r *http.Request, tunDev tun.TunDevice, sm *session.SessionManager, validTokens []config.TokenCfg, prl *sessionPacketLimiter, bwMgr *session.TokenBandwidthManager, originChecker func(r *http.Request) bool, wsCfg websocket.WSConfig, collectors *metrics.Collectors, logger *zap.Logger, masterKey []byte) {
	wsConn, err := websocket.Accept(w, r, logger, originChecker, wsCfg)
	if err != nil {
		logger.Error("ws upgrade", zap.Error(err))
		return
	}

	data, err := wsConn.ReadMessage()
	if err != nil {
		logger.Error("read client hello", zap.Error(err))
		_ = wsConn.Close()
		return
	}
	var frame framing.Frame
	if err := frame.Decode(data); err != nil {
		logger.Error("decode client hello frame", zap.Error(err))
		_ = wsConn.Close()
		return
	}
	clientHello, err := handshake.DecodeClientHello(&frame)
	if err != nil {
		logger.Error("decode client hello", zap.Error(err))
		_ = wsConn.Close()
		return
	}

	tokenCfg := auth.FindToken(clientHello.Token, validTokens)
	if tokenCfg == nil {
		pkglog.Audit(logger, zapcore.WarnLevel, "auth failed",
			zap.String("session_id", ""),
			zap.String("reason", "invalid token"),
			zap.String("remote_addr", r.RemoteAddr),
		)
		// @sk-task post-hardening#T1.4: generic auth error (AC-004)
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: "authentication failed"})
		authData, _ := authFrame.Encode()
		_ = wsConn.WriteMessage(authData)
		framing.ReturnBuffer(authData)
		_ = wsConn.Close()
		return
	}

	tokenName := tokenCfg.Name
	var sidBuf [16]byte
	if _, rerr := rand.Read(sidBuf[:]); rerr != nil {
		copy(sidBuf[:], []byte(clientHello.Token))
	}
	sessionID := hex.EncodeToString(sidBuf[:])
	sess, assignedIP, assignedIPv6, err := sm.Create(sessionID, tokenName, r.RemoteAddr, tokenCfg.MaxSessions, clientHello.IPv6)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "max sessions exceeded") {
			pkglog.Audit(logger, zapcore.WarnLevel, "max sessions exceeded",
				zap.String("token_name", tokenName),
				zap.String("remote_addr", r.RemoteAddr),
			)
		} else {
			logger.Error("session create", zap.Error(err))
		}
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: "authentication failed"})
		authData, _ := authFrame.Encode()
		_ = wsConn.WriteMessage(authData)
		framing.ReturnBuffer(authData)
		_ = wsConn.Close()
		return
	}

	// @sk-task app-crypto#T4: generate salt and derive session key (AC-006)
	var sessionCipher *crypto.SessionCipher
	var cryptoSalt []byte
	if len(masterKey) > 0 {
		cryptoSalt, err = crypto.GenerateSalt()
		if err != nil {
			logger.Error("generate crypto salt", zap.Error(err))
			_ = wsConn.Close()
			return
		}
	}

	mtu := wsCfg.MTU
	if mtu <= 0 {
		mtu = handshake.DefaultMTU
	}
	serverHello, err := handshake.EncodeServerHello(&handshake.ServerHello{
		SessionID:    sess.ID,
		AssignedIP:   assignedIP,
		AssignedIPv6: assignedIPv6,
		MTU:          mtu,
		CryptoSalt:   cryptoSalt,
	})
	if err != nil {
		logger.Error("encode server hello", zap.Error(err))
		_ = wsConn.Close()
		return
	}
	helloData, err := serverHello.Encode()
	if err != nil {
		logger.Error("encode hello frame", zap.Error(err))
		_ = wsConn.Close()
		return
	}
	if err := wsConn.WriteMessage(helloData); err != nil {
		framing.ReturnBuffer(helloData)
		logger.Error("send server hello", zap.Error(err))
		_ = wsConn.Close()
		return
	}
	framing.ReturnBuffer(helloData)

	logger.Info("session created",
		zap.String("session", sess.ID),
		zap.String("token", tokenName),
		zap.String("ip", assignedIP.String()),
	)
	collectors.ActiveSessions.Inc()

	// @sk-task app-crypto#T4: derive session key (AC-006)
	if len(cryptoSalt) > 0 {
		sessionCipher, err = crypto.NewSessionCipher(masterKey, cryptoSalt, sess.ID)
		if err != nil {
			logger.Error("session cipher init", zap.Error(err))
			_ = wsConn.Close()
			return
		}
	}

	// @sk-task production-readiness-hardening#T2.5: per-session proxy streams (AC-005)
	// @sk-task post-hardening#T1.2: per-session cancel context (AC-002)
	// @sk-task post-hardening#T3.4: use proxy.SessionStreams (AC-012)
	// @sk-task prod-issue#T1.4: inherit parent request context instead of Background() (AC-004)
	sessionCtx, sessionCancel := context.WithCancel(r.Context())
	sm.SetCancel(sess.ID, sessionCancel)
	sessionStreams := &proxy.SessionStreams{M: make(map[uint32]net.Conn)}

	eg, ctx := errgroup.WithContext(sessionCtx)
	eg.Go(func() error {
		return serverWSToTun(ctx, tunDev, wsConn, sm, sess.ID, prl, collectors, logger, sessionStreams, sessionCipher)
	})
	eg.Go(func() error {
		return serverTunToWS(ctx, tunDev, wsConn, sm, sess.ID, tokenName, bwMgr, collectors, logger, sessionCipher)
	})

	if err := eg.Wait(); err != nil {
		logger.Info("session ended",
			zap.String("session", sess.ID),
			zap.String("token", tokenName),
			zap.String("ip", assignedIP.String()),
			zap.Error(err),
		)
	}
	collectors.ActiveSessions.Dec()

	sessionStreams.CloseAll()
	sm.Remove(sess.ID)
	_ = wsConn.Close()
}

const wsTunnelTimeout = 30 * time.Second
const defaultProxyConcurrency = 1000

// @sk-task post-hardening#T2.2: proxy worker pool semaphore (AC-006)
var proxySem chan struct{}

func init() {
	proxySem = make(chan struct{}, defaultProxyConcurrency)
}

func serverWSToTun(ctx context.Context, dev tun.TunDevice, conn *websocket.WSConn, sm *session.SessionManager, sessionID string, prl *sessionPacketLimiter, collectors *metrics.Collectors, logger *zap.Logger, proxyStreams *proxy.SessionStreams, sessionCipher *crypto.SessionCipher) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		sm.UpdateActivity(sessionID)
		if prl != nil && !prl.Allow(sessionID) {
			pkglog.Audit(logger, zapcore.WarnLevel, "packet rate limited",
				zap.String("session_id", sessionID),
				zap.String("reason", "packet rate exceeded"),
			)
			continue
		}
		if err := conn.SetReadDeadline(time.Now().Add(wsTunnelTimeout)); err != nil {
			return err
		}
		data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var f framing.Frame
		if err := f.Decode(data); err != nil {
			return err
		}
		// @sk-task app-crypto#T4: decrypt incoming Data frames (AC-006)
		if f.Type == framing.FrameTypeData && sessionCipher != nil {
			decrypted, err := sessionCipher.Decrypt(f.Payload)
			if err != nil {
				logger.Warn("decrypt failed, dropping packet", zap.Error(err))
				f.Release()
				continue
			}
			f.Release()
			f.Payload = decrypted
		}
		switch f.Type {
		case framing.FrameTypeData:
			n, err := dev.Write(f.Payload)
			f.Release()
			if err != nil {
				return err
			}
			collectors.AddThroughput("rx", float64(n))
		case framing.FrameTypeClose:
			f.Release()
			logger.Debug("session close frame received", zap.String("session_id", sessionID))
			return nil
		case framing.FrameTypeProxy:
			// @sk-task local-proxy-mode#T1.2: proxy frame handler (AC-001)
			payload := f.Payload
			if len(payload) < 6 {
				f.Release()
				continue
			}
			streamID := binary.BigEndian.Uint32(payload[0:4])
			dstLen := binary.BigEndian.Uint16(payload[4:6])
			if int(6+dstLen) > len(payload) {
				f.Release()
				continue
			}
			dst := string(payload[6 : 6+dstLen])
			data := payload[6+dstLen:]

			if v, ok := proxyStreams.Load(streamID); ok {
				_, _ = v.Write(data)
				f.Release()
			} else {
				tcpConn, err := net.DialTimeout("tcp", dst, 10*time.Second)
				if err != nil {
					logger.Warn("proxy dial failed", zap.String("dst", dst), zap.Error(err))
					f.Release()
					continue
				}
				proxyStreams.Store(streamID, tcpConn)
				if len(data) > 0 {
					_, _ = tcpConn.Write(data)
				}
				f.Release()

				// @sk-task post-hardening#T2.2: acquire semaphore (AC-006)
				select {
				case proxySem <- struct{}{}:
				default:
					logger.Warn("proxy concurrency limit reached, dropping stream", zap.Uint32("stream_id", streamID))
					_ = tcpConn.Close()
					proxyStreams.Delete(streamID)
					continue
				}

				// @sk-task post-hardening#T3.2: context-aware proxy goroutine with read deadline (AC-009)
				go func(sid uint32, tcp net.Conn, ws *websocket.WSConn, streams *proxy.SessionStreams, ctx context.Context) {
					defer func() {
						<-proxySem
						_ = tcp.Close()
						streams.Delete(sid)
					}()
					buf := make([]byte, 4096)
					for {
						if err := tcp.SetReadDeadline(time.Now().Add(wsTunnelTimeout)); err != nil {
							return
						}
						select {
						case <-ctx.Done():
							return
						default:
						}
						n, err := tcp.Read(buf)
						if err != nil {
							return
						}
						frame := framing.Frame{
							Type:    framing.FrameTypeProxy,
							Flags:   framing.FrameFlagNone,
							Payload: make([]byte, 4+2+len(dst)+n),
						}
						binary.BigEndian.PutUint32(frame.Payload[0:4], sid)
						binary.BigEndian.PutUint16(frame.Payload[4:6], uint16(len(dst))) // #nosec G115 — bounded by protocol
						copy(frame.Payload[6:], dst)
						copy(frame.Payload[6+len(dst):], buf[:n])
						encoded, err := frame.Encode()
						if err != nil {
							return
						}
						if err := ws.WriteMessage(encoded); err != nil {
							framing.ReturnBuffer(encoded)
							return
						}
						framing.ReturnBuffer(encoded)
					}
				}(streamID, tcpConn, conn, proxyStreams, ctx)
			}
		default:
			f.Release()
		}
	}
}

// @sk-task production-readiness-hardening#T2.1: write deadline before each WriteMessage (AC-001)
// @sk-task app-crypto#T4: encrypt outgoing Data frames (AC-006)
func serverTunToWS(ctx context.Context, dev tun.TunDevice, conn *websocket.WSConn, sm *session.SessionManager, sessionID, tokenName string, bwMgr *session.TokenBandwidthManager, collectors *metrics.Collectors, logger *zap.Logger, sessionCipher *crypto.SessionCipher) error {
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
		sm.UpdateActivity(sessionID)
		if bwMgr != nil {
			delay, ok := bwMgr.Reserve(tokenName, n)
			if !ok {
				continue
			}
			if delay > 0 {
				time.Sleep(delay)
			}
		}
		payload := buf[:n]
		// @sk-task app-crypto#T4: encrypt outgoing Data frames (AC-006)
		if sessionCipher != nil {
			encrypted, err := sessionCipher.Encrypt(payload)
			if err != nil {
				logger.Error("encrypt failed, dropping packet", zap.Error(err))
				continue
			}
			payload = encrypted
		}
		f := framing.Frame{
			Type:    framing.FrameTypeData,
			Flags:   framing.FrameFlagNone,
			Payload: payload,
		}
		data, err := f.Encode()
		if err != nil {
			return err
		}
		if err := conn.SetWriteDeadline(time.Now().Add(wsTunnelTimeout)); err != nil {
			framing.ReturnBuffer(data)
			return err
		}
		if err := conn.WriteMessage(data); err != nil {
			framing.ReturnBuffer(data)
			return err
		}
		framing.ReturnBuffer(data)
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

func (rl *ipRateLimiter) startCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rl.mu.Lock()
				for k, lim := range rl.limiters {
					if lim.Tokens() >= float64(rl.burst) {
						delete(rl.limiters, k)
					}
				}
				rl.mu.Unlock()
			}
		}
	}()
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

func (pl *sessionPacketLimiter) startCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pl.mu.Lock()
				for k, lim := range pl.limiters {
					if lim.Tokens() >= float64(pl.perSec) {
						delete(pl.limiters, k)
					}
				}
				pl.mu.Unlock()
			}
		}
	}()
}

var tunnelShutdownTimeout = 5 * time.Second

func netParseIP(s string) net.IP {
	return net.ParseIP(s)
}

func netParseCIDR(s string) (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(s)
}
