# Arch Fix: Critical Paths — План

## Phase Contract

Inputs: spec, конституция, минимальный repo-контекст.
Outputs: plan, data-model stub.
Stop if: spec расплывчата — нет, spec конкретна, 7 AC с observable evidence.

## Цель

Инкрементально исправить 7 архитектурных проблем в core/relay без рефакторинга соседних пакетов. Каждый fix — отдельная задача, каждая задача — тест. Работа идёт последовательно: от самой опасной проблемы (QUIC backoff) к самой локальной (uint16 overflow).

## MVP Slice

Все 7 AC — равноправные части единого pass. Минимальный срез: AC-001 + AC-005 (QUIC backoff + uint16 overflow) — две самых критичных проблемы (падение сервера и битые пакеты). Остальные AC добавляются итеративно.

## First Validation Path

`go test -race -count=1 ./src/internal/bootstrap/relay/ ./src/internal/bootstrap/server/ ./src/internal/transport/quic/` — зелёный. Затем `gosec ./...` без новых warning.

## Scope

- `src/internal/bootstrap/relay/bootstrap.go` (runTerminator QUIC loop)
- `src/internal/bootstrap/relay/router.go` (forwardDNSQuery, cacheDNSResponse, buildDNSRespPacket, extractDestIP, isDNSQuery)
- `src/internal/bootstrap/relay/handler.go` (handleTerminatorStream, routeOutgoing)
- `src/internal/bootstrap/server/server.go` (Run QUIC loop)
- `src/internal/transport/quic/listen.go` (если нужен общий helper для backoff)
- `src/internal/server/handler.go` (не меняется — толькo если подскажет реализация relay session)
- `src/internal/bootstrap/relay/` test files

Границы scope не пересекают: config, transport interface, session manager interface, TUN, NAT, admin API, metrics.

## Performance Budget

- DNS cache max size: 10000 записей — ~2MB peak для map + values.
- DNS upstream pool: не более `runtime.GOMAXPROCS(0)` параллельных соединений.
- QUIC backoff: overhead < 1 alloc на вызов Accept.
- `none` для остальных изменений.

## Implementation Surfaces

| Поверхность | Почему | Новое/сущ |
|---|---|---|
| `relay/bootstrap.go:237-254` | QUIC accept loop без backoff | existing |
| `server/server.go:443-462` | QUIC accept loop без backoff | existing |
| `relay/router.go:32-71` | forwardDNSQuery: dial per request | existing |
| `relay/router.go:74-153` | cacheDNSResponse: без limit cache | existing |
| `relay/router.go:172-211` | buildDNSRespPacket: uint16 overflow | existing |
| `relay/router.go:275-293` | extractDestIP: без boundary check | existing |
| `relay/handler.go:49-163` | handleTerminatorStream: session без sm.Create | existing |
| `relay/router_test.go` | тесты router/packet parsing | new |
| `relay/handler_test.go` | тесты handler/session | new |
| `server/server_test.go` | тесты QUIC accept loop | new |
| `quic/listen.go` | helper: backoffAccept (если потребуется) | existing |

## Bootstrapping Surfaces

`none` — все поверхности существующие, тестовые файлы создаются рядом.

## Влияние на архитектуру

Локальное: каждая проблема фиксится в своём пакете, без изменения публичных интерфейсов. Единственное исключение — если backoff helper оказывается общим для server и relay, выносим в `transport/quic/listen.go`.

## Acceptance Approach

### AC-001 QUIC accept с backoff

- **Подход**: обернуть `listener.Accept(ctx)` цикл с `retryAfter()` — экспоненциальный backoff (10ms base, 5s max, ×2). Context.Canceled — немедленный return.
- **Surfaces**: `relay/bootstrap.go:237-254`, `server/server.go:443-462`
- **Evidence**: unit test: mock Accept возвращает ошибку N раз → assert loop не вышел.

### AC-002 DNS upstream через pool

