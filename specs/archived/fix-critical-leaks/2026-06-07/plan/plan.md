# Исправление критических утечек и ошибок — План

## Phase Contract

Inputs: spec (13 AC, 13 RQ), inspect (concerns — evidence precision), код (10 файлов).
Outputs: plan.md, data-model.md (no-change).
Stop if: нет — spec чёткая, inspect не blocked.

## Цель

Форма реализации: 10 локальных патчей в существующих файлах (ни одной новой сущности). Никаких изменений API, схемы БД, контрактов. Каждый патч независимо тестируем.

## MVP Slice

Исправление goroutine/context leaks (AC-001..AC-006) + BoltDB timeout (AC-007) + time.After (AC-008). Это 8 AC, закрывающих resource safety.

## First Validation Path

`go test -race ./... + runtime.NumGoroutine()` до/после reconnect-цикла.

## Scope

- Goroutine leak: TUN reader (`tunnel/session.go:85-103`), proxy listener (`proxy/listener.go:77-85`), RouteDirect (`bootstrap/client/proxy.go:210-254`)
- Context propagation: QUIC dial (`transport/quic/dial.go:14-27`), DNS in tun (`bootstrap/client/tun.go:226`), DNS refresh (`routing/domain_matcher.go:103`), WebUI broadcast (`webui/server.go:38-39`, `webui/state.go:106,140`)
- Deadlock risk: session manager (`session/session.go:281-379`)
- BoltDB timeout (`session/bolt.go:21,43`)
- time.After (`bootstrap/client/reconnect.go:23-28`)
- Swallowed errors (5 мест: server.go:335,356; admin.go:112,129; tunnel/session.go:202)
- Type assertion (`bootstrap/client/proxy.go:295`)
- Дубликаты: TLS config + backoff parsing (`proxy.go:30-53`, `tun.go:29-52`)
- sync.Pool в proxy stream (`tunnel/session.go:241,261`)

## Implementation Surfaces

| Файл | Почему | Тип |
|---|---|---|
| `tunnel/session.go` | goroutine leak в `tunReadInterruptible`; per-packet goroutine на proxy; per-loop buf/payload alloc | существующий |
| `proxy/listener.go` | unbounded goroutine в `AcceptLoop` | существующий |
| `bootstrap/client/proxy.go` | fire-and-forget + errc недоконсум + `net.Error` assertion + DNS context.Background() + dupl TLS/backoff | существующий |
| `bootstrap/client/tun.go` | DNS context.Background() + dupl TLS/backoff | существующий |
| `bootstrap/client/reconnect.go` | `time.After` leak | существующий |
| `transport/quic/dial.go` | context.Background() в `Dial` | существующий |
| `routing/domain_matcher.go` | context.Background() в DNS refresh | существующий |
| `webui/server.go` | broadcast с context.Background() | существующий |
| `webui/state.go` | broadcastLogs/broadcastStatus | существующий |
| `session/session.go` | deadlock risk: mu + cancelFuncs | существующий |
| `session/bolt.go` | bolt.Open без Timeout | существующий |
| `bootstrap/server/server.go` | _ = json.Encode (2 места) | существующий |
| `admin/admin.go` | _ = json.Encode (2 места) | существующий |
| `config/config.go` | глобальная `envPrefixForWarning` | существующий |
| `bootstrap/client/helpers.go` | **новый файл** — общие TLS/backoff helper | новый |

## Bootstrapping Surfaces

- `bootstrap/client/helpers.go` — выделение TLS config + backoff parsing из `proxy.go` и `tun.go`

## Влияние на архитектуру

- Локальное: ни одна граница пакета не меняется. Все изменения — inline патчи.
- `quic.Dial` сигнатура: `Dial(addr, tlsConf, quicConf)` → `Dial(ctx, addr, tlsConf, quicConf)`. Callers (2 места) обновляются.
- `SessionManager` lock model: `cancelFuncs` получает отдельный `sync.Mutex`.
- WebUI `Server.broadcastLogs/Status` принимают `ctx` из `Serve()` вместо `context.Background()`.

## Acceptance Approach

