package relay

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/netip"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"time"

	"github.com/quic-go/quic-go"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/control"
	"github.com/bzdvdn/kvn-ws/src/internal/routing"
	"github.com/bzdvdn/kvn-ws/src/internal/session"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	quictp "github.com/bzdvdn/kvn-ws/src/internal/transport/quic"
	tlspkg "github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	"github.com/bzdvdn/kvn-ws/src/internal/tun"
	"github.com/bzdvdn/kvn-ws/src/internal/tunnel"
)

// @sk-task relay-terminator#T1.3: Relay struct (AC-001)
// @sk-task relay-terminator#T2.1: add terminator fields (AC-001, AC-004)
// @sk-task geoip-geosite-integration#T3.3: add configPath for source cache
type Relay struct {
	cfg        *config.RelayConfig
	logger     *zap.Logger
	ctx        context.Context
	configPath string

	pool       *session.IPPool
	pool6      *session.IPPool
	sm         *session.SessionManager
	tunDev     tun.TunDevice
	tunDemux   *tunnel.TunDemux
	tlsCfg     *tls.Config
	httpServer *http.Server

	clientTransport string

	upstreamMu   sync.Mutex
	upstreamConn bool

	// Routing and forwarding state
	ruleSet  *routing.RuleSet
	nat      *natTracker
	upstream atomic.Pointer[upstreamSession]

	dnsUpstream string
	dnsCache    map[netip.Addr]time.Time
	dnsCacheMu  sync.RWMutex
	cacheTTL    time.Duration
	dnsEnabled  bool
	dnsConnPool *sync.Pool
}

// @sk-task relay-terminator#T1.3: New creates Relay with CLI flags (AC-001)
func New() (*Relay, error) {
	cfgPath := pflag.String("config", "configs/relay.yaml", "path to config file")
	pflag.Parse()

	cfg, err := config.LoadRelayConfig(*cfgPath)
	if err != nil {
		return nil, err
	}

	logger, _, err := logger.New(cfg.Log.Level)
	if err != nil {
		return nil, err
	}

	logger.Info("starting relay", zap.String("mode", cfg.Relay.Mode), zap.String("listen", cfg.Relay.Listen))
	return &Relay{cfg: cfg, logger: logger, configPath: *cfgPath}, nil
}

// @sk-task relay-terminator#T1.3: NewFromConfig creates Relay from existing config (AC-001)
// @sk-task geoip-geosite-integration#T3.3: store configPath for source cache
func NewFromConfig(cfg *config.RelayConfig, log *zap.Logger, configPath string) *Relay {
	return &Relay{cfg: cfg, logger: log, configPath: configPath}
}

// @sk-task relay-terminator#T1.3: Run dispatches to bridge or terminator (AC-001)
func (r *Relay) Run(ctx context.Context) error {
	defer r.logger.Info("relay stopped")

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	switch r.cfg.Relay.Mode {
	case "terminator":
		return r.runTerminatorMode(ctx)
	default:
		return r.runBridgeMode(ctx)
	}
}

// @sk-task relay-terminator#T1.3: bridge mode entry (AC-001)
func (r *Relay) runBridgeMode(ctx context.Context) error {
	tlsCfg, err := r.relayTLSConfig()
	if err != nil {
		return fmt.Errorf("relay tls: %w", err)
	}

	tlsListener, err := tls.Listen("tcp", r.cfg.Relay.Listen, tlsCfg)
	if err != nil {
		return fmt.Errorf("relay tls listen: %w", err)
	}
	defer tlsListener.Close()

	r.logger.Info("bridge listening", zap.String("addr", r.cfg.Relay.Listen))

	sem := make(chan struct{}, r.cfg.Relay.MaxConnections)

	eg, egCtx := errgroup.WithContext(ctx)

	srv := &http.Server{
		Handler:           r.relayServeMux(sem),
		ReadHeaderTimeout: 20 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	eg.Go(func() error {
		if err := srv.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	if r.cfg.Relay.Quic != nil {
		quicCfg := &quic.Config{
			KeepAlivePeriod: time.Duration(r.cfg.Relay.Quic.KeepAlive) * time.Second,
			MaxIdleTimeout:  time.Duration(r.cfg.Relay.Quic.IdleTimeout) * time.Second,
		}
		quicListener, err := quictp.Listen(r.cfg.Relay.Listen, tlsCfg, quicCfg)
		if err != nil {
			return fmt.Errorf("relay quic listen: %w", err)
		}
		defer quicListener.Close()

		r.logger.Info("quic bridge listening", zap.String("addr", r.cfg.Relay.Listen))

		eg.Go(func() error {
			return r.runBridgeQUICAccept(egCtx, quicListener, sem)
		})
	}

	eg.Go(func() error {
		<-egCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})

	return eg.Wait()
}

func (r *Relay) runBridgeQUICAccept(ctx context.Context, listener *quictp.Listener, sem chan struct{}) error {
	for {
		quicConn, err := listener.Accept(ctx)
		if err != nil {
			return err
		}
		select {
		case sem <- struct{}{}:
		default:
			r.logger.Warn("bridge: max connections reached, rejecting quic client")
			_ = quicConn.Close()
			continue
		}
		go func(conn transport.StreamConn) {
			defer func() { <-sem }()
			qConn, ok := conn.(*quictp.QUICConn)
			if !ok {
				r.logger.Warn("bridge: unexpected QUIC stream type")
				return
			}
			r.bridgeRelayConn(ctx, conn, fmt.Sprintf("quic-stream-%d", qConn.StreamID()))
		}(quicConn)
	}
}

func (r *Relay) relayServeMux(sem chan struct{}) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, rq *http.Request) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			http.Error(w, "403 forbidden", http.StatusForbidden)
			return
		}
		r.relayWSHandler(w, rq)
	})
	return mux
}

