# Hot Path Performance — Задачи

## Phase Contract

Inputs: `plan.md`, `data-model.md`, repo-контекст.
Outputs: упорядоченные задачи с привязкой к AC и поверхностям.
Stop if: задачи не исполнимы как один связный кусок работы. Ок — каждая задача меняет 1-2 файла.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/transport/quic/conn.go` | T1.1, T2.1, T2.3 |
| `src/internal/transport/quic/obfuscated.go` | T1.2, T1.3, T2.2 |
| `src/internal/transport/websocket/websocket.go` | T1.4, T2.4, T3.2 |
| `src/internal/ratelimit/ratelimit.go` | T2.5 |
| `src/internal/dnsproxy/dnsproxy.go` | T3.1 |

## Implementation Context

- Цель MVP: sync.Pool для QUIC буферов + nonceInit atomic + fast PRNG для padding (AC-001–AC-004)
- Границы приемки: AC-001–AC-010, все CI-верифицируемы
- Ключевые правила:
  - sync.Pool: Get+Put с cap fallback (pattern из `framing/buffer_pool.go`)
  - nonceInit: `CompareAndSwap` (не Load+Store)
  - lock ordering DNS: всегда release before acquire
  - WS control writer: отдельный канал, не захватывает `wmu`
- Инварианты данных: ни один экспортированный тип/интерфейс/сигнатура не меняется
- Контракты/протокол: без изменений
- Proof signals: `go test -race -bench=. -benchmem` PASS для каждого затронутого пакета
- Вне scope: TUN/routing/NAT/ACL/session, публичное API, performance-тесты вне `go test`

## Фаза 1: MVP — Транспортные пулы + atomic

Цель: sync.Pool для QUIC ReadMessage и ObfuscatedWriteMessage xorBuf, nonceInit atomic.Bool, padding PRNG.

- [x] T1.1 QUICConn.ReadMessage: `make([]byte, msgLen)` → `sync.Pool` с cap=1500, fallback на `make` при cap < msgLen. Touches: `src/internal/transport/quic/conn.go`
- [x] T1.2 ObfuscatedQUICConn.WriteMessage: `make([]byte, len(data))` для xorBuf → `sync.Pool`. Touches: `src/internal/transport/quic/obfuscated.go`
- [x] T1.3 nonceInit: `bool` → `atomic.Bool` c `CompareAndSwap` (не Load+Store) в `initNonce()`. Touches: `src/internal/transport/quic/obfuscated.go`
- [x] T1.4 WSConn.WriteMessage padding: `crypto/rand.Read` → `math/rand/v2`. Touches: `src/internal/transport/websocket/websocket.go`

## Фаза 2: Lock removal + rate limiter + BatchWriter

Цель: QUIC mu removal, maxMessageSize atomic, BatchWriter pool, rate limiter lock-free.

- [x] T2.1 QUICConn: удалить `c.mu.Lock()` из `WriteMessage()`; добавить `deadlineMu sync.Mutex` для `SetReadDeadline`/`SetWriteDeadline`. Touches: `src/internal/transport/quic/conn.go`
- [x] T2.2 ObfuscatedQUICConn.WriteMessage: удалить `oc.mu.Lock()`, stream.Write без общего mu. Touches: `src/internal/transport/quic/obfuscated.go`
- [x] T2.3 maxMessageSize: `int` → `atomic.Int32` (fix existing data race). Touches: `src/internal/transport/quic/conn.go`
- [x] T2.4 BatchWriter.Flush: `make([]byte, bw.buf.Len())` → `sync.Pool`. Touches: `src/internal/transport/websocket/websocket.go`
- [x] T2.5 Rate limiter: `sync.Mutex` + `map` → `sync.Map` для `IPRateLimiter.Allow()` и `SessionPacketLimiter.Allow()`. Touches: `src/internal/ratelimit/ratelimit.go`

## Фаза 3: DNS split + WS control plane

Цель: DNS proxy split mutex, WS keepalive/pong off wmu.

- [x] T3.1 DNS proxy: разделить `Server.mu RWMutex` на `configMu RWMutex` (stream, routeDirect, tracker, upstreams, origResolves) + `pendingMu sync.Mutex` (pending map). Добавить комментарий с lock ordering. Touches: `src/internal/dnsproxy/dnsproxy.go`
- [x] T3.2 WS control plane: вынести keepalive ping и pong handler из `wmu` в отдельный control writer (горутина + буферизованный канал cap=8). Написать `TestWSControlPlane` с assertion: pong handler не вызывает `wmu.Lock()`, keepalive не блокируется при удержании wmu data path'ом. Touches: `src/internal/transport/websocket/websocket.go`

## Фаза 4: Проверка

Цель: доказать отсутствие регрессии и гонок во всех затронутых пакетах.

- [x] T4.1 Запустить `go test -race -bench=. -benchmem ./src/internal/transport/... ./src/internal/ratelimit/... ./src/internal/dnsproxy/... && go vet ./...` — все PASS. Touches: все затронутые файлы.
- [x] T4.2 Отметить все задачи в tasks.md как выполненные, убедиться в наличии `@sk-task` trace-маркеров во всех изменённых файлах. Touches: все затронутые файлы.

## Покрытие критериев приемки

- AC-001 → T1.1, T4.1
- AC-002 → T1.2, T4.1
- AC-003 → T1.3, T4.1
- AC-004 → T1.4, T4.1
- AC-005 → T2.4, T4.1
- AC-006 → T2.5, T4.1
- AC-007 → T2.1, T2.2, T4.1
- AC-008 → T3.1, T4.1
- AC-009 → T3.2, T4.1
- AC-010 → T4.1, T4.2

## Заметки

- Фаза 1 (Основа из шаблона) пропущена — bootstrapping не требуется.
- T1.1 и T1.2 можно параллелить (оба sync.Pool в quic).
- T2.1 и T2.2 лучше делать последовательно (оба про mu removal).
- T3.2 — единственная задача с новым тестом (`TestWSControlPlane`).
- Все изменения тестируются существующими тестами; новые тесты только для WS control plane.
