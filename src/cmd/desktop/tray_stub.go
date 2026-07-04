//go:build !windows && !linux && !darwin

package main

// @sk-task desktop-tray#T2: platform tray fallback (AC-001, AC-002, AC-003)
func newPlatformTray() TrayManager {
	return newNoopTray()
}
