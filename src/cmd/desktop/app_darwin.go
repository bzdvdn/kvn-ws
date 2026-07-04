//go:build darwin

package main

import (
	"github.com/webview/webview_go"
)

// @sk-task desktop-tray#T2.4: darwin tray lifecycle integration (AC-001, AC-002, AC-003)
func platformRun(svc *ServiceManager, port int, serverURL string) error {
	return legacyDarwinRun(svc, port, serverURL)
}

// @sk-task desktop-tray#T2.4: legacy path without tray (AC-007)
func legacyDarwinRun(svc *ServiceManager, port int, serverURL string) error {
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("KVN Desktop")
	w.SetSize(900, 600, webview.HintNone)
	if !checkServer(serverURL) {
		showErrorPage(w, svc, serverURL)
	} else {
		w.Navigate(serverURL)
		injectRestartButton(w, svc)
	}
	w.Run()
	return nil
}
