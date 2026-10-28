#!/usr/bin/env sh
# @sk-task kvn-web#T4.1: build-web.sh (AC-001)
set -e

cd "$(dirname "$0")/.."

echo "Building React frontend..."
cd src/internal/webui/frontend
npm install --silent
npm run build
cd ../../../../
echo "Building kvn-web binary..."
go build -o bin/kvn-web ./src/cmd/web

echo "Building kvn-desktop binary (CGo native)..."
if go build -o bin/kvn-desktop ./src/cmd/desktop 2>/dev/null; then
	echo "Done: bin/kvn-desktop"
	ls -lh bin/kvn-desktop
else
	echo ""
	echo "WARN: kvn-desktop build failed (CGo deps missing)."
	echo "  Linux:  apt install libgtk-3-dev libwebkit2gtk-4.1-dev + symlink webkit2gtk-4.0 -> 4.1"
	echo "  macOS:  CGo WKWebView — build on macOS"
	echo "  Windows: CGo WebView2 — build on Windows"
fi

echo ""
echo "Done: bin/kvn-web$(ls -lh bin/kvn-web 2>/dev/null)"
