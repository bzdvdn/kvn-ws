#!/usr/bin/env bash
# Reproducible v2 surface check: TLS/ALPN, browser-facing paths, cover site,
# active probe and fallback. Network/region are recorded from DPI_* env vars.
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

fail_check() { echo "FAIL $*"; fail=1; }
ok_check() { echo "ok $*"; }

echo "target: ${HOST_ONLY}:${PORT} sni=${SNI}"
echo "network: ${NETWORK} region: ${REGION}"

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl not found" >&2
  exit 1
fi

echo "[tls]"
tls_out="$(openssl s_client -connect "${HOST_ONLY}:${PORT}" -servername "$SNI" -alpn h2 -brief </dev/null 2>&1 | tr -d '\0' || true)"
printf '%s\n' "$tls_out" | grep -E 'Protocol|Ciphersuite|ALPN|Server certificate|Verification' || true
if printf '%s' "$tls_out" | grep -q 'Verification error'; then
  if [[ "$INSECURE" == "1" ]]; then
    echo "warn: certificate verification failed (DPI_INSECURE=1)"
  else
    fail_check "tls: certificate verification failed"
  fi
fi

cert_info="$(openssl s_client -connect "${HOST_ONLY}:${PORT}" -servername "$SNI" -showcerts </dev/null 2>/dev/null | \
  openssl x509 -noout -issuer -subject 2>/dev/null | tr -d '\0' || true)"
issuer="$(printf '%s\n' "$cert_info" | sed -n 's/^issuer=//p' | head -1)"
subject="$(printf '%s\n' "$cert_info" | sed -n 's/^subject=//p' | head -1)"
if [[ -n "$issuer" && "$issuer" == "$subject" ]]; then
  if [[ "$INSECURE" == "1" ]]; then
    echo "warn: self-signed certificate (DPI_INSECURE=1)"
  else
    fail_check "tls: self-signed certificate"
  fi
else
  ok_check "tls: certificate issuer=${issuer:-unknown}"
fi

h1_out="$(printf 'GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n' "$SNI" | \
  openssl s_client -connect "${HOST_ONLY}:${PORT}" -servername "$SNI" -alpn http/1.1 2>&1 || true)"
if printf '%s' "$h1_out" | grep -q 'ALPN protocol: http/1.1'; then
  ok_check "alpn: http/1.1"
else
  fail_check "alpn: http/1.1 not negotiated (h2-only server?)"
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl not found; path probes skipped" >&2
  exit "$fail"
fi

probe_path() {
  local path="$1"
  local headers status ctype
  headers="$(curl -sS "${CURL_TLS[@]}" --http2 --max-time 8 -o /dev/null -D - "${BASE}${path}" 2>/dev/null || true)"
  status="$(printf '%s' "$headers" | awk 'toupper($1) ~ /^HTTP\// {code=$2} END {print code}')"
  ctype="$(printf '%s' "$headers" | awk 'tolower($1)=="content-type:" {print $2}' | tr -d '\r' | head -1)"
  if [[ -z "$status" ]]; then
    fail_check "${path}: no HTTP response"
    return
  fi
  if printf '%s' "$headers" | grep -qiE 'x-kvn|kvn-stream|kvn-target|pad='; then
    fail_check "${path}: KVN-specific header/marker"
    return
  fi
  if [[ "$ctype" == application/octet-stream* ]]; then
    fail_check "${path}: stream content-type on probe"
    return
  fi
  ok_check "${path}: status=${status} content-type=${ctype:-?}"
}

echo "[paths]"
for path in / /robots.txt /favicon.ico /login.html /api/health /api/v2/stream; do
  probe_path "$path"
done

echo "[cover-site]"
random_path="/no-such-page-${RANDOM}"
headers="$(curl -sS "${CURL_TLS[@]}" --http2 --max-time 8 -o /dev/null -D - "${BASE}${random_path}" 2>/dev/null || true)"
status="$(printf '%s' "$headers" | awk 'toupper($1) ~ /^HTTP\// {code=$2} END {print code}')"
if [[ "$status" == "404" ]]; then
  ok_check "catch-all: ${random_path} -> 404"
else
  fail_check "catch-all: ${random_path} -> ${status:-none} (want 404)"
fi
ctype="$(printf '%s' "$headers" | awk 'tolower($1)=="content-type:" {print $2}' | tr -d '\r' | head -1)"
if [[ "$ctype" == text/html* ]]; then
  ok_check "404 content-type: $ctype"
else
  fail_check "404 content-type: ${ctype:-none} (want text/html)"
fi

headers="$(curl -sS "${CURL_TLS[@]}" --http2 --max-time 8 -o /dev/null -D - "${BASE}/favicon.ico" 2>/dev/null || true)"
status="$(printf '%s' "$headers" | awk 'toupper($1) ~ /^HTTP\// {code=$2} END {print code}')"
ctype="$(printf '%s' "$headers" | awk 'tolower($1)=="content-type:" {print $2}' | tr -d '\r' | head -1)"
if [[ "$status" == "200" && "$ctype" != text/html* ]]; then
  ok_check "favicon: status=${status} content-type=${ctype:-?}"
else
  fail_check "favicon: status=${status:-none} content-type=${ctype:-none}"
fi

login_body="$(curl -sS "${CURL_TLS[@]}" --http2 --max-time 8 "${BASE}/login.html" 2>/dev/null || true)"
if printf '%s' "$login_body" | grep -qi '<form'; then
  ok_check "login page: form present"
else
  fail_check "login page: no form"
fi

echo "[active-probe]"
probe_headers="$(curl -sS "${CURL_TLS[@]}" --http2 --max-time 8 -o /dev/null -D - \
  -X POST "${BASE}${STREAM_PATH}" \
  -H 'Authorization: Bearer invalid-probe-token' \
  -H 'Content-Type: application/octet-stream' \
  --data-binary 'probe' 2>/dev/null || true)"
probe_status="$(printf '%s' "$probe_headers" | awk 'toupper($1) ~ /^HTTP\// {code=$2} END {print code}')"
probe_type="$(printf '%s' "$probe_headers" | awk 'tolower($1)=="content-type:" {print $2}' | tr -d '\r' | head -1)"
if [[ -z "$probe_status" ]]; then
  fail_check "active probe: no HTTP response"
elif printf '%s' "$probe_headers" | grep -qiE 'x-kvn|kvn-stream|kvn-target|pad='; then
  fail_check "active probe: KVN-specific header/marker"
elif [[ "$probe_type" == application/octet-stream* ]]; then
  fail_check "active probe: stream accepted invalid token"
else
  ok_check "active probe: status=${probe_status} content-type=${probe_type:-?}"
fi

if [[ "$fail" -ne 0 ]]; then
  echo "RESULT: FAIL"
  exit 1
fi
echo "RESULT: PASS"
