# Hot Path Performance — План

## Phase Contract

Inputs: `spec.md`, `inspect.md`, repo-контекст (транспортные пакеты).
Outputs: `plan.md`, `data-model.md`.
Stop if: spec не определяет проверяемые evidence для каждого AC. Ок — все AC имеют CI-верифицируемый evidence.

## Цель

9 изолированных оптимизаций в 4 кластерах: транспортные пулы и atomic (MVP), lock removal + rate limiter, DNS proxy split, WS control plane offload. Каждый кластер независимо тестируем, без изменения публичного API и без регрессии.

## MVP Slice

**Транспортные пулы + atomic** (T1.1–T1.4): sync.Pool для QUIC read/write буферов, nonceInit atomic.Bool, math/rand/v2 для padding.

Покрывает: AC-001, AC-002, AC-003, AC-004, AC-010.

## First Validation Path

```bash
go test -race -bench=. -benchmem ./src/internal/transport/quic/... ./src/internal/transport/websocket/... && go vet ./...
```

PASS + benchmem без регрессии по allocs/op — MVP готов.

## Scope

- `src/internal/transport/quic/conn.go` — sync.Pool для ReadMessage, deadlineMu, atomic.Int32 для maxMessageSize
- `src/internal/transport/quic/obfuscated.go` — nonceInit atomic.Bool (CAS), xorBuf sync.Pool, mu removal из WriteMessage
- `src/internal/transport/websocket/websocket.go` — padding PRNG, BatchWriter pool, control writer channel
- `src/internal/ratelimit/ratelimit.go` — sync.Map для IPRateLimiter и SessionPacketLimiter
- `src/internal/dnsproxy/dnsproxy.go` — configMu + pendingMu split
- Границы: TUN/routing/NAT/ACL/session не меняются

## Performance Budget

- `none` — spec не ставит числовых целей, только behavioural (aloc/op снижается, race detector молчит).

## Implementation Surfaces

| Surface | Change | Existing |
|---------|--------|----------|
| `transport/quic/conn.go` | sync.Pool read buffer, deadlineMu, atomic maxMessageSize | existing |
| `transport/quic/obfuscated.go` | atomic nonceInit, xorBuf pool, WriteMessage без mu | existing |
| `transport/websocket/websocket.go` | math/rand/v2, BatchWriter pool, control writer | existing |
| `ratelimit/ratelimit.go` | sync.Map для limiters | existing |
| `dnsproxy/dnsproxy.go` | configMu + pendingMu | existing |

Все surfaces существующие — новых файлов/пакетов не требуется.

## Bootstrapping Surfaces

`none` — репозиторий содержит все необходимые пакеты и тестовую инфраструктуру.

## Влияние на архитектуру

- **Локальное:** все изменения внутри одной функции/метода. Никакие интерфейсы, экспортированные типы или управляющие потоки не меняются.
- **Интеграции:** ноль.
- **Migration/rollout:** не требуется — изменения не влияют на формат данных, протокол или конфигурацию.

## Acceptance Approach

- AC-001 → T1.1: `go test -race -bench=. -benchmem ./src/internal/transport/quic/...` PASS + code review sync.Pool
- AC-002 → T1.2: `go test -race -bench=. -benchmem ./src/internal/transport/quic/...` PASS
- AC-003 → T1.3: `go test -race ./src/internal/transport/quic/...` PASS (race detector ловит data race на bool)
- AC-004 → T1.4: `go test -race ./src/internal/transport/websocket/...` PASS
- AC-005 → T2.4: `go test -race -bench=. -benchmem ./src/internal/transport/websocket/...` PASS + code review
- AC-006 → T2.5: `go test -race ./src/internal/ratelimit/...` PASS
- AC-007 → T2.1+T2.2: `go test -race ./src/internal/transport/quic/...` PASS
- AC-008 → T3.1: `go test -race ./src/internal/dnsproxy/...` PASS
- AC-009 → T3.2: `go test -race -run TestWSControlPlane ./src/internal/transport/websocket/...` PASS
- AC-010 → T4: `go test -race ./src/internal/transport/... ./src/internal/ratelimit/... ./src/internal/dnsproxy/... && go vet ./...` PASS

Все evidence — без manual steps, CI-ready.

## Данные и контракты

- `data-model.md`: no-change stub. Никакие типы данных, protobuf-схемы, JSON/конфигурационные форматы не меняются.

## Стратегия реализации

### DEC-001 sync.Pool с фиксированным cap 1500 для QUIC буферов

