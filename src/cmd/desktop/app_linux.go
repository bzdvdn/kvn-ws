//go:build linux

package main

import (
	"log"

	webview "github.com/webview/webview_go"
)

// @sk-task desktop-tray#T2.4: linux tray lifecycle integration (AC-001, AC-002, AC-003)
func platformRun(svc *ServiceManager, port int, serverURL string) error {
	if noTrayMode {
		return legacyRun(svc, port, serverURL)
	}

	tray := newPlatformTray()
	lt := tray.(*linuxTray)
	go lt.Run()

	showWindow := func() {
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
	}

	showWindow()

	if err := lt.initTray(); err != nil {
		log.Printf("kvn-desktop: tray init: %v", err)
		return nil
	}

	for {
		action := lt.WaitForAction()
		switch action {
		case TrayShow:
			showWindow()
		case TrayQuit:
			lt.Stop()
			lt.destroyTray()
			return nil
		}
	}
}

// @sk-task desktop-tray#T2.4: legacy path without tray (AC-007)
func legacyRun(svc *ServiceManager, port int, serverURL string) error {
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