- **Подход**: sync.Pool с net.DialTimeout("udp"), Get/Put с deadline. Или простой хелпер `getDNSConn()` с map[addr]pool.
- **Surfaces**: `relay/router.go:32-71`
- **Evidence**: unit test: 10 вызовов forwardDNSQuery → проверка числа созданных conn.

### AC-003 DNS cache с limit

- **Подход**: при вставке проверять `len(r.dnsCache) >= maxCacheSize`. Если превышен — удалить N записей с истекшим TTL, затем самую старую.
- **Surfaces**: `relay/router.go:74-153`
- **Evidence**: unit test: вставка 10001 записи → размер cache ≤ 10000.

### AC-004 Relay session в SessionManager

- **Подход**: в handleTerminatorStream после успешного handshake вызвать `r.sm.Create()`, затем `r.sm.SetCancel()`. В defer — `sm.Remove()`. Аналогично `sm.UpdateActivity` в tunSess.
- **Surfaces**: `relay/handler.go:49-163`, `session/session.go` (только вызовы)
- **Evidence**: integration test: connect → sm.List() > 0 → disconnect → sm.List() == 0.

### AC-005 uint16 overflow

- **Подход**: добавить `if totalLen > 65535 { return nil }` в начало buildDNSRespPacket после расчёта totalLen.
- **Surfaces**: `relay/router.go:179-181`
- **Evidence**: unit test: payload = make([]byte, 70000) → assert nil return.

### AC-006 Boundary check packet parsing

- **Подход**: каждая raw-функция получает guard: `len(packet) < N → return zero/false`. Прогнать через set граничных значений.
- **Surfaces**: `relay/router.go:156-170` (isDNSQuery), `router.go:275-293` (extractDestIP), `router.go:74-153` (cacheDNSResponse)
- **Evidence**: unit tests с len=0,1,2,..., truncated header.

### AC-007 Тесты relay handler

- **Подход**: handler_test.go с mock StreamConn, mock TunDevice, mock SessionManager. Happy path: handshake + session + pool. Error path: invalid token, pool exhausted, read error.
- **Surfaces**: `relay/handler_test.go` (new), `relay/handler.go`
- **Evidence**: `go test -race -count=1 -v ./src/internal/bootstrap/relay/` показывает тесты handler.

## Данные и контракты

- **Data model**: не меняется. Новых структур/полей не требуется. DNS cache limit — константа `defaultMaxCacheSize = 10000`, не config-параметр.
- **Контракты**: не меняются. Relay не экспортирует публичных API.
- `data-model.md`: status: no-change.

## Стратегия реализации

### DEC-001 Backoff helper — общая функция для server и relay

- **Why**: два идентичных QUIC accept цикла. Общий helper `acceptWithBackoff(ctx, listener, logger)` избегает дублирования. Сlear distinction: transient error = backoff, context.Canceled = return.
- **Tradeoff**: добавляет один уровень абстракции, но server и relay уже используют разные структуры (`Server.quicListener` vs `Relay.quicListener` — через `transport.TransportListener`). Helper оперирует на `quic.Listener`, не на интерфейсе.
- **Affects**: `internal/transport/quic/listen.go` (новый helper), `server/server.go`, `relay/bootstrap.go`
- **Validation**: unit test helper + оба accept loop покрыты тестом.

### DEC-002 DNS upstream pool — sync.Pool + dialPerKey

- **Why**: sync.Pool — без внешних зависимостей, уже используется в проекте (framing buffer pool). Ключ = upstream addr. Dial при Get, Put закрывает conn. Проще connection pool библиотек.
- **Tradeoff**: idle conn закрывается при GC коллекции pool — допустимо для DNS (легковесные UDP). Не подойдёт для TCP upstream.
- **Affects**: `relay/router.go`
- **Validation**: unit test с подсчётом числа dial.

### DEC-003 DNS cache eviction — TTL scan + oldest

