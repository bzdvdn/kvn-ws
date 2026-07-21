# Lock Optimization План

## Цель

Заменить `sync.Mutex` на `sync/atomic` для простых счётчиков/флагов/ID и на `sync.RWMutex` для read-heavy структур с редкой записью. Все изменения локальны в 4 файлах, не меняют публичного API и покрываются `go test -race`.

## MVP Slice

Collector в `metrics/client/buffer.go` — per-packet горячий путь. Закрывает AC-001, AC-002.

## First Validation Path

```bash
go test -race ./src/internal/metrics/...
go test -race ./src/internal/dnsproxy/...
go test -race ./src/internal/bootstrap/relay/...
go test -race ./src/internal/proxy/...
```

Все 4 пакета должны вернуть PASS.

## Scope

- `src/internal/metrics/client/buffer.go` — Collector: поля int64/float64 → atomic
- `src/internal/dnsproxy/dnsproxy.go` — Server.nextID → atomic.AddUint32; Server.mu → RWMutex
- `src/internal/bootstrap/relay/bridge.go` — Relay.upstreamConn → atomic.Bool
- `src/internal/bootstrap/relay/upstream.go` — upstreamSession.closed → atomic.Bool
- `src/internal/proxy/stream.go` — SessionStreams.Load + Manager.Get → RLock

Не меняется: публичное API, логика принятия решений, формат данных.

## Performance Budget

- `none` — изменения либо устраняют блокировку (atomic), либо уменьшают конкуренцию (RWMutex). Регресс невозможен.

## Implementation Surfaces

| Файл | Изменение | Существующий/Новый |
|---|---|---|
| `src/internal/metrics/client/buffer.go` | `sync.Mutex` → `sync/atomic` в Collector | существующий |
| `src/internal/dnsproxy/dnsproxy.go` | `sync.Mutex` → `sync.RWMutex` + `atomic.AddUint32` в Server | существующий |
| `src/internal/bootstrap/relay/bridge.go` | `sync.Mutex` + bool → `atomic.Bool` в Relay | существующий |
| `src/internal/bootstrap/relay/upstream.go` | `sync.Mutex` + bool → `atomic.Bool` в upstreamSession | существующий |
| `src/internal/proxy/stream.go` | `Lock` → `RLock` в SessionStreams.Load + Manager.Get | существующий |

## Bootstrapping Surfaces

- `none` — все изменения в существующих файлах

## Влияние на архитектуру

- Никакого. Все изменения заменяют механизм синхронизации на более лёгкий без изменения контрактов, типов или потока данных.

## Acceptance Approach

- **AC-001/AC-002** (Collector): заменить Mutex на atomic.AddInt64/LoadInt64 для txBytes/rxBytes/reconnects; для latencyMs — atomic.StoreUint64/LoadUint64 + math.Float64bits/frombits. `startedAt` остаётся под Mutex. Верификация: `go test -race ./src/internal/metrics/...`
- **AC-003** (nextID): заменить `s.mu.Lock(); s.nextID++; s.mu.Unlock()` на `atomic.AddUint32(&s.nextID, 1)`. Верификация: `go test -race ./src/internal/dnsproxy/...`
- **AC-004** (upstreamConn): заменить `sync.Mutex` + `bool` на `atomic.Bool` в Relay. Методы set/check используют atomic.Store/Load. Верификация: `go test -race ./src/internal/bootstrap/relay/...`
- **AC-005** (closed): заменить `sync.Mutex` + `bool` на `atomic.Bool` в upstreamSession. `isClosed()` → `Load()`, `Send()` → `Load()` + early return. Верификация: `go test -race ./src/internal/bootstrap/relay/...`
- **AC-006** (Server.mu RWMutex): `sync.Mutex` → `sync.RWMutex`. Сеттеры (SetStream, ClearStream, SetRouteFunc, SetOrigResolvers, SetTracker, SetDirectRouteFunc) и операции с pending map (HandleDNSResponse, forwardViaTunnel) используют Lock. Чтение конфига в forward() и resolveDirect() используют RLock. Верификация: `go test -race ./src/internal/dnsproxy/...`
- **AC-007** (SessionStreams.Load): `s.mu.Lock()` → `s.mu.RLock()`, `s.mu.Unlock()` → `s.mu.RUnlock()`. Верификация: `go test -race ./src/internal/proxy/...`
- **AC-008** (Manager.Get): `m.mu.Lock()` → `m.mu.RLock()`, `m.mu.Unlock()` → `m.mu.RUnlock()`. Верификация: `go test -race ./src/internal/proxy/...`

