#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")/../.."

# Generate TLS certs in examples/certs if not present
if [ ! -f examples/certs/server.pem ]; then
  mkdir -p examples/certs
  openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout examples/certs/ca-key.pem -out examples/certs/ca.pem \
    -subj "/CN=kvn-relay-ca" 2>/dev/null

  openssl req -nodes -newkey rsa:2048 \
    -keyout examples/certs/server-key.pem -out examples/certs/server.csr \
    -subj "/CN=server" \
    -addext "subjectAltName=DNS:server,DNS:relay,DNS:localhost,IP:127.0.0.1" 2>/dev/null

  openssl x509 -req -days 365 \
    -in examples/certs/server.csr \
    -CA examples/certs/ca.pem \
    -CAkey examples/certs/ca-key.pem \
    -CAcreateserial \
    -out examples/certs/server.pem \
    -copy_extensions copyall 2>/dev/null

  rm -f examples/certs/server.csr examples/certs/ca-key.pem examples/certs/ca.srl
  echo "TLS certs generated in examples/certs/"
fi

cd examples/relay
docker compose up -d

echo "Waiting for relay and client..."
sleep 4
docker compose logs client | grep -q "handshake complete" && \
  echo "SUCCESS: client -> relay -> server tunnel established" || \
  echo "Check logs: docker compose logs client"
docker compose down -v