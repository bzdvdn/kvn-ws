// @sk-task foundation#T1.1: Go module init (AC-001)
// @sk-task foundation#T3.2: client main with graceful shutdown (AC-010)
// @sk-task core-tunnel-mvp#T4.1: client forwarding loops (AC-007, AC-008)
// @sk-task core-tunnel-mvp#T4.2: graceful shutdown (AC-010)
// @sk-task production-hardening#T4.2: reconnect + kill-switch (AC-001, AC-003)
// @sk-task production-hardening#T4.3: pflag CLI (AC-011)
package main

import (
	"context"
	"log"

	"github.com/bzdvdn/kvn-ws/src/internal/bootstrap/client"
)

func main() {
	cl, err := client.New()
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	if err := cl.Run(context.Background()); err != nil {
		log.Printf("client stopped with error: %v", err)
	}
}
