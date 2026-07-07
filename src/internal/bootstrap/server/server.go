package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"

	"github.com/quic-go/quic-go"

	"github.com/bzdvdn/kvn-ws/src/internal/acl"
	"github.com/bzdvdn/kvn-ws/src/internal/admin"
	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/crypto"
	pkglog "github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/metrics"
	"github.com/bzdvdn/kvn-ws/src/internal/nat"
	"github.com/bzdvdn/kvn-ws/src/internal/ratelimit"
	"github.com/bzdvdn/kvn-ws/src/internal/session"
	quictp "github.com/bzdvdn/kvn-ws/src/internal/transport/quic"
	tlspkg "github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
	"github.com/bzdvdn/kvn-ws/src/internal/tunnel"
)

const shutdownTimeout = 5 * time.Second

type Server struct {
	cfg     *config.ServerConfig
	cfgPath string
	cfgPtr  *config.AtomicConfig[config.ServerConfig]
	logger  *zap.Logger
	lvl     zap.AtomicLevel

	cidrMatcher   *acl.CIDRMatcher
	gatewayIP     net.IP
	pool          *session.IPPool
	pool6         *session.IPPool
	bolt          *session.BoltStore
	bolt6         *session.BoltStore
	sm            *session.SessionManager
	collectors    *metrics.Collectors
	tunDev        tun.TunDevice
	tunDemux      *tunnel.TunDemux
	natMgr        nat.Manager
	tlsCfg        *tls.Config
	originChecker func(*http.Request) bool
	bwMgr         *session.TokenBandwidthManager
	masterKey     []byte
	rl            *ratelimit.IPRateLimiter
	prl           *ratelimit.SessionPacketLimiter
	mux           *http.ServeMux
	httpServer    *http.Server
	startTime     time.Time
	shutdownTO    time.Duration
}

