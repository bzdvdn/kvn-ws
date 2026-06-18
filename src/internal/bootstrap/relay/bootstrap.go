package relay

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/bzdvdn/kvn-ws/src/internal/session"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	quictp "github.com/bzdvdn/kvn-ws/src/internal/transport/quic"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
	"github.com/bzdvdn/kvn-ws/src/internal/tunnel"
)

// @sk-task relay-terminator#T2.1: init terminator state (AC-001, AC-004, AC-005)
// @sk-task relay-terminator#T2.2: TUN setup (AC-005)
// @sk-task relay-terminator#T9.2: IPv6 pool support (AC-001)
// @sk-task relay-terminator#T6.3: wire DNS config through initTerminator (RQ-008, RQ-011)
func (r *Relay) initTerminator() error {
	netCfg := r.cfg.Relay.Network
	if netCfg == nil {
		return fmt.Errorf("relay.network is required for terminator mode")
	}

	if err := r.enableIPForward(); err != nil {
		r.logger.Warn("ip_forward enable", zap.Error(err))
	}

	pool, err := session.NewIPPool(session.PoolCfg{
		Subnet:     netCfg.PoolIPv4.Subnet,
		Gateway:    netCfg.PoolIPv4.Gateway,
		RangeStart: netCfg.PoolIPv4.RangeStart,
		RangeEnd:   netCfg.PoolIPv4.RangeEnd,
	}, r.logger)
	if err != nil {
		return fmt.Errorf("create ip pool: %w", err)
	}
	r.pool = pool

	if netCfg.PoolIPv6.Subnet != "" {
		pool6, err := session.NewIPPool6(session.PoolCfg{
			Subnet:  netCfg.PoolIPv6.Subnet,
			Gateway: netCfg.PoolIPv6.Gateway,
		}, r.logger)
		if err != nil {
			r.logger.Warn("create ipv6 pool, running ipv4-only", zap.Error(err))
		} else {
			r.pool6 = pool6
		}
	}

	r.sm = session.NewSessionManager(pool, r.logger)
	if r.pool6 != nil {
		r.sm.SetPool6(r.pool6)
	}
	r.sm.Start(300*time.Second, 24*time.Hour, 10*time.Second)

	r.tunDev = tun.NewTunDevice()
	if err := r.tunDev.Open(); err != nil {
		return fmt.Errorf("open tun: %w", err)
	}

	gatewayIP := net.ParseIP(netCfg.PoolIPv4.Gateway)
	_, subnet, _ := net.ParseCIDR(netCfg.PoolIPv4.Subnet)
	if err := r.tunDev.SetIP(gatewayIP, subnet); err != nil {
		return fmt.Errorf("set tun ip: %w", err)
	}

	if r.pool6 != nil {
		gatewayIPv6 := net.ParseIP(netCfg.PoolIPv6.Gateway)
		_, v6Subnet, _ := net.ParseCIDR(netCfg.PoolIPv6.Subnet)
		if err := r.tunDev.SetIP(gatewayIPv6, v6Subnet); err != nil {
			r.logger.Warn("set tun ipv6", zap.Error(err))
		}
	}

	if err := r.tunDev.DisableGSO(); err != nil {
		r.logger.Warn("disable gso", zap.Error(err))
	}

	r.tunDemux = tunnel.NewTunDemux(r.tunDev, r.logger)

	tlsCfg, err := r.relayTLSConfig()
	if err != nil {
		return fmt.Errorf("tls config: %w", err)
	}
	r.tlsCfg = tlsCfg

	r.nat = newNATTracker(r.logger)

	if r.cfg.Relay.Routing != nil {
		ruleSet, err := newDirectRuleSet(r.cfg.Relay.Routing, r.logger)
		if err != nil {
			return fmt.Errorf("routing rule set: %w", err)
		}
		r.ruleSet = ruleSet

		if dnsCfg := r.cfg.Relay.Routing.DNS; dnsCfg != nil {
			r.dnsEnabled = true
			r.dnsUpstream = dnsCfg.Upstream
			if r.dnsUpstream == "" {
				r.dnsUpstream = "1.1.1.1:53"
			}
			ttl := 60
			if dnsCfg.CacheTTL > 0 {
				ttl = dnsCfg.CacheTTL
			}
			r.cacheTTL = time.Duration(ttl) * time.Second
			r.dnsCache = make(map[netip.Addr]time.Time)
		}
	}

	return nil
}

