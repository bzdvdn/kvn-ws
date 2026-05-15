# Post-Hardening — Технический долг

## Scope Snapshot

- **In scope**: устранение 12 задач технического долга, выявленных в `production-readiness-hardening`, не блокирующих MVP, но обязательных перед v1.0-stable.
- **Out of scope**: новый функционал (crypto, connection migration, HTTP3), архитектурные изменения, полное тестовое покрытие.

## Цель

Разработчик получает код без open issues из production-readiness-hardening: безопасный `sm.Stop()`, корректное завершение WS-горутин при expiry, правильно работающий Origin checker, безопасные error messages, WebSocket с write limit, контролируемое количество горутин proxy, защищённый `/metrics`, точный bandwidth limiter, context-aware proxy goroutines, latency histograms, runtime log level change, вынесенный `sessionProxyStreams`.

## Основной сценарий

1. Команда последовательно закрывает 12 задач (4×P1, 3×P2, 5×P3).
2. Для каждой задачи — unit-тест и trace marker `@sk-task` / `@sk-test`.
3. После verify — git-ветка `feature/post-hardening`, review, merge.

## User Stories

- **P1** — Оператор: сервер не паникует при двойном вызове `sm.Stop()`.
- **P1** — Оператор: при idle expiry WS-горутины завершаются мгновенно.
- **P1** — Оператор: Origin checker корректно обрабатывает `*.example.com/path`.
- **P1** — Безопасность: ошибки auth не раскрывают внутреннее состояние.
- **P2** — Оператор: WebSocket write limit предотвращает OOM.
- **P2** — Оператор: proxy goroutines ограничены worker pool.
- **P2** — Безопасность: `/metrics` защищён rate limiter.
- **P3** — Разработчик: bandwidth limiter точен.
- **P3** — Разработчик: proxy goroutines context-aware.
- **P3** — Оператор: latency histograms в Prometheus.
- **P3** — Оператор: runtime log level change.
- **P3** — Разработчик: `sessionProxyStreams` переиспользуемый и тестируемый.

## MVP Slice

P1-задачи (1–4): sync.Once для sm.Stop, per-session cancel, origin checker, безопасные ошибки. Эти задачи закрывают наиболее вероятные production инциденты.

## First Deployable Outcome

После P1: `sm.Stop()` не паникует, session expiry чистит горутины, Origin checker работает корректно, ошибки auth безопасны. `go test -race ./...` проходит.

## Scope

- `src/internal/session/session.go` — sync.Once Stop, per-session cancel
- `src/internal/transport/websocket/websocket.go` — origin checker fix, SetWriteLimit
- `src/internal/transport/websocket/` — data/control path refs
- `src/internal/protocol/auth/` — error messages
- `src/cmd/server/main.go` — per-session cancel plumbing, `/metrics` rate limit
- `src/internal/proxy/` — worker pool, context-aware goroutines, sessionProxyStreams extraction
- `src/internal/metrics/` — latency histograms
- `src/internal/session/bandwidth.go` — TokenBandwidthManager race fix
- `src/internal/logger/` — runtime level change
- `src/internal/config/` — runtime level config

## Контекст

- Все задачи — чистый технический долг, без изменения API/spec.
- Origin checker баг — потенциальная security уязвимость (ложный reject валидных origins).
- sm.Stop без sync.Once — panic при двойном вызове (сейчас deferred — однократный, но хрупкий).
- Session expiry без cancel — WS читатель висит до 30s (deadline).

## Требования

- RQ-001 `sm.Stop()` ДОЛЖЕН быть идемпотентным (sync.Once).
- RQ-002 При session expiry/remove ДОЛЖНА вызываться per-session cancel.
- RQ-003 Origin checker ДОЛЖЕН корректно обрабатывать URL вида `*.example.com/path`.
- RQ-004 Auth error response НЕ ДОЛЖЕН включать "invalid token", "max sessions exceeded" — только общее "authentication failed".
- RQ-005 WebSocket ДОЛЖЕН иметь `SetWriteLimit` не более 1MB.
- RQ-006 Количество proxy goroutines ДОЛЖНО быть ограничено (semaphore/worker pool).
- RQ-007 `/metrics` endpoint ДОЛЖЕН иметь rate limiter.
- RQ-008 `TokenBandwidthManager.Allow` ДОЛЖЕН быть точен при конкурентном доступе.
- RQ-009 TCP Read в proxy goroutines ДОЛЖЕН быть context-aware (SetReadDeadline).
- RQ-010 Prometheus метрики ДОЛЖНЫ включать latency гистограммы.
- RQ-011 Уровень логов ДОЛЖЕН меняться через SIGHUP.
- RQ-012 `sessionProxyStreams` ДОЛЖЕН быть вынесен в `src/internal/proxy/streams.go`.

## Вне scope

- Connection migration / resume session.
- QUIC/HTTP3.
- Полное тестовое покрытие config/proxy (отложено).
- E2E-шифрование (crypto).

## Критерии приемки

### AC-001 sm.Stop идемпотентен

