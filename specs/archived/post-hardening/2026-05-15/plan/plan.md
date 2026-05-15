# Post-Hardening — План

## Phase Contract

Inputs: spec.md, repository context (production-readiness-hardening changes).
Outputs: plan, data model (no-change stub).
Stop if: spec расплывчата — нет, 12 задач с конкретными AC.

## Цель

Закрыть технический долг, оставшийся после `production-readiness-hardening`. Все изменения — локальные патчи существующего кода. Никакого нового функционала.

## MVP Slice

P1-задачи: AC-001 (sm.Stop idempotent), AC-002 (per-session cancel), AC-003 (origin checker), AC-004 (безопасные ошибки auth). Каждая даёт независимую ценность и не зависит от других.

## First Validation Path

```bash
go test -race ./...
# Проверить что sm.Stop не паникует:
go test -run TestSessionManagerStopIdempotent ./src/internal/session/
# Проверить origin checker:
go test -run TestOriginCheckerURLPatterns ./src/internal/transport/websocket/
```

## Scope

- `src/internal/session/session.go` — sync.Once Stop, per-session cancel map
- `src/cmd/server/main.go` — plumb per-session cancel, `sessionProxyStreams` → proxy package
- `src/internal/transport/websocket/websocket.go` — origin checker fix, SetWriteLimit(1MB)
- `src/internal/protocol/auth/` — generic error messages
- `src/internal/proxy/` — worker pool, context-aware streams, extracted sessionProxyStreams
- `src/cmd/server/main.go` — `/metrics` rate limiter
- `src/internal/metrics/` — latency histograms (prometheus)
- `src/internal/session/bandwidth.go` — move Lock inside AllowN call
- `src/internal/logger/` — runtime level change via AtomicLevel
- `src/internal/config/` — parse log level for runtime update

## Implementation Surfaces

| Surface | Изменения | Статус |
|---------|-----------|--------|
| `src/internal/session/session.go` | sync.Once Stop, per-session cancel map | существующая |
| `src/cmd/server/main.go` | cancel plumbing, proxy import, `/metrics` rl | существующая |
| `src/internal/transport/websocket/websocket.go` | origin matcher, SetWriteLimit, config param | существующая |
| `src/internal/protocol/auth/auth.go` | общие error messages | существующая |
| `src/internal/proxy/` | worker pool, context, streams.go | **новая (streams.go)** |
| `src/internal/metrics/` | latency гистограммы | существующая |
| `src/internal/session/bandwidth.go` | Lock guard fix | существующая |
| `src/internal/logger/` | AtomicLevel support | существующая |
| `src/internal/config/server.go` | log level relicensing | существующая |

## Bootstrapping Surfaces

`src/internal/proxy/streams.go` — новый файл, куда переносится `sessionProxyStreams` из server/main.go. Существующий `proxy/` пакет уже имеет Manager, Stream, Listener — streams.go логически дополняет его.

## Влияние на архитектуру

- **sessionProxyStreams migration**: тип переезжает из `server/main.go` в `proxy/streams.go`. Импорт в main.go меняется с "рядом объявлен" на "из пакета proxy". Не breaking — старый тип удаляется.
- **Per-session cancel**: SessionManager хранит `map[string]context.CancelFunc`. При Remove/Create/expire — вызывает cancel. handleTunnel в main.go создаёт cancel-функцию и передаёт в WS-горутины.
- **Runtime log level**: zap.AtomicLevel заменяет статический уровень. SIGHUP handler обновляет уровень.
- **Остальное**: локальные изменения без архитектурного влияния.

## Acceptance Approach

