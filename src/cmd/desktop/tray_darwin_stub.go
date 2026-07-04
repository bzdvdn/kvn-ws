//go:build darwin && !cgo

package main

// @sk-task desktop-tray#T2.3: darwin cgo stub (AC-001, AC-002, AC-003)
func newPlatformTray() TrayManager {
	return newNoopTray()
}
