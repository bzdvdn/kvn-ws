# Production Hardening — План

## Phase Contract

Inputs: spec production-hardening, inspect (pass), repo surfaces
Outputs: plan.md, plan.digest.md, data-model.md

## Цель

Production-ready VPN: reconnect, keepalive, kill-switch, rate limiting, session expiry, IP persistence, Prometheus, health, SIGHUP, audit, CLI flags. 24h stability gate.

## MVP Slice

Rate limiting + session expiry + health endpoint + structured audit. AC-004, AC-005, AC-008, AC-010.

## First Validation Path

1. `go test ./src/internal/session/...` — session expiry + reclaim
2. `curl -X POST https://server/tunnel -H "Authorization: Bearer wrong"` × 6 → 429
3. `curl http://server:443/health` → `{"status":"ok"}`
4. Лог содержит `{"msg":"auth failed","reason":"invalid token"}`

## Scope

- `src/internal/protocol/control/` — PING/PONG frame types
- `src/internal/session/` — expiry goroutine, BoltDB persistence
- `src/internal/metrics/` — Prometheus collectors
- `src/internal/config/` — CLI flags (pflag), SIGHUP reload
- `src/internal/transport/websocket/` — ping handler, reconnect wrapper
- `src/internal/logger/` — structured audit enhancements
- `src/cmd/server/main.go` — rate limiter middleware, /health, /metrics, SIGHUP
- `src/cmd/client/main.go` — reconnect loop, kill-switch

## Implementation Surfaces

- **control-ping** (`src/internal/protocol/control/`) — stub → PING/PONG frame constants + helpers
- **ws-transport** (`src/internal/transport/websocket/`) — add SetPingHandler, reconnect wrapper
- **client-main** (`src/cmd/client/main.go`) — reconnect loop wrapping Dial + forwarding
- **server-main** (`src/cmd/server/main.go`) — rate limiter middleware, /health, /metrics, SIGHUP handler
- **session-store** (`src/internal/session/`) — expiry goroutine in SessionManager, BoltDB store for IPPool
- **metrics** (`src/internal/metrics/`) — stub → Gauges (active sessions), Counters (bytes, errors)
- **config-cli** (`src/internal/config/`) — switch from stdlib flag to pflag, add Listen flag
- **config-reload** (`src/internal/config/config.go`) — SIGHUP → atomic config swap
- **logger** (`src/internal/logger/`) — add `Audit` method with session_id, reason, remote_addr fields

## Bootstrapping Surfaces

- `src/internal/session/bolt.go` — new file: BoltDB store
- Добавить зависимости: `go.etcd.io/bbolt`, `github.com/prometheus/client_golang`

## Влияние на архитектуру

- SessionManager расширяется: expiry goroutine + BoltDB persistence
- IPPool получает BoltDB backend (опционально, in-memory fallback)
- Server mux расширяется: rate limiter middleware + /health + /metrics эндпоинты
- Client main переходит на reconnect loop вместо одноразового Dial
- Config: использование pflag вместо stdlib flag для CLI; viper уже поддерживает env

## Acceptance Approach

- AC-001 (reconnect) -> client-main: exponential backoff loop + unit test с mock WS
- AC-002 (keepalive) -> ws-transport: gorilla/websocket SetPingHandler + SetPongHandler
- AC-003 (kill-switch) -> client-main: nftables reject на disconnect, remove на reconnect
- AC-004 (rate limiting) -> server-main: token bucket middleware, 429 response
- AC-005 (session expiry) -> session-store: goroutine ticker, CheckIdle/CheckTTL
- AC-006 (BoltDB) -> session-bolt: BoltDB store, Load/Save IPPool
- AC-007 (Prometheus) -> metrics: promhttp.Handler на /metrics
- AC-008 (health) -> server-main: GET /health handler
- AC-009 (SIGHUP) -> config-reload: signal.Notify + atomic.LoadPointer на cfg
- AC-010 (audit) -> logger: zap.Logger.With structured fields
- AC-011 (CLI) -> config-cli: pflag + viper.BindPFlag
- AC-012 (stability) -> AC-001–AC-011 infra, 24h прогон в staging

## Данные и контракты

См. `data-model.md`.

## Стратегия реализации

### DEC-001: reconnect — exponential backoff loop

- Why: spec RQ-001. Min 1s, max 30s, jitter ±500ms. Бесконечный loop (админ убивает процесс).
- Tradeoff: не делает graceful backoff при ручной остановке.
- Affects: client-main.
- Validation: AC-001 unit test с mock server.

### DEC-002: keepalive — gorilla/websocket native PING/PONG

- Why: gorilla/websocket уже поддерживает PING/PONG на уровне протокола. Не надо кастомных фреймов.
- Tradeoff: PING/PONG не видны в нашем протоколе (не логируются как Data фреймы).
- Affects: ws-transport, control-ping (константы).
- Validation: AC-002 — SetPingHandler вызывается; лог при таймауте.

### DEC-003: kill-switch — nftables reject rule

- Why: при disconnect добавляем nftables правило reject для трафика (кроме DNS до reconnect).
- Tradeoff: Linux-only (nftables). На macOS → pfctl (отложено).
- Affects: client-main.
- Validation: AC-003 — ping 8.8.8.8 не проходит при отключении.

### DEC-004: rate limiter — token bucket per IP

- Why: in-memory token bucket (rate.Limiter из x/time/rate). 5/min auth, 1000/s packets.
- Tradeoff: in-memory → после рестарта сбрасывается (приемлемо).
- Affects: server-main.
- Validation: AC-004 — 429 после 5+ попыток.

