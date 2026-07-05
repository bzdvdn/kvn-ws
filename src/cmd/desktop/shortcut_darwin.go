//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func maybeRegisterShortcut() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	appDir := filepath.Join(home, "Applications", "KVN Desktop.app", "Contents", "MacOS")
	resDir := filepath.Join(appDir, "..", "Resources")
	plistPath := filepath.Join(appDir, "..", "Info.plist")
	bundlePath := filepath.Join(appDir, "kvn-desktop")

	if _, err := os.Stat(plistPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(appDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(resDir, 0755); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	if err := os.Symlink(exe, bundlePath); err != nil {
		return err
	}

	iconData, err := readIconBytes("kvn-desktop.png")
	if err != nil {
		return err
	}
	iconPath := filepath.Join(resDir, "kvn-desktop.png")
	if err := os.WriteFile(iconPath, iconData, 0644); err != nil {
		return err
	}

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>KVN Desktop</string>
	<key>CFBundleDisplayName</key>
	<string>KVN Desktop</string>
	<key>CFBundleIdentifier</key>
	<string>com.kvn.desktop</string>
	<key>CFBundleVersion</key>
	<string>1.0</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleExecutable</key>
	<string>kvn-desktop</string>
	<key>CFBundleIconFile</key>
	<string>kvn-desktop</string>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
`)

	return os.WriteFile(plistPath, []byte(plistContent), 0644)
}