- Почему это важно: двойной close канала — panic.
- **Given** SessionManager.Stop() вызван
- **When** Stop() вызывается повторно
- **Then** второй вызов — no-op
- **Evidence**: `TestSessionManagerStopIdempotent` в session_test.go

### AC-002 Session expiry отменяет WS горутины

- Почему это важно: без отмены горутины висят до deadline (30s).
- **Given** сессия с активными WS горутинами
- **When** сессия истекает по idle/TTL
- **Then** per-session context отменяется, WS Read/Write возвращают ctx.Err()
- **Evidence**: `TestSessionExpiryCancelsWS` в session_test.go или integration test

### AC-003 Origin checker корректно обрабатывает URL

- Почему это важно: `path.Match("*", "https://example.com/path")` возвращает false.
- **Given** Origin whitelist = `*.example.com`
- **When** Origin = `https://sub.example.com/path`
- **Then** Match возвращает true
- **Evidence**: `TestOriginCheckerURLPatterns` в websocket_test.go

### AC-004 Auth error — общее сообщение

- Почему это важно: атакующий не должен различать "invalid token" и "max sessions".
- **Given** сервер с auth
- **When** клиент шлёт невалидный токен
- **Then** response = `{"error":"authentication failed"}`
- **Evidence**: `TestAuthErrorMessageDoesNotLeakInfo` в auth_test.go

### AC-005 WebSocket WriteLimit

- Почему это важно: без лимита медленный читатель вызывает OOM.
- **Given** WSConn с SetWriteLimit(1MB)
- **When** буфер превышает лимит
- **Then** WriteMessage блокируется/возвращает ошибку
- **Evidence**: `TestWSWriteLimit` в websocket_test.go

### AC-006 Worker pool для proxy

- Почему это важно: 1000 сессий × 100 потоков = 100k горутин.
- **Given** конфигурация с maxProxyConcurrency
- **When** количество одновременных proxy streams превышает лимит
- **Then** новые соединения ожидают в очереди
- **Evidence**: `TestProxyWorkerPool` в proxy_test.go

### AC-007 Rate limiter для /metrics

- Почему это важно: Prometheus endpoint — вектор DoS.
- **Given** `/metrics` endpoint
- **When** запросов > N в секунду
- **Then** возвращается 429
- **Evidence**: `TestMetricsRateLimiter` в server/main_test.go или integration

### AC-008 TokenBandwidthManager точен

- Почему это важно: race condition между Unlock и AllowN снижает точность.
- **Given** TokenBandwidthManager с лимитом 1000 bps
- **When** 10 горутин одновременно вызывают Allow
- **Then** суммарный пропущенный трафик ≤ 1000 bps + epsilon
- **Evidence**: `TestBandwidthManagerRace` в bandwidth_test.go

### AC-009 Proxy goroutine context-aware

- Почему это важно: TCP Read блокируется навсегда без контекста.
- **Given** proxy goroutine с context
- **When** контекст отменён
- **Then** TCP Read возвращает ошибку deadline/timeout
- **Evidence**: `TestProxyGoroutineContextCancel` в proxy_test.go

### AC-010 Prometheus latency histograms

- Почему это важно: без гистограмм нельзя алертить по задержкам.
- **Given** сервер с метриками
- **When** трафик проходит через туннель
- **Then** `/metrics` содержит гистограммы tunnel_latency_{p50,p95,p99}
- **Evidence**: интеграционный тест / проверка `/metrics` вывода

### AC-011 Runtime log level change

- Почему это важно: debug-логи включаются без перезапуска.
- **Given** SIGHUP сигнал
- **When** новый конфиг содержит другой log level
- **Then** zap logger переключает уровень
- **Evidence**: `TestLoggerRuntimeLevelChange` в logger_test.go

### AC-012 sessionProxyStreams в отдельном пакете

- Почему это важно: тип в server/main.go не тестируем.
- **Given** `sessionProxyStreams`
- **When** вынесен в `src/internal/proxy/streams.go`
- **Then** пакет proxy имеет тесты для всех методов
- **Evidence**: `TestSessionProxyStreams*` в proxy_test.go

## Допущения

- Все изменения обратно совместимы.
- P3-задачи могут быть отложены до следующего спринта без блокировки v1.0-beta.
- sm.Stop() вызывается только из main() — sync.Once защита на будущее.

## Критерии успеха

- SC-001 `go test -race ./...` проходит.
- SC-002 Ни один из существующих тестов не сломан.
- SC-003 Все P1-задачи закрыты в первом passes.

## Краевые случаи

- sm.Stop() вызван до sm.Start() — sync.Once сработает, stopCh закрыт, reclaimLoop не запущен.
- Origin checker с пустым whitelist — поведение не меняется.
- SetWriteLimit(0) — unlimited (backward compat).
- Worker pool exhaustion — соединение ждёт в очереди, не дропается (если таймаут — дроп).

## Открытые вопросы

- ~~Какое значение SetWriteLimit по умолчанию?~~ **1MB**
- ~~Max proxy concurrency по умолчанию?~~ **1000 на сервер, 100 на клиента**
- ~~Какой rate limit для /metrics?~~ **100 req/s на IP**
