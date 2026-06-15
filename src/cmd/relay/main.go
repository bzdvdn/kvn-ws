// @sk-task relay-terminator#T1.2: relay entrypoint (AC-001)
package main

import (
	"context"
	"log"

	"github.com/bzdvdn/kvn-ws/src/internal/bootstrap/relay"
)

func main() {
	r, err := relay.New()
	if err != nil {
		log.Fatalf("relay: %v", err)
	}

	if err := r.Run(context.Background()); err != nil {
		log.Printf("relay stopped with error: %v", err)
	}
}
