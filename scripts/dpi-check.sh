#!/usr/bin/env bash
# Smoke-check the visible TLS surface of a v2 endpoint.
# It is not a full JA3/JA4 lab; it prints ALPN/cipher/SNI for one handshake.
set -euo pipefail

HOST="${1:?usage: dpi-check.sh host[:port] [sni]}"
SNI="${2:-${HOST%%:*}}"
PORT="${HOST##*:}"
[[ "$PORT" == "$HOST" ]] && PORT=443
HOST_ONLY="${HOST%%:*}"

echo "target: ${HOST_ONLY}:${PORT} sni=${SNI}"
if command -v openssl >/dev/null 2>&1; then
  printf 'GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n' "$SNI" | \
    openssl s_client -connect "${HOST_ONLY}:${PORT}" -servername "$SNI" -alpn h2 -brief 2>&1 | \
    grep -E 'Protocol|Ciphersuite|ALPN|Server certificate|Verification' || true
else
  echo "openssl not found" >&2
  exit 1
fi
