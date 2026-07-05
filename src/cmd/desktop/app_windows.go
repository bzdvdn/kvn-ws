//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/webview/webview_go"

	"github.com/bzdvdn/kvn-ws/src/internal/webui"
)

func platformRun(svc *ServiceManager, port int, serverURL string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := webui.New(port)
	if err != nil {
		log.Printf("kvn-desktop: webui init: %v", err)
		return fmt.Errorf("webui init: %w", err)
	}

	go func() {
		if serveErr := srv.Serve(ctx); serveErr != nil {
			log.Printf("kvn-desktop: server stopped: %v", serveErr)
		}
	}()

	waitForServer(serverURL, 10*time.Second)

	SetServerRestart(func() error {
		cancel()
		time.Sleep(500 * time.Millisecond)

		newCtx, newCancel := context.WithCancel(context.Background())
		cancel = newCancel

		newSrv, newErr := webui.New(port)
		if newErr != nil {
			return newErr
		}
		srv = newSrv

		go func() {
			if serveErr := srv.Serve(newCtx); serveErr != nil {
				log.Printf("kvn-desktop: server restart: %v", serveErr)
			}
		}()

		waitForServer(serverURL, 5*time.Second)
		return nil
	})

	SetServerStop(func() error {
		cancel()
		return nil
	})

	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("KVN Desktop")
	w.SetSize(1280, 800, webview.HintNone)
	w.Navigate(serverURL)
	injectRestartButton(w, svc)
	w.Run()

	return nil
}

func waitForServer(url string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if checkServer(url) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
