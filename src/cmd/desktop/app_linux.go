//go:build linux

package main

import (
	webview "github.com/webview/webview_go"
)

func platformRun(svc *ServiceManager, port int, serverURL string) error {
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("KVN Desktop")
	w.SetSize(1280, 800, webview.HintNone)
	injectRestartButton(w, svc)
	if !checkServer(serverURL) {
		showErrorPage(w, svc, serverURL)
	} else {
		w.Navigate(serverURL)
	}
	w.Run()
	return nil
}
