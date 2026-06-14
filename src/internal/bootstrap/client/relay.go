package client

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
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	tlspkg "github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	"github.com/bzdvdn/kvn-ws/src/internal/transport/websocket"
	quictp "github.com/bzdvdn/kvn-ws/src/internal/transport/quic"
	"github.com/bzdvdn/kvn-ws/src/internal/protocol/control"
)

// @sk-task client-relay-mode#T2.1: relay mode entry point (AC-003)
// @sk-task client-relay-mode#T3.1: max_connections semaphore (RQ-011)
// @sk-task quic-relay-mode#T2.1: dual listener via errgroup (AC-001)
// @sk-task quic-relay-mode#T3.1: QUIC accept loop (AC-001)
func (c *Client) runRelayMode(ctx context.Context) error {
	tlsCfg, err := c.relayTLSConfig()
	if err != nil {
		return fmt.Errorf("relay tls: %w", err)
	}

	tlsListener, err := tls.Listen("tcp", c.cfg.Relay.Listen, tlsCfg)
	if err != nil {
		return fmt.Errorf("relay tls listen: %w", err)
	}
	defer tlsListener.Close()

	c.logger.Info("relay listening", zap.String("addr", c.cfg.Relay.Listen))

	sem := make(chan struct{}, c.cfg.Relay.MaxConnections)

	eg, egCtx := errgroup.WithContext(ctx)

	srv := &http.Server{
		Handler:           c.relayServeMux(sem),
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

	if c.cfg.Relay.Quic != nil {
		quicCfg := &quic.Config{
			KeepAlivePeriod: time.Duration(c.cfg.Relay.Quic.KeepAlive) * time.Second,
			MaxIdleTimeout:  time.Duration(c.cfg.Relay.Quic.IdleTimeout) * time.Second,
		}
		quicListener, err := quictp.Listen(c.cfg.Relay.Listen, tlsCfg, quicCfg)
		if err != nil {
			return fmt.Errorf("relay quic listen: %w", err)
		}
		defer quicListener.Close()

		c.logger.Info("quic relay listening", zap.String("addr", c.cfg.Relay.Listen))

		eg.Go(func() error {
			return c.runRelayQUICAccept(egCtx, quicListener, sem)
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

// @sk-task quic-relay-mode#T3.1: QUIC accept loop (AC-001, AC-005)
func (c *Client) runRelayQUICAccept(ctx context.Context, listener *quictp.Listener, sem chan struct{}) error {
	for {
		quicConn, err := listener.Accept(ctx)
		if err != nil {
			return err
		}
		select {
		case sem <- struct{}{}:
		default:
			c.logger.Warn("relay: max connections reached, rejecting quic client")
			_ = quicConn.Close()
			continue
		}
		go func(conn transport.StreamConn) {
			defer func() { <-sem }()
			c.bridgeRelayConn(ctx, conn, fmt.Sprintf("quic-stream-%d", conn.(*quictp.QUICConn).StreamID()))
		}(quicConn)
	}
}

// @sk-task quic-relay-mode#T2.1: relay HTTP mux with WS handler and global semaphore (AC-001)
func (c *Client) relayServeMux(sem chan struct{}) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			http.Error(w, "403 forbidden", http.StatusForbidden)
			return
		}
		c.relayHandler(w, r)
	})
	return mux
}

// @sk-task client-relay-mode#T3.1: self-signed TLS fallback (AC-003)
func (c *Client) relayTLSConfig() (*tls.Config, error) {
	if c.cfg.Relay.TLS != nil && c.cfg.Relay.TLS.Cert != "" && c.cfg.Relay.TLS.Key != "" {
		return tlspkg.NewServerTLSConfig(c.cfg.Relay.TLS.Cert, c.cfg.Relay.TLS.Key, "", "")
	}

	c.logger.Warn("relay.tls not configured, generating self-signed certificate")
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

// @sk-task client-relay-mode#T2.1: WS path allowlist check (AC-003)
func (c *Client) allowedRelayPath(path string) bool {
	for _, p := range c.cfg.Relay.WSPaths {
		if p == path {
			return true
		}
	}
	return false
}

// @sk-task client-relay-mode#T2.1: relay HTTP handler (AC-003, RQ-002)
func (c *Client) relayHandler(w http.ResponseWriter, r *http.Request) {
	if !isWebSocketRequest(r) {
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}

	if !c.allowedRelayPath(r.URL.Path) {
		http.Error(w, "404 not found", http.StatusNotFound)
		return
	}

	wsConn, err := websocket.Accept(w, r, c.logger)
	if err != nil {
		c.logger.Error("relay ws upgrade", zap.Error(err))
		return
	}

	c.bridgeRelayConn(r.Context(), wsConn, r.RemoteAddr)
}

func isWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// @sk-task client-relay-mode#T2.2: minimal upstream dial — relay is opaque pipe, no obfuscation/padding
func (c *Client) dialRelayUpstream(ctx context.Context) (transport.StreamConn, error) {
	tlsCfg, err := clientTLSConfig(c.cfg)
	if err != nil {
		return nil, err
	}

	conn, err := websocket.Dial(c.cfg.Server, tlsCfg, c.logger)
	if err != nil {
		return nil, err
	}
	conn.SetKeepalive(control.DefaultPingInterval, control.DefaultPongTimeout)
	return conn, nil
}

// @sk-task client-relay-mode#T2.2: bridge session — handshake forward + data bridge (AC-001, AC-002, AC-005)
// @sk-task client-relay-mode#T3.1: upstream failure handling (AC-004)
func (c *Client) bridgeRelayConn(ctx context.Context, clientConn transport.StreamConn, remoteAddr string) {
	defer func() { _ = clientConn.Close() }()

	clientConn.SetReadDeadline(time.Now().Add(30 * time.Second))
	clientHello, err := clientConn.ReadMessage()
	if err != nil {
		c.logger.Warn("relay: read client hello timeout", zap.Error(err))
		return
	}
	clientConn.SetReadDeadline(time.Time{})

	upstreamConn, err := c.dialRelayUpstream(ctx)
	if err != nil {
		c.logger.Warn("upstream dial failed, rejecting client", zap.Error(err))
		return
	}
	defer func() { _ = upstreamConn.Close() }()

	c.logger.Debug("relay: forwarding clientHello", zap.Int("len", len(clientHello)))

	if err := upstreamConn.WriteMessage(clientHello); err != nil {
		c.logger.Warn("relay: forward client hello failed", zap.Error(err))
		return
	}

	serverHello, err := upstreamConn.ReadMessage()
	if err != nil {
		c.logger.Warn("relay: read server hello failed", zap.Error(err))
		return
	}

	if err := clientConn.WriteMessage(serverHello); err != nil {
		c.logger.Warn("relay: forward server hello failed", zap.Error(err))
		return
	}

	c.logger.Info("handshake forwarded", zap.String("remote", remoteAddr))

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
		copyDirection("client->upstream", clientConn, upstreamConn, closeBoth, c.logger)
	}()
	go func() {
		defer wg.Done()
		copyDirection("upstream->client", upstreamConn, clientConn, closeBoth, c.logger)
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
