#!/usr/bin/env sh
# Build v2 release artifacts.
# Usage: ./scripts/build.sh [client|server|web|desktop|both]
set -e

TARGET="${1:-both}"

build_client() {
  echo "Building v2 client..."
  go build -ldflags="-s -w" -o bin/kvn-client ./src/cmd/client
}

build_server() {
  echo "Building v2 server..."
  go build -ldflags="-s -w" -o bin/kvn-server ./src/cmd/server
}

build_web() {
  echo "Building frontend..."
  (cd src/internal/webui/frontend && npm ci && npm run build)
  echo "Building v2 web..."
  go build -ldflags="-s -w" -o bin/kvn-web ./src/cmd/web
}

webkit_tags() {
  if ! command -v pkg-config >/dev/null 2>&1; then
    return 0
  fi
  if ! pkg-config --exists webkit2gtk-4.1 2>/dev/null && pkg-config --exists webkit2gtk-4.0 2>/dev/null; then
    echo "-tags webkit2_40"
  fi
}

build_desktop() {
  echo "Building v2 desktop..."
  # shellcheck disable=SC2046
  go build $(webkit_tags) -ldflags="-s -w" -o bin/kvn-desktop ./src/cmd/desktop
}

case "$TARGET" in
  client) build_client ;;
  server) build_server ;;
  web) build_web ;;
  desktop) build_desktop ;;
  both)
    build_client
    build_server
    build_web
    build_desktop
    ;;
  *)
    echo "Usage: $0 [client|server|web|desktop|both]" >&2
    exit 1
    ;;
esac

echo "Done. Binaries in bin/:"
ls -la bin/ 2>/dev/null || true
