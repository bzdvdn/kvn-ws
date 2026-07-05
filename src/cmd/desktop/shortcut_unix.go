//go:build linux

package main

import (
	"os"
	"path/filepath"
)

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

	iconDir := filepath.Join(home, ".local", "share", "icons", "hicolor", "256x256", "apps")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return err
	}

	iconData, err := readIconBytes("kvn-desktop.png")
	if err != nil {
		return err
	}

	iconPath := filepath.Join(iconDir, "kvn-desktop.png")
	if err := os.WriteFile(iconPath, iconData, 0o600); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	content := "[Desktop Entry]\nType=Application\nName=KVN Desktop\nComment=KVN Web UI desktop wrapper\nExec=" + exe + "\nIcon=kvn-desktop\nTerminal=false\nCategories=Network;\n"
	return os.WriteFile(path, []byte(content), 0o600)
}
