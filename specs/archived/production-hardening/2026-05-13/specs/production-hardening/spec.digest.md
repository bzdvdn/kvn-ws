AC-001: auto-reconnect с exponential backoff + jitter
AC-002: keepalive PING/PONG, детекция обрыва <30s
AC-003: kill-switch — блокировка route leakage при падении
AC-004: rate limiting — auth attempts + packets per session
AC-005: session expiry — idle timeout + TTL, reclaim IP
AC-006: IP pool persistence — BoltDB, восстановление после рестарта
AC-007: Prometheus /metrics — active sessions, throughput, errors
AC-008: health endpoint — GET /health (liveness + readiness)
AC-009: graceful config reload — SIGHUP
AC-010: structured error handling + audit trail
AC-011: CLI flags + env override поверх YAML
AC-012: 24h stability gate — без падений и утечек памяти