func New(configPath string) (*Server, error) {
	cfg, err := config.LoadServerConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	logger, lvl, err := pkglog.New(cfg.Logging.Level)
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	logger.Info("starting server", zap.String("listen", cfg.Listen))

	cidrMatcher, err := acl.NewCIDRMatcher(cfg.ACL.AllowCIDRs, cfg.ACL.DenyCIDRs)
	if err != nil {
		return nil, fmt.Errorf("cidr matcher: %w", err)
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
		return nil, fmt.Errorf("create ip pool: %w", err)
	}

	var pool6 *session.IPPool
	var bolt, bolt6 *session.BoltStore
	if cfg.BoltDBPath != "" {
		bolt, err = session.NewBoltStore(cfg.BoltDBPath, logger)
		if err != nil {
			logger.Warn("bolt db init, using in-memory pool", zap.Error(err))
		} else {
			pool.SetBoltStore(bolt)
			if err := pool.LoadFromBolt(); err != nil {
				logger.Warn("bolt db load", zap.Error(err))
			}
			logger.Info("ip pool loaded from bolt", zap.String("path", cfg.BoltDBPath))
		}
	}

	sm := session.NewSessionManager(pool, logger)

	if cfg.Network.PoolIPv6.Subnet != "" {
		pool6, err = session.NewIPPool6(session.PoolCfg{
			Subnet:  cfg.Network.PoolIPv6.Subnet,
			Gateway: cfg.Network.PoolIPv6.Gateway,
		}, logger)
		if err != nil {
			logger.Warn("create ipv6 pool, running ipv4-only", zap.Error(err))
		} else {
			if cfg.BoltDBPath != "" {
				bolt6, err = session.NewBoltStore6(cfg.BoltDBPath, logger)
				if err != nil {
					logger.Warn("bolt db6 init", zap.Error(err))
				} else {
					pool6.SetBoltStore(bolt6)
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

	collectors := metrics.NewCollectors()

	tunDev := tun.NewTunDevice()
	if err := tunDev.Open(); err != nil {
		return nil, fmt.Errorf("open tun: %w", err)
	}

	gatewayIP := net.ParseIP(cfg.Network.PoolIPv4.Gateway)
	_, subnet, _ := net.ParseCIDR(cfg.Network.PoolIPv4.Subnet)
	if err := tunDev.SetIP(gatewayIP, subnet); err != nil {
		return nil, fmt.Errorf("set tun ip: %w", err)
	}

	if cfg.Network.PoolIPv6.Subnet != "" {
		gatewayIPv6 := net.ParseIP(cfg.Network.PoolIPv6.Gateway)
		_, v6Subnet, _ := net.ParseCIDR(cfg.Network.PoolIPv6.Subnet)
		if err := tunDev.SetIP(gatewayIPv6, v6Subnet); err != nil {
			logger.Warn("set tun ipv6", zap.Error(err))
		}
	}

	if err := tunDev.DisableGSO(); err != nil {
		logger.Warn("disable gso", zap.Error(err))
	} else {
		logger.Info("gso/gro disabled on tun")
	}

	tunDemux := tunnel.NewTunDemux(tunDev, logger)

	natMgr := nat.NewManager()
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
		return nil, fmt.Errorf("tls config: %w", err)
	}
	if cfg.TLS.ClientCAFile != "" {
		logger.Info("mtls enabled", zap.String("client_ca", cfg.TLS.ClientCAFile), zap.String("client_auth", cfg.TLS.ClientAuth))
	}

	originChecker := websocket.NewOriginChecker(cfg.Origin.Whitelist, cfg.Origin.AllowEmpty)

	bwCfg := make(map[string]int)
	for _, tc := range cfg.Auth.Tokens {
		bwCfg[tc.Name] = tc.BandwidthBPS
	}
	bwMgr := session.NewTokenBandwidthManager(bwCfg)

	var masterKey []byte
	if cfg.Crypto.Enabled {
		masterKey, err = crypto.ParseMasterKey(cfg.Crypto.Key)
		if err != nil {
			return nil, fmt.Errorf("crypto key: %w", err)
		}
		logger.Info("app-layer encryption enabled")
	} else {
		logger.Info("app-layer encryption disabled")
	}

	rl := ratelimit.NewIPRateLimiter(cfg.RateLimiting.AuthBurst, cfg.RateLimiting.AuthPerMinute)
	prl := ratelimit.NewSessionPacketLimiter(cfg.RateLimiting.PacketsPerSec)

	s := &Server{
		cfg:     cfg,
		cfgPath: configPath,
		cfgPtr:  config.NewAtomicConfig(cfg),
		logger:  logger,
		lvl:     lvl,

		cidrMatcher:   cidrMatcher,
		gatewayIP:     gatewayIP,
		pool:          pool,
		pool6:         pool6,
		bolt:          bolt,
		bolt6:         bolt6,
		sm:            sm,
		collectors:    collectors,
		tunDev:        tunDev,
		tunDemux:      tunDemux,
		natMgr:        natMgr,
		tlsCfg:        tlsCfg,
		originChecker: originChecker,
		bwMgr:         bwMgr,
		masterKey:     masterKey,
		rl:            rl,
		prl:           prl,
		startTime:     time.Now(),
		shutdownTO:    shutdownTimeout,
	}

	s.mux = s.buildMux()
	s.httpServer = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 20 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s, nil
}

// @sk-task whitelist-obfuscation#T3.2: padding config in WSConfig (AC-005)
func (s *Server) buildMux() *http.ServeMux {
	mux := http.NewServeMux()

	tunnelHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// @sk-task whitelist-obfuscation#T2.2: WS path allowlist check (AC-003)
		if !s.allowedWSPath(r.URL.Path) {
			http.Error(w, "404 not found", http.StatusNotFound)
			return
		}
		if !s.rl.Allow(r.RemoteAddr) {
			pkglog.Audit(s.logger, zapcore.WarnLevel, "auth rate limited",
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("reason", "rate limit exceeded"),
			)
			http.Error(w, "429 too many requests", http.StatusTooManyRequests)
			return
		}
		paddingEnabled := s.cfg.Obfuscation != nil && s.cfg.Obfuscation.Padding != nil && s.cfg.Obfuscation.Padding.Enabled
		wsCfg := websocket.WSConfig{
			Multiplex:      s.cfg.Multiplex,
			MTU:            s.cfg.MTU,
			PaddingEnabled: paddingEnabled,
			PaddingSize:    paddingSizeOrDefault(s.cfg.Obfuscation),
		}
		s.handleTunnel(w, r, wsCfg)
	})

	cidrMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			ip := net.ParseIP(host)
			if ip != nil && !s.cidrMatcher.Allowed(ip) {
				pkglog.Audit(s.logger, zapcore.WarnLevel, "connection denied by CIDR ACL",
					zap.String("remote_addr", r.RemoteAddr),
					zap.String("ip", host),
				)
				http.Error(w, "403 forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	mux.Handle("/", cidrMiddleware(tunnelHandler))
	mux.HandleFunc("/livez", s.handleLivez)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/health", s.handleHealth)

	mrl := ratelimit.NewIPRateLimiter(100, 100)
	metricsHandler := admin.TokenMiddleware(s.cfg.Admin.Token)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !mrl.Allow(r.RemoteAddr) {
			http.Error(w, "429 too many requests", http.StatusTooManyRequests)
			return
		}
		promhttp.Handler().ServeHTTP(w, r)
	}))
	mux.Handle("/metrics", metricsHandler)

	return mux
}