- **Why**: LRU требует linked list/map — больше кода. Текущая схема уже TTL-based. При переполнении: сначала удаляем expired, если не хватило — одну самую старую (iter map, first found). Достаточно для 10k записей.
- **Tradeoff**: не идеальный порядок удаления, но worst case — временное превышение на 1 запись. Для 10k записей — незначимо.
- **Affects**: `relay/router.go`
- **Validation**: unit test с 10001 insert → assert ≤ 10000.

### DEC-004 Relay session интеграция — прямой вызов sm.Create

- **Why**: server handler уже использует этот паттерн. Relay не имеет rate limiting/ACL/TTL — создаём базовую сессию. SetCancel для корректной остановки tunSess при expiry.
- **Tradeoff**: relay получает session manager dependency — уже есть pool, добавляется sm. Минимальный diff.
- **Affects**: `relay/handler.go`, `relay/bootstrap.go` (initTerminator: sm уже есть)
- **Validation**: integration test.

## Incremental Delivery

### MVP (Первая ценность)

1. AC-001 (QUIC backoff) — самая опасная проблема (падение сервера)
2. AC-005 (uint16 overflow) — самый простой fix (3 строки)

### Итеративное расширение

3. AC-006 (boundary check) — защита парсинга
4. AC-002 (DNS pool) — производительность
5. AC-003 (DNS cache limit) — память
6. AC-004 (relay session) — наблюдаемость
7. AC-007 (тесты) — качество (может частично пересекаться с 1-6)

## Порядок реализации

1. **AC-005** (uint16) — 5 минут, без риска
2. **AC-006** (boundary checks) — 15 минут, без риска
3. **AC-001** (QUIC backoff) — 30 минут + тесты, backoff helper
4. **AC-002** (DNS pool) — 20 минут + тесты, sync.Pool
5. **AC-003** (DNS cache limit) — 15 минут + тесты, eviction
6. **AC-004** (relay session) — 20 минут + integration test
7. **AC-007** (handler tests) — 45 минут, mock-based

Последовательные (каждый fix верифицируется отдельно). Параллельно: AC-005 + AC-006 (независимые).

## Риски

- **Риск 1**: QUIC backoff может маскировать баги конфигурации (бесконечные retry). **Mitigation**: логгинг каждой ошибки Accept с уровнем Error; capped backoff 5s.
- **Риск 2**: DNS pool с sync.Pool может держать устаревшие соединения. **Mitigation**: deadline 5s на conn; sync.Pool естественно GC-ится.
- **Риск 3**: relay session интеграция может выявить несовместимость с существующей логикой sm (relay не использует idle timeout/TTL). **Mitigation**: sm.Start уже вызывается в initTerminator; set idleTimeout=0 для relay или явно отключить expiry.
- **Риск 4**: cache eviction без LRU может удалять "горячие" записи. **Mitigation**: для 10k лимита — маловероятно; при необходимости upgrade до LRU в будущем.

## Rollout и compatibility

- Все изменения backward-compatible: меняется только внутреннее поведение, не конфиги, не API, не протокол.
- Специальных rollout действий не требуется.

## Проверка

| Шаг | Что проверяем | AC / DEC |
|---|---|---|
| `go test -race ./src/internal/transport/quic/` | QUIC backoff helper | AC-001 |
| `go test -race ./src/internal/bootstrap/server/` | Server QUIC loop | AC-001 |
| `go test -race ./src/internal/bootstrap/relay/` | Relay handler, router, session | AC-002..007 |
| `gosec ./...` | Нет новых security warning | AC-001..007 |
| `golangci-lint run ./...` | Чистый lint | AC-001..007 |
| `go vet ./...` | Чистый vet | AC-001..007 |

## Соответствие конституции

- нет конфликтов. Traceability: каждый fix получает `@sk-task` и `@sk-test` маркеры. Go 1.25 консистентен.
