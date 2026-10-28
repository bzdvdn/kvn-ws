#!/usr/bin/env bash
# Reproducible v2 surface check: TLS/ALPN, browser-facing paths, active probe
# and fallback. Network/region are recorded from DPI_NETWORK/DPI_REGION.
set -uo pipefail

HOST="${1:?usage: dpi-check.sh host[:port] [sni]}"
SNI="${2:-${HOST%%:*}}"
PORT="${HOST##*:}"
[[ "$PORT" == "$HOST" ]] && PORT=443
HOST_ONLY="${HOST%%:*}"
BASE="https://${HOST}"
STREAM_PATH="${KVN_STREAM_PATH:-/}"
NETWORK="${DPI_NETWORK:-lab}"
REGION="${DPI_REGION:-unspecified}"
INSECURE="${DPI_INSECURE:-0}"
CURL_TLS=()
[[ "$INSECURE" == "1" ]] && CURL_TLS=(-k)
fail=0

echo "target: ${HOST_ONLY}:${PORT} sni=${SNI}"
echo "network: ${NETWORK} region: ${REGION}"

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl not found" >&2
  exit 1
fi

echo "[tls]"
tls_out="$(printf 'GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n' "$SNI" | \
  openssl s_client -connect "${HOST_ONLY}:${PORT}" -servername "$SNI" -alpn h2 -brief 2>&1 || true)"
printf '%s\n' "$tls_out" | grep -E 'Protocol|Ciphersuite|ALPN|Server certificate|Verification' || true
if printf '%s' "$tls_out" | grep -q 'Verification error'; then
  if [[ "$INSECURE" == "1" ]]; then
    echo "warn: certificate verification failed (DPI_INSECURE=1)"
  else
    echo "FAIL tls: certificate verification failed"
    fail=1
  fi
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl not found; path probes skipped" >&2
  exit "$fail"
fi

echo "[paths]"
for path in / /robots.txt /favicon.ico /api/health /api/v2/stream; do
  headers="$(curl -sS "${CURL_TLS[@]}" --http2 --max-time 8 -o /dev/null -D - "${BASE}${path}" 2>/dev/null || true)"
  status="$(printf '%s' "$headers" | awk 'toupper($1) ~ /^HTTP\// {code=$2} END {print code}')"
  ctype="$(printf '%s' "$headers" | awk 'tolower($1)=="content-type:" {print $2}' | tr -d '\r' | head -1)"
  if [[ -z "$status" ]]; then
    echo "FAIL ${path}: no HTTP response"
    fail=1
  elif printf '%s' "$headers" | grep -qiE 'x-kvn|kvn-stream|kvn-target'; then
    echo "FAIL ${path}: KVN-specific header"
    fail=1
  else
    echo "ok ${path}: status=${status} content-type=${ctype:-?}"
  fi
done

echo "[active-probe]"
probe_headers="$(curl -sS "${CURL_TLS[@]}" --http2 --max-time 8 -o /dev/null -D - \
  -X POST "${BASE}${STREAM_PATH}" \
  -H 'Authorization: Bearer invalid-probe-token' \
  -H 'Content-Type: application/octet-stream' \
  --data-binary 'probe' 2>/dev/null || true)"
probe_status="$(printf '%s' "$probe_headers" | awk 'toupper($1) ~ /^HTTP\// {code=$2} END {print code}')"
probe_type="$(printf '%s' "$probe_headers" | awk 'tolower($1)=="content-type:" {print $2}' | tr -d '\r' | head -1)"
if [[ -z "$probe_status" ]]; then
  echo "FAIL active probe: no HTTP response"
  fail=1
elif printf '%s' "$probe_headers" | grep -qiE 'x-kvn|kvn-stream|kvn-target'; then
  echo "FAIL active probe: KVN-specific header"
  fail=1
elif [[ "$probe_type" == application/octet-stream ]]; then
  echo "FAIL active probe: stream accepted invalid token"
  fail=1
else
  echo "ok active probe: status=${probe_status} content-type=${probe_type:-?}"
fi

if [[ "$fail" -ne 0 ]]; then
  echo "RESULT: FAIL"
  exit 1
fi
echo "RESULT: PASS"
