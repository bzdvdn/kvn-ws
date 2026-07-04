//go:build linux || darwin

package main

import (
	"os"
	"path/filepath"
)

// @sk-task desktop-tray#T3.1: unix .desktop shortcut registration (AC-004)
func maybeRegisterShortcut() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dir := filepath.Join(home, ".local", "share", "applications")
	path := filepath.Join(dir, "kvn-desktop.desktop")

	if _, err := os.Stat(path); err == nil {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	content := "[Desktop Entry]\nType=Application\nName=KVN Desktop\nExec=" + exe + "\nTerminal=false\nCategories=Network;\n"
	return os.WriteFile(path, []byte(content), 0o600)
}
