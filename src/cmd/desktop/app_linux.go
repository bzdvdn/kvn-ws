//go:build linux

package main

import (
	webview "github.com/webview/webview_go"
)

// @sk-task kvn-desktop#T2.1: linux webview to localhost (AC-001)
func platformRun(svc *ServiceManager, port int, serverURL string) error {
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