func (r *Relay) relayTLSConfig() (*tls.Config, error) {
	if r.cfg.Relay.TLS != nil && r.cfg.Relay.TLS.Cert != "" && r.cfg.Relay.TLS.Key != "" {
		return tlspkg.NewServerTLSConfig(r.cfg.Relay.TLS.Cert, r.cfg.Relay.TLS.Key, "", "")
	}

	r.logger.Warn("relay.tls not configured, generating self-signed certificate")
	cert, err := generateSelfSignedCert()
	if err != nil {
		return nil, fmt.Errorf("generate self-signed cert: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"KVN Relay"},
		},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return tls.X509KeyPair(certPEM, keyPEM)
}

func (r *Relay) allowedRelayPath(path string) bool {
	for _, p := range r.cfg.Relay.WSPaths {
		if p == path {
			return true
		}
	}
	return false
}

func (r *Relay) relayWSHandler(w http.ResponseWriter, rq *http.Request) {
	if !isWebSocketRequest(rq) {
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}

	if !r.allowedRelayPath(rq.URL.Path) {
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}

	wsConn, err := websocket.Accept(w, rq, r.logger)
	if err != nil {
		r.logger.Error("relay ws upgrade", zap.Error(err))
		return
	}

	r.bridgeRelayConn(rq.Context(), wsConn, rq.RemoteAddr)
}

func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// @sk-task transport-factory#T2.4: dialRelayUpstream uses WSFactory (AC-004)
func (r *Relay) dialRelayUpstream(ctx context.Context) (transport.StreamConn, error) {
	tlsCfg, err := relayTLSConfig(r.cfg)
	if err != nil {
		return nil, err
	}

	factoryCfg := &transport.FactoryConfig{
		TLS:               tlsCfg,
		Logger:            r.logger,
		KeepaliveInterval: control.DefaultPingInterval,
		KeepaliveTimeout:  control.DefaultPongTimeout,
	}
	factory := transport.NewFactory("ws", factoryCfg)
	return factory.Dial(ctx, r.cfg.Server)
}

func relayTLSConfig(cfg *config.RelayConfig) (*tls.Config, error) {
	tlsCfg, err := tlspkg.NewClientTLSConfigFromSettings(tlspkg.ClientTLSSettings{
		CAFile:     cfg.TLS.CAFile,
		ServerName: cfg.TLS.ServerName,
		VerifyMode: cfg.TLS.VerifyMode,
	})
	if err != nil {
		return nil, err
	}
	if sni := tlspkg.SelectSNI(cfg.TLS.SNI); sni != "" {
		tlsCfg.ServerName = sni
	}
	return tlsCfg, nil
}

func (r *Relay) bridgeRelayConn(ctx context.Context, clientConn transport.StreamConn, remoteAddr string) {
	defer func() { _ = clientConn.Close() }()

	_ = clientConn.SetReadDeadline(time.Now().Add(30 * time.Second))
	clientHello, err := clientConn.ReadMessage()
	if err != nil {
		r.logger.Warn("relay: read client hello timeout", zap.Error(err))
		return
	}
	_ = clientConn.SetReadDeadline(time.Time{})

	upstreamConn, err := r.dialRelayUpstream(ctx)
	if err != nil {
		r.logger.Warn("upstream dial failed, rejecting client", zap.Error(err))
		return
	}
	defer func() { _ = upstreamConn.Close() }()

	r.logger.Debug("relay: forwarding clientHello", zap.Int("len", len(clientHello)))

	if err := upstreamConn.WriteMessage(clientHello); err != nil {
		r.logger.Warn("relay: forward client hello failed", zap.Error(err))
		return
	}

	serverHello, err := upstreamConn.ReadMessage()
	if err != nil {
		r.logger.Warn("relay: read server hello failed", zap.Error(err))
		return
	}

	if err := clientConn.WriteMessage(serverHello); err != nil {
		r.logger.Warn("relay: forward server hello failed", zap.Error(err))
		return
	}

	r.logger.Info("handshake forwarded", zap.String("remote", remoteAddr))

	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			_ = clientConn.Close()
			_ = upstreamConn.Close()
		})
	}
	defer closeBoth()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		copyDirection("client->upstream", clientConn, upstreamConn, closeBoth, r.logger)
	}()
	go func() {
		defer wg.Done()
		copyDirection("upstream->client", upstreamConn, clientConn, closeBoth, r.logger)
	}()

	wg.Wait()
}

func copyDirection(name string, from, to transport.StreamConn, closeFn func(), logger *zap.Logger) {
	defer closeFn()
	for {
		msg, err := from.ReadMessage()
		if err != nil {
			logger.Debug("relay bridge closed", zap.String("direction", name), zap.Error(err))
			return
		}
		if err := to.WriteMessage(msg); err != nil {
			logger.Debug("relay bridge write error", zap.String("direction", name), zap.Error(err))
			return
		}
	}
}

// @sk-task relay-terminator#T2.1: terminator mode entry (AC-001, AC-004, AC-005)
func (r *Relay) runTerminatorMode(ctx context.Context) error {
	return r.runTerminator(ctx)
}