// @sk-task relay-terminator#T3.1: upstream dial + routing wire (AC-002, AC-003, AC-004)
// @sk-task relay-terminator#T3.2: upstream tunnel lifecycle (AC-004)
// @sk-task relay-terminator#T5.2: upstream transport auto-select (AC-004)
func (r *Relay) connectUpstream(ctx context.Context) error {
	if r.cfg.Server == "" {
		r.logger.Warn("no upstream server configured, upstream routing will drop packets")
		r.upstreamConn = true
		return nil
	}
	us, err := dialUpstream(ctx, r.ctx, r.cfg, r.tunDev, r.logger, r.clientTransport, r.nat)
	if err != nil {
		return fmt.Errorf("upstream connect: %w", err)
	}
	r.logger.Info("upstream connected",
		zap.String("server", r.cfg.Server),
		zap.String("assigned_ip", us.assignedIP.String()),
	)
	r.upstream.Store(us)
	return nil
}

// @sk-task relay-terminator#T7.2: lazy upstream connect with retry (RQ-014)
func (r *Relay) ensureUpstream(ctx context.Context) error {
	r.upstreamMu.Lock()
	defer r.upstreamMu.Unlock()
	if r.upstreamConn {
		return nil
	}
	if err := r.connectUpstream(ctx); err != nil {
		return err
	}
	r.upstreamConn = true
	return nil
}

// @sk-task relay-terminator#T2.1: terminator accept loops (AC-001, AC-004)
// @sk-task relay-terminator#T2.2: TUN lifecycle (AC-005)
// @sk-task relay-terminator#T2.3: cleanup at disconnect (AC-006)
// @sk-task relay-terminator#T8.6: decouple receiveLoop context from client request (AC-001)
func (r *Relay) runTerminator(ctx context.Context) error {
	r.ctx = ctx
	if err := r.initTerminator(); err != nil {
		return fmt.Errorf("terminator init: %w", err)
	}

	defer r.logger.Info("terminator stopped")
	defer r.sm.Stop()
	defer func() { _ = r.tunDev.Close() }()

	if r.cfg.Transport != "" {
		if err := r.connectUpstream(ctx); err != nil {
			r.logger.Warn("upstream connect failed, will retry on first client", zap.Error(err))
		}
	} else {
		r.logger.Info("upstream transport auto-select enabled, connecting on first client")
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		r.handleTerminatorWS(w, req)
	})

	r.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 20 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	tlsListener, err := tls.Listen("tcp", r.cfg.Relay.Listen, r.tlsCfg)
	if err != nil {
		return fmt.Errorf("tls listen: %w", err)
	}
	defer func() { _ = tlsListener.Close() }()

	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		r.logger.Info("terminator listening", zap.String("addr", r.cfg.Relay.Listen))
		if err := r.httpServer.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	eg.Go(func() error {
		<-egCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return r.httpServer.Shutdown(shutdownCtx)
	})

	if r.cfg.Relay.Quic != nil {
		quicCfg := &quic.Config{
			KeepAlivePeriod: time.Duration(r.cfg.Relay.Quic.KeepAlive) * time.Second,
			MaxIdleTimeout:  time.Duration(r.cfg.Relay.Quic.IdleTimeout) * time.Second,
		}
		quicListener, err := quictp.Listen(r.cfg.Relay.Listen, r.tlsCfg, quicCfg)
		if err != nil {
			return fmt.Errorf("quic listen: %w", err)
		}
		defer func() { _ = quicListener.Close() }()

		r.logger.Info("terminator quic listening", zap.String("addr", quicListener.Addr()))
		eg.Go(func() error {
			for {
				quicConn, err := quicListener.Accept(egCtx)
				if err != nil {
					return fmt.Errorf("quic accept: %w", err)
				}
				clientStream := transport.StreamConn(quicConn)
				if r.cfg.Obfuscation != nil && r.cfg.Obfuscation.Enabled {
					obfConn, obfErr := quictp.NewObfuscatedQUICConn(quicConn)
					if obfErr != nil {
						r.logger.Error("quic obfuscation init failed, closing connection", zap.Error(obfErr))
						_ = quicConn.Close()
						continue
					}
					clientStream = obfConn
				}
				go r.handleTerminatorStream(egCtx, clientStream, quicListener.Addr(), "quic")
			}
		})
	}

	return eg.Wait()
}

// @sk-task relay-terminator#T8.2: async upstream reconnect serialised by upstreamMu (AC-007)
func (r *Relay) reconnectUpstream() {
	r.upstreamMu.Lock()
	defer r.upstreamMu.Unlock()

	us := r.upstream.Load()
	if us != nil && !us.isClosed() {
		return
	}

	r.logger.Info("reconnecting upstream")
	r.upstreamConn = false
	if err := r.connectUpstream(r.ctx); err != nil {
		r.logger.Error("upstream reconnect failed", zap.Error(err))
		return
	}
	r.upstreamConn = true
	r.logger.Info("upstream reconnected")
}

// @sk-task relay-terminator#T8.3: enable ip_forward for DNAT response routing (RQ-015)
func (r *Relay) enableIPForward() error {
	return exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
}
