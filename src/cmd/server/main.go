package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
	"github.com/bzdvdn/kvn-ws/src/internal/server"
	"github.com/bzdvdn/kvn-ws/src/internal/transport"
	tlspkg "github.com/bzdvdn/kvn-ws/src/internal/transport/tls"
	"github.com/bzdvdn/kvn-ws/src/internal/tunpool"
)

// @sk-task kvn-v2-transport#T1.3: v2 server entrypoint (AC-001, AC-003)
func main() {
	configPath := flag.String("config", "configs/server.yaml", "path to v2 server config")
	exportCover := flag.String("export-cover-site", "", "export the built-in cover site to a directory and exit")
	flag.Parse()

	if *exportCover != "" {
		if err := server.ExportCoverSite(*exportCover); err != nil {
			log.Fatalf("export cover site: %v", err)
		}
		fmt.Printf("cover site exported to %s\n", *exportCover)
		return
	}

	cfg, err := config.LoadV2ServerConfig(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	logger, _, err := logger.New("info")
	if err != nil {
		log.Fatalf("logger: %v", err)
	}
	tokens := make(map[string]string, len(cfg.Auth.Tokens))
	for _, t := range cfg.Auth.Tokens {
		tokens[t.Secret] = t.Name
	}
	lookup := func(token string) (string, bool) {
		name, ok := tokens[token]
		return name, ok
	}
	pool, perr := tunpool.New(&tunpool.Config{
		Subnet: cfg.TUN.Subnet, RangeStart: cfg.TUN.RangeStart, RangeEnd: cfg.TUN.RangeEnd,
		Gateway: cfg.TUN.Gateway, LeaseTTLSec: cfg.TUN.LeaseTTLSec, LeaseFile: cfg.TUN.LeaseFile,
	})
	if perr != nil {
		log.Fatalf("tun pool: %v", perr)
	}
	fallback := server.NewFallback(&cfg.Fallback)
	handler := transport.NewServer(lookup, fallback, cfg.MaxStreams, logger, transport.WithTUNPool(pool))

	sniCerts := make(map[string]tlspkg.CertKeyPair, len(cfg.TLS.SNICerts))
	for name, pair := range cfg.TLS.SNICerts {
		sniCerts[name] = tlspkg.CertKeyPair{CertFile: pair.Cert, KeyFile: pair.Key}
	}
	tlsCfg, terr := tlspkg.NewPublicServerTLSConfig(cfg.TLS.Cert, cfg.TLS.Key, cfg.TLS.ClientCAFile, cfg.TLS.ClientAuth, sniCerts)
	if terr != nil {
		log.Fatalf("tls: %v", terr)
	}
	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		TLSConfig:         tlsCfg,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if cfg.QuicListen != "" {
		cert, cerr := tls.LoadX509KeyPair(cfg.TLS.Cert, cfg.TLS.Key)
		if cerr != nil {
			logger.Warn("v2 QUIC disabled: load certificate", zap.Error(cerr))
		} else {
			quicSrv, qerr := transport.ListenQUIC(cfg.QuicListen, &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS13,
			}, lookup, logger)
			if qerr != nil {
				logger.Warn("v2 QUIC listen failed", zap.String("addr", cfg.QuicListen), zap.Error(qerr))
			} else {
				logger.Info("v2 QUIC datagrams listening", zap.String("addr", cfg.QuicListen))
				go func() { _ = quicSrv.Serve(ctx) }()
			}
		}
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	logger.Info("v2 server listening", zap.String("addr", cfg.Listen))
	if err := httpServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "v2 server: %v\n", err)
		os.Exit(1) //nolint:gocritic // terminal error path; deferred signal stop is irrelevant
	}
}