| AC | Подход | Surfaces | Validation |
|----|--------|----------|------------|
| AC-001 | sync.Once в Stop | `session.go` | `TestSessionManagerStopIdempotent` |
| AC-002 | cancel map в SessionManager | `session.go`, `server/main.go` | `TestSessionExpiryCancelsWS` |
| AC-003 | glob/fnmatch вместо path.Match | `websocket.go` | `TestOriginCheckerURLPatterns` |
| AC-004 | "authentication failed" для всех ошибок | `auth/` | `TestAuthErrorMessageDoesNotLeakInfo` |
| AC-005 | SetWriteLimit(1MB) в WSConfig + Apply | `websocket.go` | `TestWSWriteLimit` |
| AC-006 | semaphore в proxy | `proxy/` | `TestProxyWorkerPool` |
| AC-007 | rate limiter middleware на /metrics | `server/main.go` | `TestMetricsRateLimiter` |
| AC-008 | Lock внутри AllowN | `bandwidth.go` | `TestBandwidthManagerRace` |
| AC-009 | SetReadDeadline в proxy goroutine | `proxy/`, `server/main.go` | `TestProxyGoroutineContextCancel` |
| AC-010 | prometheus HistogramVec | `metrics/` | `curl /metrics` содержит гистограмму |
| AC-011 | zap.AtomicLevel в Logger + SIGHUP | `logger/`, `server/main.go` | `TestLoggerRuntimeLevelChange` |
| AC-012 | streams.go в proxy пакете | `proxy/streams.go` | `TestSessionProxyStreams*` |

## Данные и контракты

- Data model не меняется.
- API не меняется (auth error response — internal, не публичный контракт).
- WSConfig добавляет `WriteLimit int` поле — совместимо (0 = unlimited).
- `data-model.md` — no-change stub.

## Стратегия реализации

### DEC-001 Per-session cancel через map в SessionManager

Why: самый простой способ — SessionManager хранит cancel-функции. WS goroutines создают cancel через context.WithCancel.
Tradeoff: нужно помнить о вызове cancel в expire/Remove. Если забыть — утечка cancel-функции (легко, но не фатально).
Affects: session.go, server/main.go
Validation: expire удаляет сессию → cancel вызван → WS горутина завершается.

### DEC-002 Origin checker: замена path.Match на strings.Contains / glob

Why: path.Match не обрабатывает `/` в URL. Простейшая замена — `strings.Contains(origin, pattern)` или `strings.HasSuffix` для wildcard.
Tradeoff: менее строгий matcher, но для origin checking достаточен. Более строгое решение — `golang.org/x/text/language/match` — overkill.
Affects: websocket.go
Validation: `*.example.com` совпадает с `https://sub.example.com/path`.

### DEC-003 Worker pool для proxy: channel-based semaphore

Why: `chan struct{}` — идиоматичный Go semaphore. Без внешних зависимостей.
Tradeoff: блокировка при заполненном пуле. Нужен таймаут.
Affects: proxy/
Validation: лимит 2, 3 запроса — 3-й ждёт или timeout.

## Incremental Delivery

### MVP (P1 — AC-001–AC-004)

- sync.Once Stop
- Per-session cancel
- Origin checker fix
- auth error messages

Каждая задача независима, можно параллелить.

### Итеративное расширение (P2 — AC-005–AC-007)

- WriteLimit
- Worker pool
- Metrics rate limiter

### P3 (AC-008–AC-012)

- Bandwidth race fix
- Context-aware proxy
- Latency histograms
- Runtime log level
- streams.go extraction

## Порядок реализации

1. **AC-001** sm.Stop sync.Once — 10 строк, эффект сразу.
2. **AC-003** origin checker — баг, потенциально security.
3. **AC-004** auth messages — 2 строки, эффект сразу.
4. **AC-002** per-session cancel — меняет архитектуру отмены, требует аккуратного теста.
5. **P2 + P3** — независимы, можно параллелить после P1.

## Риски

- **AC-002**: cancel-functions leak если expiry не вызывает cancel. Mitigation: тест `TestSessionExpiryCancelsWS`.
- **AC-006**: semaphore может заблокировать AcceptLoop. Mitigation: timeout на acquire.
- **AC-011**: zap.AtomicLevel requires реконфигурация логгера. Mitigation: интеграционный тест.

## Rollout и compatibility

- Auth error message change — может сломать клиенты, парсящие "invalid token". **Это intentional** — безопасность важнее.
- WSConfig.WriteLimit(0) = unlimited (compat).
- Все изменения обратно совместимы по API/конфигурации.

## Проверка

- `go test -race ./...` — все AC
- `go vet ./...` — code quality
- P1-specific: тесты на sm.Stop, origin checker, cancel, auth messages

## Соответствие конституции

Нет конфликтов.
