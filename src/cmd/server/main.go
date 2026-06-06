package main

import (
	"context"
	"flag"
	"log"

	"github.com/bzdvdn/kvn-ws/src/internal/bootstrap/server"
)

// @sk-task security-acl#T3: CIDR ACL middleware integration
// @sk-task security-acl#T7: Bandwidth limiter integration
// @sk-task security-acl#T10: Admin API integration
// @sk-task docs-and-release#T5.1: fix session ID hex encoding (AC-008)
// @sk-task tun-data-path#T6.1: SetIP call fixed (AC-003)
// @sk-task foundation#T1.1: Go module init (AC-001)
// @sk-task foundation#T3.2: server main with graceful shutdown (AC-010)
// @sk-task core-tunnel-mvp#T4.1: server forwarding loops (AC-007, AC-008)
// @sk-task core-tunnel-mvp#T4.2: graceful shutdown (AC-010)
func main() {
	cfgPath := flag.String("config", "configs/server.yaml", "path to config file")
	flag.Parse()

	srv, err := server.New(*cfgPath)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	if err := srv.Run(context.Background()); err != nil {
		log.Printf("server stopped with error: %v", err)
	}
}