- Why: >99% сообщений — MTU-размер. sync.Pool с `make([]byte, 1500)` покрывает типовой случай без аллокации. Редкие большие сообщения fallback на `make`.
- Tradeoff: cap=1500 не покрывает DNS TCP-ответы >1500 (fallback make — одна доп аллокация на ответ, приемлемо).
- Affects: `conn.go`, `obfuscated.go`
- Validation: `go test -race -bench=. -benchmem`

### DEC-002 deadlineMu отдельно от hot path

- Why: `SetReadDeadline`/`SetWriteDeadline` вызываются редко. Вынос в отдельный mutex убирает contention из WriteMessage без потери safety.
- Tradeoff: deadline и write могут взаимно перекрыться (gorilla/quic-go гарантируют thread-safe deadline change + concurrent write).
- Affects: `conn.go`
- Validation: `go test -race ./src/internal/transport/quic/...`

### DEC-003 WS control writer — канал вместо общего mutex

- Why: ping/pong не требуют padding и не должны ждать data writes. Буферизованный канал (cap=8) + горутина-писатель с отдельным вызовом `conn.WriteMessage` (не через `wmu`).
- Tradeoff: gorilla/websocket.Conn.WriteMessage внутренне сериализован своим `muW`. Control не ждёт `wmu`, но ждёт gorilla's muW. Для ping/pong это несущественно.
- Affects: `websocket.go`
- Validation: `go test -race -run TestWSControlPlane`

### DEC-004 sync.Map для rate limiter

- Why: `Allow()` — read-heavy (существующий IP читается), редкие write (новый IP). sync.Map optimised для read-heavy + rare write.
- Tradeoff: sync.Map медленнее шардированной map при >100k ключей. Ожидается <10k. Если упрётся — замена на шарды тривиальна.
- Affects: `ratelimit.go`
- Validation: `go test -race ./src/internal/ratelimit/...`

### DEC-005 DNS proxy split — два независимых mutex

- Why: config (читается часто) и pending (пишется/удаляется) — ортогональные data sets. Один RWMutex создаёт ложную contention.
- Tradeoff: lock ordering требует дисциплины (release before acquire). Документирован в spec.
- Affects: `dnsproxy.go`
- Validation: `go test -race ./src/internal/dnsproxy/...`

## Incremental Delivery

### MVP (Фаза 1 — Транспортные пулы + atomic)

T1.1–T1.4, AC-001–AC-004, AC-010.

### Фаза 2 — Lock removal + rate limiter + BatchWriter

T2.1–T2.5, AC-005–AC-007, AC-010.

### Фаза 3 — DNS proxy split + WS control plane

T3.1–T3.2, AC-008–AC-009, AC-010.

### Фаза 4 — Проверка

T4, AC-010.

## Порядок реализации

1. **Фаза 1 (MVP):** независимые изменения в 2 файлах. Можно параллелить T1.1+T1.2 (оба QUIC, sync.Pool), затем T1.3 (nonceInit CAS), затем T1.4 (padding PRNG).
2. **Фаза 2:** T2.1+T2.2 (mu removal) лучше делать вместе — оба QUIC. T2.3 (maxMessageSize atomic) — тривиально. T2.4 (BatchWriter) — независим. T2.5 (rate limiter) — независим.
3. **Фаза 3:** T3.1 (DNS split) — изолирован. T3.2 (WS control plane) — самый сложный, требует нового теста.
4. **Фаза 4:** `go test -race` всех затронутых пакетов.

Никакие изменения не требуют feature flag или rollout-стратегии — всё internal и обратно совместимо.

## Риски

- **Риск 1:** sync.Pool для QUIC ReadMessage — баг с невостребованным буфером (утечка). Mitigation: `ReturnBuffer`/`Put` в defer после использования, pattern из `framing/buffer_pool.go`.
- **Риск 2:** WS control writer — deadlock при закрытии conn. Mitigation: закрытие канала через `closeOnce` + select на `stopCh`.
- **Риск 3:** DNS proxy split — lock ordering violation при будущих изменениях. Mitigation: комментарий с lock ordering над объявлением mutex'ов.

## Rollout и compatibility

- Не требуется. Все изменения internal, без изменения протокола, конфигурации или API.

## Проверка

- T1.1–T2.5: `go test -race` каждого пакета после реализации
- T3.2: новый тест `TestWSControlPlane`
- T4: `go test -race` всех затронутых пакетов + `go vet`
- Каждый PR/Task: code review с фокусом на sync.Pool return, lock ordering, CAS vs Load+Store

## Соответствие конституции

- нет конфликтов
