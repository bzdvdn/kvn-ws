# Production Hardening — Summary

**Goal:** Production-ready VPN: reconnect, keepalive, kill-switch, rate limiting, session expiry, IP persistence, Prometheus metrics, health endpoint, config reload, audit trail, CLI flags. Gate: 24h stability.

| AC | Описание | Приоритет |
|----|----------|-----------|
| AC-001 | Auto-reconnect (exponential backoff + jitter) | P1 |
| AC-002 | Keepalive PING/PONG <30s | P1 |
| AC-003 | Kill-switch (route leakage) | P1 |
| AC-004 | Rate limiting (auth + packets) | P1 |
| AC-005 | Session expiry + reclaim | P1 |
| AC-006 | IP Pool persistence (BoltDB) | P1 |
| AC-007 | Prometheus /metrics | P1 |
| AC-008 | Health endpoint | P1 |
| AC-009 | Graceful config reload (SIGHUP) | P2 |
| AC-010 | Structured error + audit trail | P1 |
| AC-011 | CLI flags + env override | P1 |
| AC-012 | 24h stability gate | P0 |

**Out of Scope:** Горизонтальное масштабирование, распределённый IP pool, GUI/TUI, BGP, Windows/macOS kill-switch, dynamic DNS.

**MVP:** rate limiting + session expiry + health endpoint (AC-004, AC-005, AC-008).

**Новые зависимости:** `go.etcd.io/bbolt`, `github.com/prometheus/client_golang`.
