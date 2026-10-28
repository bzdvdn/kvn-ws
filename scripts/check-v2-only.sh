#!/usr/bin/env bash
# Guard against reintroducing the removed v1 runtime or legacy config surface.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0
for path in \
  src/internal/v2 \
  src/internal/bootstrap \
  src/internal/tunnel \
  src/internal/protocol \
  src/internal/session \
  src/internal/nat \
  src/internal/acl \
  src/internal/admin \
  src/internal/ratelimit \
  src/internal/metrics \
  src/internal/systemproxy \
  src/internal/transparent \
  src/internal/transport/websocket \
  src/internal/transport/quic \
  src/internal/transport/framing \
  src/cmd/v2client \
  src/cmd/v2server \
  protocol; do
  if [ -e "$path" ]; then
    echo "FAIL: legacy surface present: $path" >&2
    fail=1
  fi
done
if rg -n 'kvn-web-v2|KvnVpnService' src --glob '!**/CONTEXT.md' 2>/dev/null; then
  echo "FAIL: legacy v2 UI/identity references" >&2
  fail=1
fi
if [ "$fail" -ne 0 ]; then exit 1; fi
echo "v2-only guard: PASS"
