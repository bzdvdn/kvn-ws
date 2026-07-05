package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

// @sk-task kvn-desktop#T1.1: entrypoint with cli flags (AC-001, AC-002, AC-003)
func main() {
	port := flag.Int("port", 2311, "KVN Web UI port")
	serverURL := flag.String("server", "", "KVN Web UI URL (default http://127.0.0.1:<port>)")
	flag.Parse()

	url := *serverURL
	if url == "" {
		url = fmt.Sprintf("http://127.0.0.1:%d", *port)
	}

	svc := &ServiceManager{}

	if !guardSingleInstance() {
		os.Exit(0)
	}
	defer releaseGuard()

	if err := maybeRegisterShortcut(); err != nil {
		log.Printf("kvn-desktop: shortcut registration: %v", err)
	}

	if err := platformRun(svc, *port, url); err != nil {
		log.Fatalf("kvn-desktop: %v", err) //nolint:gocritic // log.Fatalf exit is acceptable here
	}
}
