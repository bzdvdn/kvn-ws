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
	"github.com/bzdvdn/kvn-ws/src/internal/v2/server"
	"github.com/bzdvdn/kvn-ws/src/internal/v2/transport"
)

// @sk-task kvn-v2-transport#T1.3: v2 server entrypoint (AC-001, AC-003)
func main() {
	configPath := flag.String("config", "configs/v2-server.yaml", "path to v2 server config")
	flag.Parse()

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
	fallback := server.NewFallback(cfg.Fallback)
	handler := transport.NewServer(lookup, fallback, cfg.MaxStreams, logger)

	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
			NextProtos: []string{"h2"},
		},
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	logger.Info("v2 server listening", zap.String("addr", cfg.Listen))
	if err := httpServer.ListenAndServeTLS(cfg.TLS.Cert, cfg.TLS.Key); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "v2 server: %v\n", err)
		os.Exit(1)
	}
}
