# Lock Optimization Задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/metrics/client/buffer.go` | T2.1, T2.2 |
| `src/internal/dnsproxy/dnsproxy.go` | T3.1, T3.2 |
| `src/internal/bootstrap/relay/bridge.go` | T3.3 |
| `src/internal/bootstrap/relay/upstream.go` | T3.4 |
| `src/internal/proxy/stream.go` | T3.5 |

## Implementation Context

- Цель MVP: заменить Mutex на atomic в Collector для per-packet метрик (AC-001, AC-002)
- Инварианты/семантика:
  - Snapshot() после atomic: поля читаются независимо, snapshot-консистентности нет (как и раньше не требовалось)
  - startedAt остаётся под Mutex — не hot path
  - Server.mu → RWMutex: один mutex на всё (DEC-003), без split'а
- Границы scope:
  - Не меняем публичное API
  - Не split'им Server.mu на два mutex'а
- Proof signals: `go test -race ./...` для каждого затронутого пакета
- References: DEC-001, DEC-002, DEC-003

## Фаза 1: Основа

Пропущена — все изменения в существующих файлах, структурных prerequisite нет.

## Фаза 2: MVP — Collector atomic (AC-001, AC-002)

- [x] T2.1 Заменить txBytes, rxBytes, reconnects в Collector на atomic.AddInt64/LoadInt64/StoreInt64. Touches: `src/internal/metrics/client/buffer.go`
- [x] T2.2 Заменить Collector.latencyMs на atomic.StoreUint64/LoadUint64 + math.Float64bits/frombits. Touches: `src/internal/metrics/client/buffer.go`

## Фаза 3: Основная реализация

- [x] T3.1 Заменить Server.nextID на atomic.AddUint32; убрать nextID из-под Server.mu. Touches: `src/internal/dnsproxy/dnsproxy.go`
- [x] T3.2 Заменить Server.mu sync.Mutex → sync.RWMutex: сеттеры и pending map операции оставить Lock, forward()/resolveDirect() перевести на RLock. Touches: `src/internal/dnsproxy/dnsproxy.go`
- [x] T3.3 Заменить Relay.upstreamConn bool на atomic.Bool. Touches: `src/internal/bootstrap/relay/bridge.go`, `src/internal/bootstrap/relay/bootstrap.go`
- [x] T3.4 Заменить upstreamSession.mu + closed bool на atomic.Bool; isClosed() → Load(), Send() → Load() + early return. Touches: `src/internal/bootstrap/relay/upstream.go`
- [x] T3.5 Заменить Lock→RLock в SessionStreams.Load() и Manager.Get(). Touches: `src/internal/proxy/stream.go`

## Фаза 4: Проверка

- [x] T4.1 Запустить `go test -race ./src/internal/metrics/... ./src/internal/dnsproxy/... ./src/internal/bootstrap/relay/... ./src/internal/proxy/...` и убедиться в PASS. Touches: `src/internal/metrics/client/buffer.go`, `src/internal/dnsproxy/dnsproxy.go`, `src/internal/bootstrap/relay/bridge.go`, `src/internal/bootstrap/relay/upstream.go`, `src/internal/proxy/stream.go`
- [x] T4.2 Запустить `go vet ./...` — чистый вывод. Touches: все затронутые файлы

## Покрытие критериев приемки

- AC-001 -> T2.1, T4.1
- AC-002 -> T2.2, T4.1
- AC-003 -> T3.1, T4.1
- AC-004 -> T3.3, T4.1
- AC-005 -> T3.4, T4.1
- AC-006 -> T3.2, T4.1
- AC-007 -> T3.5, T4.1
- AC-008 -> T3.5, T4.1

## Заметки

- T3.1 и T3.2 можно выполнять в одном коммите (один файл)
- T3.3 и T3.4 независимы и могут параллелиться
- T3.5 независим от T3.1–T3.4