### DEC-005: session expiry — background goroutine

- Why: SessionManager запускает ticker, проверяет ConnectedAt + idle. Idle timeout = 300s, TTL = 24h.
- Tradeoff: ticker каждые 10s → не мгновенное освобождение (приемлемо).
- Affects: session-store.
- Validation: AC-005 — TestSessionExpiryReclaimsIP.

### DEC-006: BoltDB persistence — bolt store

- Why: BoltDB — embeded KV, zero deps (кроме bbolt). Save на allocate, Load на старте.
- Tradeoff: не cluster-aware, single file. При повреждении — graceful in-memory fallback.
- Affects: session-bolt.
- Validation: AC-006 — TestPoolPersistence.

### DEC-007: Prometheus metrics — promhttp.Handler

- Why: стандартный prometheus client_golang. `prometheus.NewCounterVec`, `prometheus.NewGauge`.
- Tradeoff: session_id label на total — убрать (high cardinality). Только type, reason.
- Affects: metrics, server-main.
- Validation: AC-007 — curl /metrics.

### DEC-008: health endpoint — /health handler

- Why: HTTP handler на server mux. Liveness: always 200. Readiness: pool+session ready.
- Tradeoff: single HTTP server на listen port (не отдельный порт). =0
- Affects: server-main.
- Validation: AC-008 — curl /health 200/503.

### DEC-009: SIGHUP reload — atomic config swap

- Why: signal.Notify(SIGHUP) → перечитываем YAML → cas-обновляем cfg pointer. Rate limiter и auth читают из cfg.
- Tradeoff: listen address не меняется (spec Assumptions). Только лимиты + токены.
- Affects: config-reload, server-main.
- Validation: AC-009 — изменить токен → kill -HUP → новый client подключается.

### DEC-010: structured audit — zap.With

- Why: zap.Logger уже используется. Добавить `Audit(level, msg, fields)` с session_id, reason, remote_addr.
- Tradeoff: дополнительное поле в каждой строке (минимально).
- Affects: logger, server-main (вызовы audit при auth failure).
- Validation: AC-010 — grep лога по `"msg":"auth failed"`.

### DEC-011: CLI/env — pflag + viper

- Why: pflag поддерживает --flag синтаксис. viper.BindPFlag даёт CLI > env > YAML.
- Tradeoff: замена stdlib flag на pflag (ломает существующий --config).
- Affects: config-cli, client-main, server-main.
- Validation: AC-011 — server --help; KVN_SERVER_LISTEN=:9443 server --listen :9090 → слушает :9090.

## Incremental Delivery

### MVP (Первая ценность)

Rate limiter + session expiry + health + audit. AC-004, AC-005, AC-008, AC-010.

### Итеративное расширение

**Iter 2 (Persistence + metrics):** BoltDB + Prometheus. AC-006, AC-007.

**Iter 3 (Client resilience):** reconnect + keepalive + kill-switch + CLI flags. AC-001, AC-002, AC-003, AC-011.

**Iter 4 (Ops):** SIGHUP reload + stability gate. AC-009, AC-012.

## Порядок реализации

1. Зависимости: `go get go.etcd.io/bbolt github.com/prometheus/client_golang github.com/spf13/pflag golang.org/x/time/rate`
2. **Session expiry** (AC-005) — добавляем goroutine в SessionManager
3. **Rate limiter** (AC-004) — token bucket middleware в server main
4. **Health endpoint** (AC-008) — /health handler
5. **Structured audit** (AC-010) — zap поля в auth failure
6. **BoltDB** (AC-006) — bolt store, load/save IPPool
7. **Prometheus** (AC-007) — /metrics handler, collectors
8. **Keepalive** (AC-002) — PING/PONG handler в WS conn
9. **Kill-switch** (AC-003) — nftables reject on client
10. **Reconnect** (AC-001) — backoff loop вокруг Dial
11. **CLI flags** (AC-011) — pflag + viper
12. **SIGHUP** (AC-009) — graceful reload
13. **Stability gate** (AC-012) — 24h прогон

Параллельные группы: {session, rate, health, audit}, {boltdb, prometheus}, {keepalive, kill-switch, reconnect}, {cli, sighup}.

## Риски

- **BoltDB файл повреждён** — in-memory fallback + warn log.
- **Rate limiter overhead** — <1ms target, token bucket O(1).
- **Kill-switch на macOS** — pfctl не реализован; Linux-only в этом спринте.
- **SIGHUP на Windows** — не поддерживается; контейнеры Linux.

## Rollout и compatibility

- Rate limiter: default threshold = 0 (disabled), явная настройка включает.
- BoltDB: опционально, без него in-memory pool (backward compat).
- Config: pflag заменяет stdlib flag — --config флаг сохраняется.
- SIGHUP: нейтрально для существующих сессий.

## Проверка

- `go test ./src/internal/session/...` — expiry + BoltDB
- `go test ./src/internal/transport/websocket/...` — keepalive
- `go test ./src/internal/config/...` — CLI flags
- `go test ./src/internal/metrics/...` — collectors
- `go test -race ./...` — race detector
- Docker Compose integration: rate limit + health через curl
- `pprof` heap после 1h нагрузки

## Соответствие конституции

- нет конфликтов: Go, Clean Architecture, traceability, Docker
- Новые зависимости (bbolt, prometheus, pflag, x/time) — pure Go, совместимы с multi-stage
