#!/bin/bash
# @sk-task docs-and-release#T1.3: run.sh generates TLS and starts compose (AC-005)
set -euo pipefail

# Generate self-signed TLS certificate
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout key.pem -out cert.pem \
  -subj "/CN=localhost" 2>/dev/null

echo "TLS certificate generated (cert.pem, key.pem)"

# Start services
docker compose up -d

echo "Waiting for client connection..."
sleep 3
docker compose logs client | grep "handshake complete" && \
  echo "SUCCESS: Client connected!" || \
  echo "Check logs: docker compose logs client"
