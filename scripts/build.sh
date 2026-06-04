#!/usr/bin/env sh
# @sk-task foundation#T2.1: build script for bin/ (AC-011)
# Usage: ./scripts/build.sh          # build both
#        ./scripts/build.sh client   # build client only
#        ./scripts/build.sh server   # build server only
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

case "$TARGET" in
  client) build_client ;;
  server) build_server ;;
  both)
    build_client
    build_server
    ;;
  *)
    echo "Usage: $0 [client|server|both]" >&2
    exit 1
    ;;
esac

echo "Done. Binaries in bin/:"
ls -la bin/ 2>/dev/null || true
