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
	pkglog "github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/metrics"
	"github.com/bzdvdn/kvn-ws/src/internal/nat"
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

	// @sk-task ipv6-dual-stack#T2.3: init IPv6 pool and bolt store (AC-004)
	if cfg.Network.PoolIPv6.Subnet != "" {
		pool6, err := session.NewIPPool6(session.PoolCfg{
			Subnet:  cfg.Network.PoolIPv6.Subnet,
			Gateway: cfg.Network.PoolIPv6.Gateway,
		})
		if err != nil {
			logger.Warn("create ipv6 pool, running ipv4-only", zap.Error(err))
		} else {
			if cfg.BoltDBPath != "" {
				boltStore6, err := session.NewBoltStore6(cfg.BoltDBPath)
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

	sm.Start(300*time.Second, 24*time.Hour, 10*time.Second)

	collectors := metrics.NewCollectors()
	ready := false

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

	mux := http.NewServeMux()

	rl := newRateLimiter(cfg.RateLimiting.AuthBurst, cfg.RateLimiting.AuthPerMinute)
	prl := newSessionPacketLimiter(cfg.RateLimiting.PacketsPerSec)

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
		handleTunnel(w, r, tunDev, sm, cfg.Auth.Tokens, prl, bwMgr, originChecker, wsCfg, collectors, logger)
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

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// @sk-task production-gap#T3.1: protect metrics with the shared operational token gate (AC-005)
	mux.Handle("/metrics", admin.TokenMiddleware(cfg.Admin.Token)(promhttp.Handler()))

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
				if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

func handleTunnel(w http.ResponseWriter, r *http.Request, tunDev tun.TunDevice, sm *session.SessionManager, validTokens []config.TokenCfg, prl *sessionPacketLimiter, bwMgr *session.TokenBandwidthManager, originChecker func(r *http.Request) bool, wsCfg websocket.WSConfig, collectors *metrics.Collectors, logger *zap.Logger) {
	wsConn, err := websocket.Accept(w, r, originChecker, wsCfg)
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
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: "invalid token"})
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
		authFrame, _ := handshake.EncodeAuthError(&handshake.AuthError{Reason: errMsg})
		authData, _ := authFrame.Encode()
		_ = wsConn.WriteMessage(authData)
		framing.ReturnBuffer(authData)
		_ = wsConn.Close()
		return
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

	eg, ctx := errgroup.WithContext(context.Background())
	eg.Go(func() error {
		return serverWSToTun(ctx, tunDev, wsConn, sess.ID, prl, collectors, logger)
	})
	eg.Go(func() error {
		return serverTunToWS(ctx, tunDev, wsConn, sess.ID, tokenName, bwMgr, collectors, logger)
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

	sm.Remove(sess.ID)
	_ = wsConn.Close()
}

// @sk-task local-proxy-mode#T1.2: server-side proxy stream handler (AC-001)
var proxyStreams sync.Map

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
			f.Release()
			if err != nil {
				return err
			}
			collectors.AddThroughput("rx", float64(n))
		} else if f.Type == framing.FrameTypeProxy {
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
				tcpConn := v.(net.Conn)
				_, _ = tcpConn.Write(data)
				f.Release()
			} else {
				tcpConn, err := net.Dial("tcp", dst)
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

				go func(sid uint32, tcp net.Conn, ws *websocket.WSConn) {
					defer func() {
						_ = tcp.Close()
						proxyStreams.Delete(sid)
					}()
					buf := make([]byte, 4096)
					for {
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
						binary.BigEndian.PutUint16(frame.Payload[4:6], uint16(len(dst)))
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
				}(streamID, tcpConn, conn)
			}
		} else {
			f.Release()
		}
	}
}

func serverTunToWS(ctx context.Context, dev tun.TunDevice, conn *websocket.WSConn, sessionID, tokenName string, bwMgr *session.TokenBandwidthManager, collectors *metrics.Collectors, logger *zap.Logger) error {
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
		if bwMgr != nil && !bwMgr.Allow(tokenName, n) {
			time.Sleep(10 * time.Millisecond)
			continue
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
