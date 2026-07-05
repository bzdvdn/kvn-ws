package main

import (
	"embed"
	"io/fs"
)

//go:embed icons/kvn-desktop.png icons/kvn-desktop.ico
var iconFS embed.FS

func readIconBytes(name string) ([]byte, error) {
	return fs.ReadFile(iconFS, "icons/"+name)
}
