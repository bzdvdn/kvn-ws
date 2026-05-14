#!/bin/bash
# @sk-task docs-and-release#T1.3: run.sh generates TLS and starts compose (AC-005)
# @sk-task production-gap#T2.3: generate runtime-only root demo cert material (AC-003)
set -euo pipefail

mkdir -p certs

# Generate CA and server certificate for local runtime only.
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout certs/ca-key.pem -out certs/ca.pem \
  -subj "/CN=kvn-root-demo-ca" 2>/dev/null

openssl req -nodes -newkey rsa:2048 \
  -keyout certs/server-key.pem -out certs/server.csr \
  -subj "/CN=server" \
  -addext "subjectAltName=DNS:server,DNS:localhost,IP:127.0.0.1" 2>/dev/null

openssl x509 -req -days 365 \
  -in certs/server.csr \
  -CA certs/ca.pem \
  -CAkey certs/ca-key.pem \
  -CAcreateserial \
  -out certs/server.pem \
  -copy_extensions copyall 2>/dev/null

rm -f certs/server.csr certs/ca-key.pem certs/ca.srl

echo "TLS certificate generated in ./certs (ca.pem, server.pem, server-key.pem)"

# Start services
docker compose up -d

echo "Waiting for client connection..."
sleep 3
docker compose logs client | grep "handshake complete" && \
  echo "SUCCESS: Client connected!" || \
  echo "Check logs: docker compose logs client"
