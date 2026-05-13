#!/bin/sh
# @sk-task routing-split-tunnel#T4.1: gate test script (AC-010)
set -e

echo "========================================"
echo " Routing & Split Tunnel — Gate Test"
echo "========================================"

echo ""
echo "--- 1. Environment ---"
uname -a
echo "nftables: $(nft --version 2>/dev/null || echo 'not installed')"

echo ""
echo "--- 2. Routing engine unit tests ---"
cd /app && go test ./src/internal/routing/... -v -count=1 2>&1

echo ""
echo "--- 3. DNS resolver unit tests ---"
cd /app && go test ./src/internal/dns/... -v -count=1 2>&1

echo ""
echo "--- 4. NAT module unit tests ---"
cd /app && go test ./src/internal/nat/... -v -count=1 2>&1

echo ""
echo "--- 5. NFTables NAT integration ---"
if nft --version >/dev/null 2>&1; then
    echo "nftables available, testing NAT setup..."
    nft add table ip kvn-test
    nft add chain ip kvn-test postrouting '{ type nat hook postrouting priority srcnat; }'
    nft add rule ip kvn-test postrouting masquerade
    echo "  Ruleset:"
    nft list ruleset
    nft delete table ip kvn-test
    echo "  + NAT teardown: OK"
else
    echo "  SKIP: nftables not available in this environment"
fi

echo ""
echo "--- 6. Config files check ---"
echo "  client config: $(wc -l < /etc/kvn-ws/client.yaml) lines"
echo "  server config: $(wc -l < /etc/kvn-ws/server.yaml) lines"
head -5 /etc/kvn-ws/client.yaml
echo "  ..."

echo ""
echo "--- 7. Routing gate scenario ---"
echo "  Config: default_route=direct, include_ranges=[10.0.0.0/8,172.16.0.0/12], include_domains=[corp.example.com]"

cd /app && go run ./src/cmd/gatetest/ 2>&1

echo ""
echo "========================================"
echo " RESULT: ALL GATE TESTS PASSED"
echo "========================================"
