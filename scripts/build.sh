#!/usr/bin/env sh
# @sk-task foundation#T2.1: build script for bin/ (AC-011)
# @sk-task web-ci-install-autostart#T2.1: add web target with npm build
# Usage: ./scripts/build.sh          # build both
#        ./scripts/build.sh client   # build client only
#        ./scripts/build.sh server   # build server only
#        ./scripts/build.sh web      # build web only
set -e

TARGET="${1:-both}"

build_client() {
  echo "Building client..."
  go build -ldflags="-s -w" -o bin/client ./src/cmd/client
}

build_server() {
  echo "Building server..."
  go build -ldflags="-s -w" -o bin/server ./src/cmd/server
}

build_web() {
  echo "Building frontend..."
  (cd src/internal/webui/frontend && npm ci && npm run build)
  echo "Building web..."
  go build -ldflags="-s -w" -o bin/kvn-web ./src/cmd/web
}

case "$TARGET" in
  client) build_client ;;
  server) build_server ;;
  web) build_web ;;
  both)
    build_client
    build_server
    build_web
    ;;
  *)
    echo "Usage: $0 [client|server|web|both]" >&2
    exit 1
    ;;
esac

echo "Done. Binaries in bin/:"
ls -la bin/ 2>/dev/null || true
