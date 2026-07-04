package main

import (
	"embed"
	"io/fs"
)

// @sk-task desktop-tray#T1.2: embedded tray icons (AC-001, AC-002, AC-003)
//
//go:embed icons/*
var iconsFS embed.FS

// @sk-task desktop-tray#T1.2: embedded tray icons (AC-001, AC-002, AC-003)
func readIcon(name string) ([]byte, error) {
	return fs.ReadFile(iconsFS, "icons/"+name)
}
