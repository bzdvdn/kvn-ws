package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bzdvdn/kvn-ws/src/internal/client"
	"github.com/bzdvdn/kvn-ws/src/internal/config"
	"github.com/bzdvdn/kvn-ws/src/internal/logger"
)

// @sk-task kvn-v2-transport#T2.0: v2 proxy client entrypoint for early testing (AC-001)
func main() {
	configPath := flag.String("config", "configs/client.yaml", "path to v2 client config")
	flag.Parse()
	cfg, err := config.LoadV2ClientConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	if cfg.Mode == "" {
		cfg.Mode = "proxy"
	}
	logger, _, err := logger.New("info")
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: %v\n", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if err := client.New(cfg, logger).Run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "v2 client: %v\n", err)
		os.Exit(1) //nolint:gocritic // terminal error path; deferred signal stop is irrelevant
	}
}
