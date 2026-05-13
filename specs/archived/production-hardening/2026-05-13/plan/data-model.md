# Production Hardening — Модель данных

## Scope

- Связанные `AC-*`: AC-001–AC-012
- Связанные `DEC-*`: DEC-001–DEC-011
- Статус: `changed`
- SessionManager расширяется (expiry, BoltDB), IPPool получает BoltDB backend, новые сущности: RateLimiter, HealthStatus, MetricsCollectors, AtomicConfig

## Сущности

### DM-001: SessionExpiryConfig

- Назначение: конфигурация таймаутов сессий.
- Источник истины: server.yaml.
- Поля:
  - `idle_timeout` — `time.Duration`, default 300s.
  - `session_ttl` — `time.Duration`, default 24h.
  - `reclaim_interval` — `time.Duration`, default 10s.
- Связанные `AC-*`: AC-005.
- Связанные `DEC-*`: DEC-005.

### DM-002: BoltStore

- Назначение: BoltDB persistence для IPPool allocated state.
- Источник истины: файл БД (path из конфига, default `/var/lib/kvn-ws/ip-pool.db`).
- Инварианты:
  - Bucket: `allocations`, key = sessionID → value = IP string.
  - Load на старте сервера восстанавливает map.
  - Save на каждом Allocate/Release.
- Связанные `AC-*`: AC-006.
- Связанные `DEC-*`: DEC-006.
- Жизненный цикл:
  - Open при старте сервера.
  - Update при allocate/release.
  - Close при shutdown.
  - Повреждённый файл → in-memory fallback + warn.

### DM-003: RateLimiter

- Назначение: token bucket per IP (auth) + per session (packets).
- Источник истины: in-memory (не cluster-aware).
- Поля:
  - `authLimiter` — `rate.Limiter`, 5/1min per remote IP.
  - `packetLimiter` — `rate.Limiter`, 1000/1s per session.
- Конфигурация: `rate_limiting.auth_burst`, `rate_limiting.packets_per_sec` из server.yaml.
- Связанные `AC-*`: AC-004.
- Связанные `DEC-*`: DEC-004.

### DM-004: MetricsCollectors

- Назначение: Prometheus-совместимые метрики.
- Источник истины: in-memory регистрация при старте сервера.
- Метрики:
  - `kvn_active_sessions` — Gauge.
  - `kvn_throughput_bytes_total` — CounterVec (type={tx,rx}).
  - `kvn_errors_total` — CounterVec (type={auth,rate_limit,session}).
- Связанные `AC-*`: AC-007.
- Связанные `DEC-*`: DEC-007.

### DM-005: HealthStatus

- Назначение: liveness/readiness state сервера.
- Источник истины: runtime state.
- Liveness: всегда OK (200).
- Readiness: OK когда pool initialized + sessions ready (503 если нет).
- Связанные `AC-*`: AC-008.
- Связанные `DEC-*`: DEC-008.

### DM-006: AtomicConfig

- Назначение: thread-safe доступ к конфигу для SIGHUP reload.
- Реализация: `atomic.Pointer[ServerConfig]` / `atomic.Pointer[ClientConfig]`.
- Критическая секция: rate limiter thresholds, auth tokens читаются через Load().
- Связанные `AC-*`: AC-009.
- Связанные `DEC-*`: DEC-009.

### DM-007: ReconnectState

- Назначение: состояние reconnect loop.
- Поля:
  - `attempt` — int, current backoff attempt.
  - `delay` — time.Duration, current delay (1s→2s→4s→...→30s).
  - `stopCh` — chan struct{}.
- Связанные `AC-*`: AC-001.
- Связанные `DEC-*`: DEC-001.

## Связи

- `DM-002 BoltStore → session.IPPool`: BoltStore.Load восстанавливает allocated map, Store.Save сохраняет.
- `DM-003 RateLimiter → server-main`: middleware проверяет лимит до upgrade.
- `DM-004 Metrics → server-main`: forwarding loops увеличивают counters.
- `DM-005 HealthStatus → server-main`: /health handler читает статус.
- `DM-006 AtomicConfig → server-main`: SIGHUP handler обновляет, rate/auth читают.

## Производные правила

- Precedence: CLI flag > env var > YAML > default (DEC-011).
- Rate limiter auth: burst=5, затем 1/min. Сброс через 1 минуту бездействия.
- Kill-switch: default route через TUN → при disconnect удаляем default route → трафик не уходит.

## Переходы состояний

- Session: Created → (idle > timeout или TTL expired) → Removed → IP reclaimed.
- Reconnect: Connected → Disconnected → backoff sleep → Dial → Connected.

## Вне scope

- Cluster-aware rate limiter (Redis)
- Distributed IP pool (несколько серверов)
- Persistence для логов (только stdout)
- Health check с deep dependency probe (БД, upstream)