| AC | Подход | Surfaces | Evidence |
|---|---|---|---|
| AC-001 | Убрать `tunReadInterruptible` — заменить на постоянный reader + закрытие TUN device при ctx cancel | `tunnel/session.go` | `runtime.NumGoroutine` до/после 10 reconnect |
| AC-002 | Добавить `sem chan struct{}` в `Listener` с cap | `proxy/listener.go` | 2000 парал. соединений ≤ 1000 горутин |
| AC-003 | Заменить `go func() { ... }` в RouteDirect на `errgroup` / `sync.WaitGroup` | `bootstrap/client/proxy.go` | pprof после 5 соединений |
| AC-004 | `ctx` параметр в `Dial` | `transport/quic/dial.go` | cancel → err within 200ms |
| AC-005 | Передать `ctx` в DNS lookup | `bootstrap/client/tun.go`, `routing/domain_matcher.go` | timeout → `ctx.DeadlineExceeded` |
| AC-006 | Передать контекст `Serve()` в broadcast | `webui/server.go`, `webui/state.go` | shutdown → goroutines complete |
| AC-007 | `bbolt.DefaultOptions` c `Timeout: 1s` | `session/bolt.go` | lock-файл → timeout error |
| AC-008 | `time.NewTimer` + `defer timer.Stop()` | `bootstrap/client/reconnect.go` | `Timer.Stop() == true` |
| AC-009 | `logger.Error` на каждый `_ = json.Encode` | `server.go`, `admin.go` | grep = 0 `_ = json.NewEncoder` |
| AC-010 | Отдельный `sync.Mutex` для `cancelFuncs` | `session/session.go` | parallel call test, 3s timeout |
| AC-011 | `errors.As(err, &netErr)` | `bootstrap/client/proxy.go` | юнит-тест с wrapped error |
| AC-012 | Shared helpers | `bootstrap/client/helpers.go` | grep — дубликатов нет |
| AC-013 | `sync.Pool` для 4KB buf | `tunnel/session.go` | `-benchmem` снижение allocs |

## Данные и контракты

- `data-model.md`: **no-change** — ни одна таблица/структура данных не меняется.
- `quic.Dial` сигнатура меняется: добавляется `ctx context.Context` первым аргументом. Это ломающий change для callers (2 места: `proxy.go`, `tun.go`). Совместимость: compile-time break, легко чинится.

## Стратегия реализации

### DEC-001 Замена tunReadInterruptible на постоянный reader + close device
  Why: текущий паттерн создаёт горутину на каждый TUN read — при ctx cancel она течёт до возврата из tun.Read(). Решение: один reader-горутина + select на ctx.Done(), при cancel — закрыть TUN device, что прервёт Read().
  Tradeoff: закрытие device — необратимая операция, требует пересоздания device при reconnect. Но reconnect уже пересоздаёт device.
  Affects: `tunnel/session.go`
  Validation: `runtime.NumGoroutine()` стабилен после 10 reconnect

### DEC-002 Semaphore в proxy Listener
  Why: unbounded goroutine при AcceptLoop. Используем тот же паттерн `chan struct{}`, что уже есть в `session.go:proxySem`.
  Tradeoff: лишняя блокировка при достижении лимита — новые соединения ждут. Альтернатива: immediate reject (close). Выбрано блокирование для reliability.
  Affects: `proxy/listener.go`
  Validation: AC-002 тест с 2000 парал.

### DEC-003 errgroup для RouteDirect
  Why: две `io.Copy` горутины не отслеживаются; errc канал читается 1 из 2. errgroup даёт lifecycle tracking + propagation первой ошибки.
  Tradeoff: errgroup отменяет контекст при первой ошибке — вторая горутина завершится с context.Canceled. Это OK — обе горутины должны завершиться при любой ошибке.
  Affects: `bootstrap/client/proxy.go`
  Validation: AC-003 pprof

### DEC-004 Отдельный lock для cancelFuncs
  Why: `expireIdle/expireTTL/Remove` держат `sm.mu.Lock()` и вызывают `cancelSession()`. `SetCancel` тоже держит `sm.mu`. Если cancelFunc вызывает callback на SessionManager → deadlock. Решение: `cancelFuncsMu sync.Mutex` только для `cancelFuncs`.
  Tradeoff: два мутекса вместо одного — потенциально больше contention, но cancelFuncs доступ редкий (только create/expire/remove).
  Affects: `session/session.go`
  Validation: AC-010 parallel test

