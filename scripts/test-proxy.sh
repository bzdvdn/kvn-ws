#!/bin/sh
# @sk-task local-proxy-mode: e2e proxy test
set -e

echo "=== Proxy E2E Test ==="

# Start server
/server --config /etc/kvn-ws/server.yaml &
SERVER_PID=$!
sleep 1

# Start client in proxy mode
/client --config /etc/kvn-ws/client.yaml --mode proxy &
CLIENT_PID=$!
sleep 2

# Test via SOCKS5 proxy
echo "Testing SOCKS5 proxy..."
RESULT=$(curl --socks5 127.0.0.1:2310 -s -o /dev/null -w "%{http_code}" --connect-timeout 5 https://example.com 2>&1 || echo "FAIL")

echo "Result: $RESULT"

kill $CLIENT_PID 2>/dev/null || true
kill $SERVER_PID 2>/dev/null || true

if [ "$RESULT" = "200" ] || [ "$RESULT" = "301" ] || [ "$RESULT" = "302" ]; then
    echo "[PASS] Proxy test passed"
    exit 0
else
    echo "[FAIL] Proxy test failed: $RESULT"
    exit 1
fi
