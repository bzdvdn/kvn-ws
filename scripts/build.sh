#!/usr/bin/env sh
# @sk-task foundation#T2.1: build script for bin/ (AC-011)
# @sk-task web-ci-install-autostart#T2.1: add web target with npm build
# Usage: ./scripts/build.sh          # build both
#        ./scripts/build.sh client   # build client only
#        ./scripts/build.sh server   # build server only
#        ./scripts/build.sh web      # build web only
set -e

TARGET="${1:-both}"

generate_protocol() {
  # @sk-task kvn-android#T1.2: codegen before any Go build (AC-004)
  if [ -f scripts/generate-protocol.sh ]; then
    ./scripts/generate-protocol.sh
  fi
}

build_client() {
  generate_protocol
  echo "Building client..."
  go build -ldflags="-s -w" -o bin/client ./src/cmd/client
}

build_server() {
  generate_protocol
  echo "Building server..."
  go build -ldflags="-s -w" -o bin/server ./src/cmd/server
}

build_web() {
  generate_protocol
  echo "Building frontend..."
  (cd src/internal/webui/frontend && npm ci && npm run build)
  echo "Building web..."
  go build -ldflags="-s -w" -o bin/kvn-web ./src/cmd/web
}

build_relay() {
  generate_protocol
  echo "Building relay..."
  go build -ldflags="-s -w" -o bin/relay ./src/cmd/relay
}

case "$TARGET" in
  client) build_client ;;
  server) build_server ;;
  web) build_web ;;
  relay) build_relay ;;
  both)
    build_client
    build_server
    build_web
    build_relay
    ;;
  *)
    echo "Usage: $0 [client|server|web|relay|both]" >&2
    exit 1
    ;;
esac

echo "Done. Binaries in bin/:"
ls -la bin/ 2>/dev/null || true