### DEC-005 sync.Pool для 4KB буферов
  Why: `make([]byte, 4096)` в `tunnel/session.go:241` на каждый proxy stream — GC pressure под нагрузкой. Используем `sync.Pool`.
  Tradeoff: буфер не обнуляется при возврате — безопасно, т.к. читаем ровно n байт. Для MTU 9000 нужен пул на 9000 или два пула.
  Affects: `tunnel/session.go`
  Validation: `-benchmem`

### DEC-006 Структура helpers.go
  Why: TLS config (12 строк) и backoff parsing (10 строк) дублируются в `proxy.go` и `tun.go`. Вынос в `bootstrap/client/helpers.go` с экспортируемыми функциями.
  Affects: `bootstrap/client/helpers.go` (new), `proxy.go`, `tun.go`
  Validation: grep по дубликатам = 0

## Incremental Delivery

### MVP (Первая ценность)

AC-001..AC-008 — resource safety. После MVP: тесты с race detector проходят, `NumGoroutine` стабилен.
Проверка: `go test -race ./... + scripts/test-gate.sh`

### Итеративное расширение

1. **MVP**: AC-001..AC-008 (goroutine/context leaks + BoltDB + time.After)
2. **Шаг 2**: AC-009, AC-011 (swallowed errors + type assertion) — 6 мест, простые замены
3. **Шаг 3**: AC-010 (deadlock risk) — требует теста с parallel calls
4. **Шаг 4**: AC-012 (duplicate code helpers) — рефакторинг без изменения поведения
5. **Шаг 5**: AC-013 (sync.Pool) — performance, можно отложить

## Порядок реализации

1. **AC-007** (BoltDB timeout) — safest, 1 строка, нет зависимостей
2. **AC-008** (time.After) — 5 строк, нет зависимостей
3. **AC-004** (QUIC ctx) — меняет сигнатуру `Dial`, нужно обновить callers сразу
4. **AC-005** (DNS ctx) — тривиально, вместе с AC-004 (обе context propagation)
5. **AC-006** (WebUI broadcast ctx) — изолировано
6. **AC-009** (swallowed errors) — grep + logger.Error, можно параллельно с AC-001..AC-003
7. **AC-011** (type assertion) — 1 строка, вместе с AC-009
8. **AC-001** (TUN reader) — самый intrusive, требует понимания tun device lifecycle
9. **AC-002** (proxy semaphore) — требует нового поля в Listener
10. **AC-003** (RouteDirect errgroup) — изолировано от AC-001/002
11. **AC-010** (lock ordering) — требует отдельного мьютекса + тест
12. **AC-012** (helpers.go) — рефакторинг в конце
13. **AC-013** (sync.Pool) — P2, может быть deferred

**Параллельно**: AC-007, AC-008, AC-009, AC-011 (безопасно, нет пересечений).
**Параллельно**: AC-004, AC-005, AC-006 (все context propagation, но разные файлы).

## Риски

- **Риск 1**: TUN device.Read() может быть blocking syscall без deadline — закрытие device единственный способ прервать. Mitigation: проверить что `tun.Device.Close()` действительно прерывает Read() на целевой платформе.
- **Риск 2**: `quic.Dial` ctx propagation — quic-go может не支持 cancellation во время TLS handshake. Mitigation: тест AC-004.
- **Риск 3**: lock splitting в SessionManager может пропустить race (cancelFunc читается вне нового lock). Mitigation: все обращения к cancelFuncs через отдельный мьютекс, code review.

## Rollout и compatibility

- Специальных rollout-действий не требуется. Все изменения — локальные патчи, не затрагивающие внешний API.
- Compatibility: `quic.Dial` сигнатура меняется — компилятор поймает все места.
- Feature flag: не нужен.

## Проверка

- `go test -race ./...` — baseline
- `golangci-lint run` — чистота кода
- `scripts/test-gate.sh` — integration gate
- AC-specific тесты (parallel calls, timeout, lock ordering, wrapped errors)
- Визуальная проверка: grep `_ = json.NewEncoder` = 0, grep `context.Background()` в DNS = 0
- `runtime.NumGoroutine` до/после reconnect — стабильность

## Соответствие конституции

- нет конфликтов. Все изменения следуют Go 1.25, не добавляют глобального состояния (наоборот — убирают `envPrefixForWarning` mutation), не нарушают Clean Architecture.