## Данные и контракты

- `data-model.md` — no-change stub. Никакие типы данных, API или контракты не меняются.

## Стратегия реализации

### DEC-001 Изолированные замены без рефакторинга

Why: каждое изменение заменяет ровно один механизм синхронизации, не меняя структуры кода. Это минимизирует риск регрессии.
Tradeoff: не рефакторим сопутствующий код (напр., не выделяем `pending` map в отдельный mutex). В будущем может потребоваться, но сейчас это oversplit.
Affects: все файлы из Scope.
Validation: `go test -race ./...` для каждого затронутого пакета.

### DEC-002 Collector.Snapshot без snapshot-консистентности

Why: каждое поле Collector читается через atomic.Load независимо. В отличие от Mutex (который давал консистентность набора полей), atomic.Load допускает, что между чтениями полей другой goroutine обновит значение. Для метрик это приемлемо — значения отображаются приближённо.
Tradeoff: Snapshot может содержать txBytes с одного момента, rxBytes — с другого. Для отображения метрик в UI это не имеет значения.
Affects: `src/internal/metrics/client/buffer.go`
Validation: тесты Collector не тестируют snapshot-консистентность — то есть изменение не заметно.

### DEC-003 RWMutex для Server.mu — один mutex на всё

Why: Server.mu защищает и конфиг-поля (редкая запись, частое чтение), и pending map (частые write). Разделение на два mutex'а дало бы максимальную производительность, но это сложнее и повышает риск deadlock'а. Один RWMutex — прагматичный компромисс.
Tradeoff: pending map write блокирует чтение конфига и наоборот. Конфиг читается только в forward/resolveDirect коротким захватом (чтение нескольких указателей), так что блокировка пренебрежима.
Affects: `src/internal/dnsproxy/dnsproxy.go`
Validation: `go test -race ./src/internal/dnsproxy/...`

## Incremental Delivery

### MVP (Первая ценность)

Collector atomic (AC-001, AC-002). `go test -race ./src/internal/metrics/...` — PASS.

### Итеративное расширение

1. Server.nextID atomic + Server.mu RWMutex (AC-003, AC-006) — DNS proxy
2. upstreamConn + upstreamSession.closed atomic (AC-004, AC-005) — relay
3. SessionStreams.Load + Manager.Get RLock (AC-007, AC-008) — proxy

## Порядок реализации

1. Collector (самый горячий путь, максимальный эффект)
2. DNS proxy (контention на DNS-запросах)
3. Relay флаги (изолированы, безопасны)
4. Proxy RLock (минимальный риск, можно параллелить с п.3)

Пункты 3 и 4 не зависят друг от друга и могут выполняться параллельно.

## Риски

- **Риск 1**: Race detector может не поймать гонку при определённом тайминге.
  Mitigation: код каждой замены тривиален (2–5 строк), code review достаточен. Для atomic.Bool и atomic.AddUint32 паттерн стандартный и многократно проверен.

- **Риск 2**: RWMutex: forward() читает конфиг под RLock, затем может вызывать forwardViaTunnel() которая берёт Lock. Это штатный паттерн upgradable read — не deadlock, т.к. Lock захватывается после полного Release RLock (forwardViaTunnel — новый вызов с новым захватом).
  Mitigation: убедиться что forward() не держит RLock при вызове forwardViaTunnel(). В текущем коде RLock/RUnlock — до вызова forwardViaTunnel, поэтому deadlock невозможен.

## Rollout и compatibility

- Никаких rollout-действий не требуется. Все изменения обратно совместимы на уровне ABI и поведения.

## Проверка

| Шаг | Команда | AC |
|---|---|---|
| 1 | `go test -race ./src/internal/metrics/...` | AC-001, AC-002 |
| 2 | `go test -race ./src/internal/dnsproxy/...` | AC-003, AC-006 |
| 3 | `go test -race ./src/internal/bootstrap/relay/...` | AC-004, AC-005 |
| 4 | `go test -race ./src/internal/proxy/...` | AC-007, AC-008 |
| 5 | `go vet ./...` | все |

## Соответствие конституции

- нет конфликтов
