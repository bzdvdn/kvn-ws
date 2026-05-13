// @sk-task foundation#T1.1: Go module init (AC-001)
// @sk-task foundation#T3.2: client main with graceful shutdown (AC-010)

package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
	"go.uber.org/zap"
)

func main() {
	cfgPath := flag.String("config", "configs/client.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.LoadClientConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	logger, err := logger.New(cfg.Log.Level)
	if err != nil {
		log.Fatalf("logger: %v", err)
	}

	logger.Info("starting client", zap.String("server", cfg.Server))
	defer logger.Info("client stopped")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	<-ctx.Done()
	logger.Info("shutting down")
}
