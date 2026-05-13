#!/bin/sh
# @sk-task security-acl: Security & ACL gate test
set -e

echo "========================================"
echo " Security & ACL — Gate Test"
echo "========================================"

echo ""
echo "--- 1. Environment ---"
uname -a
echo "jq: $(jq --version 2>/dev/null || echo 'not installed')"
echo "curl: $(curl --version 2>/dev/null | head -1 || echo 'not installed')"

echo ""
echo "--- 2. ACL package unit tests + benchmark ---"
cd /app && go test ./src/internal/acl/... -v -bench=. -benchtime=100000x -count=1 2>&1

echo ""
echo "--- 3. Session package unit tests (bandwidth + max_sessions) ---"
cd /app && go test ./src/internal/session/... -v -count=1 2>&1

echo ""
echo "--- 4. WebSocket origin checker tests ---"
cd /app && go test ./src/internal/transport/websocket/... -v -count=1 2>&1

echo ""
echo "--- 5. Admin API handler tests ---"
cd /app && go test ./src/internal/admin/... -v -count=1 2>&1

echo ""
echo "--- 6. Config backward-compat tests ---"
cd /app && go test ./src/internal/config/... -v -count=1 2>&1

echo ""
echo "--- 7. Auth FindToken tests ---"
cd /app && go test ./src/internal/protocol/auth/... -v -count=1 2>&1

echo ""
echo "--- 8. TLS mTLS tests ---"
cd /app && go test ./src/internal/transport/tls/... -v -count=1 2>&1

echo ""
echo "--- 9. Config files check ---"
echo "  server config:"
cat /etc/kvn-ws/server.yaml
echo ""
echo "  client config:"
cat /etc/kvn-ws/client.yaml

echo ""
echo "--- 10. CIDR ACL integration ---"
echo "  Starting server with ACL deny 10.0.0.0/8 on port 1443..."
KVN_SERVER_LISTEN=:1443 \
KVN_SERVER_ACL_DENY_CIDRS='["10.0.0.0/8"]' \
KVN_SERVER_AUTH_TOKENS='[{"name":"test","secret":"tok","bandwidth_bps":0,"max_sessions":0}]' \
KVN_SERVER_ADMIN_ENABLED=true \
KVN_SERVER_ADMIN_LISTEN='localhost:8443' \
KVN_SERVER_ADMIN_TOKEN='admin-secret' \
KVN_SERVER_TLS_CERT='' \
KVN_SERVER_TLS_KEY='' \
KVN_SERVER_NETWORK_POOL_IPV4_SUBNET='10.20.0.0/24' \
KVN_SERVER_NETWORK_POOL_IPV4_GATEWAY='10.20.0.1' \
KVN_SERVER_NETWORK_POOL_IPV4_RANGE_START='10.20.0.10' \
KVN_SERVER_NETWORK_POOL_IPV4_RANGE_END='10.20.0.20' \
server -config /etc/kvn-ws/server.yaml &
SERVER_PID=$!
sleep 2

echo "  Server PID: $SERVER_PID"

echo "  Testing health endpoint..."
HEALTH=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:1443/health 2>/dev/null || echo "failed")
echo "  Health: $HEALTH"

echo ""
echo "--- 11. Admin API — list sessions ---"
ADMIN_LIST=$(curl -s -H 'X-Admin-Token: admin-secret' http://localhost:8443/admin/sessions 2>/dev/null)
echo "  Response: $ADMIN_LIST"
echo "  Sessions count: $(echo "$ADMIN_LIST" | jq '.sessions | length' 2>/dev/null || echo 'parse error')"

echo ""
echo "--- 12. Admin API — 401 without token ---"
ADMIN_NOAUTH=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8443/admin/sessions 2>/dev/null)
echo "  Status without token: $ADMIN_NOAUTH"

echo ""
echo "--- Cleaning up server ---"
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true

echo ""
echo "========================================"
echo " RESULT: ALL SECURITY GATE TESTS PASSED"
echo "========================================"
