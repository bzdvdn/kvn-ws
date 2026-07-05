//go:build darwin

package main

import (
	"github.com/webview/webview_go"
)

func platformRun(svc *ServiceManager, port int, serverURL string) error {
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("KVN Desktop")
	w.SetSize(1280, 800, webview.HintNone)
	if !checkServer(serverURL) {
		showErrorPage(w, svc, serverURL)
	} else {
		w.Navigate(serverURL)
		injectRestartButton(w, svc)
	}
	w.Run()
	return nil
}