// @sk-task fix-critical-leaks#T1.3: log swallowed errors (AC-009)
func (s *Server) handleLivez(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "alive",
		"uptime_s": time.Since(s.startTime).Seconds(),
	}); err != nil {
		s.logger.Error("livez encode failed", zap.Error(err))
	}
}

// @sk-task fix-critical-leaks#T1.3: log swallowed errors (AC-009)
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	boltStatus := "disabled"
	if s.bolt != nil {
		boltStatus = "ok"
	}
	sessions := s.sm.List()
	health := map[string]interface{}{
		"status":          "ok",
		"uptime_s":        time.Since(s.startTime).Seconds(),
		"tun":             "ok",
		"nat":             "ok",
		"active_sessions": len(sessions),
		"bolt":            boltStatus,
	}
	if err := json.NewEncoder(w).Encode(health); err != nil {
		s.logger.Error("readyz encode failed", zap.Error(err))
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.handleReadyz(w, r)
}

// @sk-task quic-obfuscation#T2.3: wrap QUICConn after Accept if obfuscation (AC-001, AC-002)
// @sk-task whitelist-obfuscation#T4.1: QUIC isClient param removed (AC-006)
func (s *Server) Run(ctx context.Context) error {
	defer s.logger.Info("server stopped")

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	s.startSighupHandler(ctx)
	defer s.sm.Stop()
	defer func() { _ = s.tunDev.Close() }()
	defer func() {
		if err := s.natMgr.Teardown(); err != nil {
			s.logger.Warn("nat teardown", zap.Error(err))
		}
		if s.cfg.Network.PoolIPv6.Subnet != "" {
			if err := s.natMgr.Teardown6(); err != nil {
				s.logger.Warn("ipv6 nat teardown", zap.Error(err))
			}
		}
	}()
	if s.bolt != nil {
		defer func() { _ = s.bolt.Close() }()
	}
	if s.bolt6 != nil {
		defer func() { _ = s.bolt6.Close() }()
	}

	s.rl.StartCleanup(ctx)
	s.prl.StartCleanup(ctx)

	tlsListener, err := tls.Listen("tcp", s.cfg.Listen, s.tlsCfg)
	if err != nil {
		return fmt.Errorf("tls listen: %w", err)
	}
	defer func() { _ = tlsListener.Close() }()

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		s.logger.Info("listening", zap.String("addr", s.cfg.Listen))
		if err := s.httpServer.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	eg.Go(func() error {
		<-ctx.Done()
		s.logger.Info("shutting down http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTO)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	})

	if s.cfg.Admin.Enabled {
		s.startAdminAPI(ctx, eg)
	}

	if s.cfg.Transport == "quic" {
		quicCfg := &quic.Config{
			KeepAlivePeriod: 15 * time.Second,
		}
		quicListener, err := quictp.Listen(s.cfg.Listen, s.tlsCfg, quicCfg)
		if err != nil {
			return fmt.Errorf("quic listen: %w", err)
		}
		defer func() { _ = quicListener.Close() }()

		s.logger.Info("quic listening", zap.String("addr", quicListener.Addr()))
		eg.Go(func() error {
			for {
				quicConn, err := quicListener.Accept(ctx)
				if err != nil {
					return fmt.Errorf("quic accept: %w", err)
				}
				if s.cfg.Obfuscation != nil && s.cfg.Obfuscation.Enabled {
					s.logger.Debug("quic obfuscation enabled, wrapping connection")
					obfConn, obfErr := quictp.NewObfuscatedQUICConn(quicConn)
					if obfErr != nil {
						s.logger.Error("quic obfuscation init failed, closing connection", zap.Error(obfErr))
						_ = quicConn.Close()
						continue
					}
					go s.handleStream(ctx, obfConn, s.cfg.MTU, quicListener.Addr())
					continue
				}
				go s.handleStream(ctx, quicConn, s.cfg.MTU, quicListener.Addr())
			}
		})
	}

	return eg.Wait()
}

func (s *Server) startSighupHandler(ctx context.Context) {
	sighupCh := make(chan os.Signal, 1)
	signal.Notify(sighupCh, syscall.SIGHUP)
	go func() {
		var mu sync.Mutex
		for {
			select {
			case <-ctx.Done():
				signal.Stop(sighupCh)
				return
			case <-sighupCh:
			}
			mu.Lock()
			s.logger.Info("sighup received, reloading config")
			newCfg, err := config.LoadServerConfig(s.cfgPath)
			if err != nil {
				s.logger.Warn("config reload failed, keeping old config", zap.Error(err))
				mu.Unlock()
				continue
			}
			s.cfgPtr.Store(newCfg)
			if newCfg.Logging.Level != "" {
				var lvl zapcore.Level
				if err := lvl.UnmarshalText([]byte(newCfg.Logging.Level)); err == nil {
					s.lvl.SetLevel(lvl)
				}
			}
			s.logger.Info("config reloaded",
				zap.Int("tokens", len(newCfg.Auth.Tokens)),
				zap.String("log_level", s.lvl.Level().String()),
			)
			mu.Unlock()
		}
	}()
}

func (s *Server) startAdminAPI(ctx context.Context, eg *errgroup.Group) {
	adminCfg := admin.AdminCfg{
		Enabled: s.cfg.Admin.Enabled,
		Listen:  s.cfg.Admin.Listen,
		Token:   s.cfg.Admin.Token,
	}
	adminSrv := admin.NewAdminServer(adminCfg, s.sm, s.logger)

	if s.cfg.Admin.Token == "" {
		s.logger.Warn("admin api token not set, disabling admin api")
		return
	}
	addr := s.cfg.Admin.Listen
	if !strings.HasPrefix(addr, "localhost:") && !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "unix:") {
		s.logger.Warn("admin api listening on non-loopback interface", zap.String("addr", addr))
	}
	s.logger.Info("admin api enabled", zap.String("listen", addr))

	eg.Go(func() error {
		if err := adminSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("admin api: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTO)
		defer cancel()
		return adminSrv.Shutdown(shutdownCtx)
	})
}
