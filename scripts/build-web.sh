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

echo "Done: bin/kvn-web"
ls -lh bin/kvn-web
