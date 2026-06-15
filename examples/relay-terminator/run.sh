#!/bin/bash
# @sk-task relay-terminator#T4.1: terminator docker-compose smoke-test (AC-001, AC-002, AC-004)
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

cd examples/relay-terminator
docker compose up -d

echo "Waiting for terminator relay and clients..."
sleep 8

ws_ok=false
quic_ok=false
upstream_ok=false
session_ok=false

if docker compose logs client 2>/dev/null | grep -q "handshake complete"; then
  echo "SUCCESS: WS client -> relay terminator tunnel established"
  ws_ok=true
else
  echo "WARN: WS client tunnel not established (check logs: docker compose logs client)"
fi

if docker compose logs quic-client 2>/dev/null | grep -q "handshake complete"; then
  echo "SUCCESS: QUIC client -> relay terminator tunnel established"
  quic_ok=true
else
  echo "WARN: QUIC client tunnel not established (check logs: docker compose logs quic-client)"
fi

if docker compose logs relay 2>/dev/null | grep -q "upstream handshake OK"; then
  echo "SUCCESS: relay -> upstream server tunnel established"
  upstream_ok=true
else
  echo "WARN: relay upstream tunnel not established"
fi

if docker compose logs relay 2>/dev/null | grep -q "terminator session created"; then
  echo "SUCCESS: relay accepted client connections"
  session_ok=true
else
  echo "WARN: no client sessions on relay"
fi

if [ "$ws_ok" = false ] && [ "$quic_ok" = false ]; then
  echo "FAIL: no clients connected"
  docker compose logs
  docker compose down -v
  exit 1
fi

echo "=== Summary ==="
echo "WS client handshake: $ws_ok"
echo "QUIC client handshake:$quic_ok"
echo "Upstream tunnel:     $upstream_ok"
echo "Client sessions:     $session_ok"
echo ""
echo "Note: routing evidence (route=direct/upstream) requires"
echo "active traffic (ping). Check relay logs after sending packets."
docker compose down -v
