#!/usr/bin/env sh
# @sk-task kvn-web#T4.1: build-web.sh (AC-001)
set -e

cd "$(dirname "$0")/.."

echo "Building React frontend..."
cd src/internal/webui/frontend
npm install --silent
npm run build
cd ../../../../
generate_protocol() {
  # @sk-task kvn-android#T1.2: codegen before any Go build (AC-004)
  if [ -f scripts/generate-protocol.sh ]; then
    ./scripts/generate-protocol.sh
  fi
}
generate_protocol
echo "Building kvn-web binary..."
go build -o bin/kvn-web ./src/cmd/web

echo "Done: bin/kvn-web"
ls -lh bin/kvn-web
