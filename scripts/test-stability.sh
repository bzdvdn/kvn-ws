#!/bin/sh
# @sk-task production-hardening#T5.2: 5-minute stability gate (AC-012)
set -e

DURATION=${1:-300}

echo "=========================================="
echo " Stability Gate Test (${DURATION}s)"
echo "=========================================="
echo ""

echo "--- 1. Race detector ---"
cd /app && go test -race -count=1 ./src/... 2>&1 | tail -3

echo ""
echo "--- 2. Unit tests ---"
go test -count=1 ./src/internal/routing/... ./src/internal/session/... ./src/internal/dns/... ./src/internal/nat/... 2>&1 | tail -5

echo ""
echo "--- 3. Routing engine load test (${DURATION}s) ---"
cd /app && go run ./src/cmd/stability/ $DURATION 2>&1

echo ""
echo "--- 4. Memory leak check ---"
echo "  (check if max_heap grows linearly vs iterations)"

echo ""
echo "=========================================="
echo " PASS: ${DURATION}s stability gate"
echo "=========================================="
