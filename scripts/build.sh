#!/usr/bin/env sh
# @sk-task foundation#T2.1: build script for bin/ (AC-011)
set -e

echo "Building client..."
go build -o bin/client ./src/cmd/client

echo "Building server..."
go build -o bin/server ./src/cmd/server

echo "Done. Binaries in bin/:"
ls -la bin/
