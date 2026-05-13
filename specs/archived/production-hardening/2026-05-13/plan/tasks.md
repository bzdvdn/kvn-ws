# Production Hardening — Задачи

## Phase Contract

Inputs: spec, plan, data-model, plan.digest, spec.digest
Outputs: tasks.md с фазами, Surface Map, покрытие AC

## Implementation Context

- **Цель MVP:** rate limiting + session expiry + health endpoint + structured audit (AC-004, AC-005, AC-008, AC-010)
- **Инварианты:**
  - Rate limiter: token bucket per IP, 5/min auth, 1000/s packets
  - Session expiry: idle=300s, TTL=24h, reclaim interval=10s
  - BoltDB: optional, in-memory fallback при повреждении
  - CLI > env > YAML precedence
  - SIGHUP меняет только лимиты и токены (не listen address)
  - Kill-switch: nftables reject, Linux-only
  - Precedence: CLI flag > env > YAML > default
- **Новые зависимости:** `go.etcd.io/bbolt`, `github.com/prometheus/client_golang`, `github.com/spf13/pflag`, `golang.org/x/time/rate`
- **Границы scope:** нет cluster-aware rate limiter, нет распределённого IP pool, нет macOS/Windows kill-switch
- **Proof signals:** `go test -race ./...`, `curl /health 200`, `curl /metrics` содержит kvn_ метрики, 429 после 5+ auth попыток, BoltDB load/save тест

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/session/session.go` | T1.1, T2.1, T3.1 |
| `src/internal/session/bolt.go` | T3.1 |
| `src/internal/config/config.go` | T1.1, T5.1 |
| `src/internal/config/server.go` | T4.3 |
| `src/internal/config/client.go` | T4.3 |
| `src/internal/logger/logger.go` | T2.3 |
| `src/internal/metrics/metrics.go` | T3.2 |
| `src/internal/protocol/control/control.go` | T4.1 |
| `src/internal/transport/websocket/websocket.go` | T4.1, T4.2 |
| `src/cmd/server/main.go` | T2.1, T2.2, T3.2, T5.1 |
| `src/cmd/client/main.go` | T4.2, T4.3 |
| `go.mod` | T1.1 |

## Фаза 1: Основа

Цель: добавить зависимости, подготовить конфиг и logger для новых возможностей.

- [x] T1.1 Добавить зависимости и расширить конфиг для production-hardening
  Touches: go.mod, src/internal/config/server.go, src/internal/config/client.go, src/internal/config/config.go
  Details:
  - `go get go.etcd.io/bbolt github.com/prometheus/client_golang github.com/spf13/pflag golang.org/x/time/rate`
  - server.yaml: секция `rate_limiting` (auth_burst, packets_per_sec), `session` (idle_timeout, session_ttl), `bolt_db_path`
  - client.yaml: `kill_switch` (bool), `reconnect` (min_backoff, max_backoff)
  - config.go: `AtomicConfig` — `atomic.Pointer` для server/client config

## Фаза 2: MVP

Цель: rate limiting + session expiry + health endpoint + structured audit.

- [x] T2.1 Session expiry — background goroutine с reclaim
- [x] T2.2 Rate limiter — token bucket middleware на /tunnel
- [x] T2.3 Health endpoint + structured audit
  Touches: src/cmd/server/main.go, src/internal/logger/logger.go
  Details:
  - HealthHandler: GET /health → `{"status":"ok"}` (200). Readiness: 503 если не готов
  - AuditLogger: метод `Audit(level, msg, fields)` с session_id, reason, remote_addr
  - Auth failure handler вызывает `audit.Warn("auth failed", ...)` с structured полями
  - AC-008 proof: `curl /health` → 200
  - AC-010 proof: auth failure → JSON лог с `"msg":"auth failed"` + reason

## Фаза 3: Persistence + Metrics

Цель: BoltDB для IP pool, Prometheus метрики.

- [x] T3.1 BoltDB persistence для IP pool
  Touches: src/internal/session/session.go, src/internal/session/bolt.go
  Details:
  - BoltStore: Open(path), Close(), SaveAllocations(map), LoadAllocations() map
  - Bucket "allocations": key=sessionID, value=IP string
  - IPPool.Save() / IPPool.Load(): если BoltStore настроен, сохраняет/восстанавливает allocated
  - Graceful fallback: если БД повреждена → warn + continue with empty pool
  - AC-006 proof: `TestPoolPersistence` — allocate → Save → Load → verify IP

- [x] T3.2 Prometheus /metrics endpoint
  Touches: src/internal/metrics/metrics.go, src/cmd/server/main.go
  Details:
  - prometheus.NewGauge: `kvn_active_sessions`
  - prometheus.NewCounterVec: `kvn_throughput_bytes_total` (labels: type={tx,rx})
  - prometheus.NewCounterVec: `kvn_errors_total` (labels: type={auth,rate_limit,session})
  - promhttp.HandlerFor на /metrics, register в server mux
  - Forwarding loops увеличивают counters (bytes tx/rx, errors)
  - AC-007 proof: `curl /metrics` → `kvn_active_sessions 1`

## Фаза 4: Client Resilience

Цель: keepalive, kill-switch, reconnect, CLI flags.

- [x] T4.1 Keepalive PING/PONG + control frame types
  Touches: src/internal/protocol/control/control.go, src/internal/transport/websocket/websocket.go
  Details:
  - Control frame types: PING=0x01, PONG=0x02 (константы в control.go)
  - WSConn.SetKeepalive(interval, timeout): gorilla/websocket SetPingHandler + SetPongHandler
  - Ping ticker: шлёт PING каждые N секунд при отсутствии трафика
  - Pong timeout: если ответа нет 30s → закрыть соединение
  - AC-002 proof: ping timeout → лог `"ping timeout 30s, closing connection"`

- [x] T4.2 Reconnect loop + kill-switch
  Touches: src/cmd/client/main.go
  Details:
  - Reconnect loop: exponential backoff 1s→2s→4s→...→30s max, jitter ±500ms
  - Бесконечный retry (админ убивает процесс для остановки)
  - Kill-switch: при disconnect → nftables reject rule для всего трафика; при reconnect → удалить правило
  - При успешном reconnect: восстановить forwarding loops
  - AC-001 proof: лог `"reconnect attempt 3 in 4s"` → `"connected"`
  - AC-003 proof: ping 8.8.8.8 не проходит при disconnected состоянии

- [x] T4.3 CLI flags + env override (pflag)
  Touches: src/cmd/server/main.go, src/cmd/client/main.go, src/internal/config/server.go, src/internal/config/client.go
  Details:
  - Замена stdlib flag на spf13/pflag
  - Server: `--config`, `--listen`, `--metrics-addr`
  - Client: `--config`, `--server`
  - viper.BindPFlag для каждого флага
  - Precedence: CLI flag > env var > YAML > default
  - AC-011 proof: `server --listen :9090` слушает на :9090; `KVN_SERVER_LISTEN=:9443 server --listen :9090` → 9090 (CLI wins)

## Фаза 5: Ops + Stability

Цель: SIGHUP reload, stability gate.

- [x] T5.1 Graceful config reload (SIGHUP)
  Touches: src/internal/config/config.go, src/cmd/server/main.go
  Details:
  - signal.Notify(sighupCh, syscall.SIGHUP)
  - handler: viper.ReadInConfig() → atomic.StorePointer(newCfg)
  - Rate limiter + auth читают cfg через atomic.LoadPointer
  - При ошибке в новом конфиге: log.Warn + continue with old cfg
  - AC-009 proof: изменить токен в YAML → kill -HUP → новый client с новым токеном подключается

- [x] T5.2 Stability gate — race detector + 5min load test
  Touches: src/cmd/stability/main.go, scripts/test-stability.sh, docker-compose.test.yml
  Details:
  - `go test -race ./...` — 0 races
  - `go test -count=1 ./...` — all pass
  - Проверка `pprof` на утечки после 1h нагрузки
  - Сценарий 24h стабильности в CI/CD
  - AC-012 proof: `uptime` 24h, RSS стабилен, `go test -race ./...` pass

## Покрытие критериев приемки

- AC-001 -> T4.2
- AC-002 -> T4.1
- AC-003 -> T4.2
- AC-004 -> T2.2
- AC-005 -> T2.1
- AC-006 -> T3.1
- AC-007 -> T3.2
- AC-008 -> T2.3
- AC-009 -> T5.1
- AC-010 -> T2.3
- AC-011 -> T4.3
- AC-012 -> T5.2
