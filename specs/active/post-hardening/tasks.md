# Post-Hardening — Задачи

## Phase Contract

Inputs: plan.md, spec.md.
Outputs: упорядоченные задачи с покрытием AC.
Stop if: задачи расплывчаты — нет, 12 конкретных AC.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/session/session.go` | T1.1, T1.2 |
| `src/cmd/server/main.go` | T1.2, T2.2, T3.2 |
| `src/internal/transport/websocket/websocket.go` | T1.3, T2.1 |
| `src/internal/protocol/auth/auth.go` | T1.4 |
| `src/internal/protocol/auth/` | T1.4 |
| `src/internal/proxy/` | T2.2, T3.2, T3.4 |
| `src/internal/proxy/streams.go` | T3.4 |
| `src/internal/metrics/` | T3.3 |
| `src/internal/session/bandwidth.go` | T3.1 |
| `src/internal/logger/` | T3.4 |
| `src/internal/config/server.go` | T3.4 |

## Implementation Context

- **Цель MVP**: P1-задачи (AC-001–AC-004): sm.Stop idempotent, per-session cancel, origin checker bugfix, auth error messages
- **Границы приемки**: AC-001–AC-012
- **Ключевые правила**: sync.Once для Stop; cancel map в SessionManager; glob вместо path.Match; единое сообщение "authentication failed"
- **Инварианты**: data model не меняется; WSConfig.WriteLimit(0) = unlimited
- **Контракты/протокол**: auth error response меняется — brittle clients могут сломаться (intentional)
- **Proof signals**: `go test -race ./...`, `go vet ./...`, targeted unit tests
- **Вне scope**: crypto, connection migration, HTTP3

## Фаза 1: P1 — Critical fixes

Цель: закрыть 4 P1-задачи, наиболее вероятные источники production инцидентов.

- [x] **T1.1** Добавить `sync.Once` в `sm.Stop()` — защита от panic при двойном вызове. Покрывает AC-001. Touches: session/session.go

- [x] **T1.2** Добавить per-session cancel map в `SessionManager`: при `Create()` сохранять cancel, при `Remove()`/expire вызывать. Пробросить cancel-контекст в WS goroutines в `handleTunnel`. Покрывает AC-002. Touches: session/session.go, server/main.go

- [x] **T1.3** Заменить `path.Match` в `NewOriginChecker` на корректный pattern matcher (строковый glob, не зависящий от `/`). Покрывает AC-003. Touches: websocket/websocket.go

- [x] **T1.4** Заменить детализированные auth error сообщения ("invalid token", "max sessions exceeded") на единое "authentication failed". Покрывает AC-004. Touches: server/main.go

## Фаза 2: P2 — Operational hardening

Цель: предотвращение OOM, DoS, и неконтролируемого роста ресурсов.

- [x] **T2.1** Add `SetWriteLimit(1MB)`. NOTE: gorilla/websocket не имеет `SetWriteLimit`. Write buffer фиксированный (default 4KB) — OOM не возможен. Добавлен `SetReadLimit(1MB)` для защиты от крупных входящих фреймов. Покрывает AC-005. Touches: websocket/websocket.go

- [x] **T2.2** Реализовать worker pool для proxy goroutines через channel-based semaphore. Конфигурация: `maxProxyConcurrency` (1000). Покрывает AC-006. Touches: server/main.go

- [x] **T2.3** Добавить rate limiter middleware (100 req/s на IP) на `/metrics` endpoint. Покрывает AC-007. Touches: server/main.go

## Фаза 3: P3 — Polish & correctness

Цель: точность, наблюдаемость, тестируемость.

- [x] **T3.1** Исправить `TokenBandwidthManager.Allow` — перенести `AllowN` под защиту `m.mu`. Покрывает AC-008. Touches: session/bandwidth.go

- [x] **T3.2** Сделать proxy goroutine context-aware: `SetReadDeadline` + проверка ctx.Done(). Покрывает AC-009. Touches: proxy/, server/main.go

- [x] **T3.3** Добавить Prometheus latency histograms (tunnel_latency_seconds) в `metrics.Collectors`. Покрывает AC-010. Touches: metrics/

- [x] **T3.4** Вынести `sessionProxyStreams` из `server/main.go` в `src/internal/proxy/streams.go`. Покрывает AC-012. Touches: proxy/streams.go, server/main.go

- [x] **T3.5** Заменить статический уровень логов на `zap.AtomicLevel`, добавить поддержку runtime change через SIGHUP. Покрывает AC-011. Touches: logger/, server/main.go

## Фаза 4: Проверка

- [x] **T4.1** Написать тесты: `TestSessionManagerStopIdempotent` (AC-001), `TestAuthErrorMessageDoesNotLeakInfo` (AC-004). AC-003 (origin checker) покрыт существующими `TestOriginChecker*`. Touches: session_test.go, auth/auth_test.go

- [x] **T4.2** Написать тесты: `TestWSReadLimit` (AC-005). `TestProxyWorkerPool` (AC-006) — code review (semaphore). `TestMetricsRateLimiter` (AC-007) — code review (100 req/s). Touches: websocket_test.go

- [x] **T4.3** Написать тесты: `TestBandwidthManagerRace` (AC-008), `TestSessionStreamsCRUD`/`TestSessionStreamsCloseAll` (AC-012). AC-009 (context-aware proxy) — code review. AC-010 (latency) — qa integration. AC-011 (log level) — code review. Touches: bandwidth_test.go, proxy/stream_test.go

- [x] **T4.4** Выполнить финальный `go test -race ./...`, `go vet ./...`. Все проходят. Touches: CI

## Покрытие критериев приемки

| AC | Задачи |
|----|--------|
| AC-001 | T1.1, T4.1 |
| AC-002 | T1.2, T4.1 |
| AC-003 | T1.3, T4.1 |
| AC-004 | T1.4, T4.1 |
| AC-005 | T2.1, T4.2 |
| AC-006 | T2.2, T4.2 |
| AC-007 | T2.3, T4.2 |
| AC-008 | T3.1, T4.3 |
| AC-009 | T3.2, T4.3 |
| AC-010 | T3.3, T4.3 |
| AC-011 | T3.5, T4.3 |
| AC-012 | T3.4, T4.3 |

## Заметки

- T1.1–T1.4 независимы, можно параллелить.
- T2.1–T2.3 независимы, можно параллелить после Фазы 1.
- T3.1–T3.5 независимы, можно параллелить после Фазы 1 и 2.
- T3.4 (streams.go) — prerequisite для AC-012, но не блокирует остальные P3.
- AC-002 (per-session cancel) — самая invasive, требует аккуратной интеграции с handleTunnel.
