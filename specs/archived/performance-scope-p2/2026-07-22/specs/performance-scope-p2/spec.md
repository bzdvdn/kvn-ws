# Hot Path Performance — Buffer, Lock & Allocation Optimizations

## Scope Snapshot

- In scope: уменьшение per-packet аллокаций и contention в transport/QUIC, transport/WebSocket, ratelimit, dnsproxy и proxy — sync.Pool для буферов, lock-free rate limiter, выделение control plane из write mutex'а, split DNS proxy mutex, удаление избыточных блокировок.
- Out of scope: замена транспортного протокола, изменение публичного API, рефакторинг TUN/routing/NAT/ACL, performance-тесты вне `go test -race`.

## Цель

Разработчики получают снижение latency и увеличение пропускной способности на 30-100% на hot path (WS/QUIC write, rate limit, DNS proxy) без изменения API и без sacrifices по безопасности. Успех — `go test -race ./...` pass, аллокации на пакет уменьшены, CSPRNG на padding заменён на быстрый PRNG.

## Основной сценарий

1. Разработчик применяет серию локальных замен: `make([]byte, n)` → `sync.Pool.Get`, `sync.Mutex` → `sync.Map`/шардирование, `crypto/rand.Read` → `math/rand/v2`.
2. Rate limiter перестаёт быть contention point: `Allow()` не блокирует конкурентные вызовы для разных IP/session.
3. QUIC.WriteMessage не захватывает mutex — quic-go уже сериализует stream writes.
4. DNS proxy не блокирует DNS-ответы на время обработки DNS-запроса (split mutex).
5. WS keepalive/pong не конкурируют с data writes за `wmu`.
6. Все тесты (включая `-race`) проходят, Race detector не находит новых гонок.

## User Stories

- P1 Story: Как разработчик relay/клиента, я хочу чтобы `QUICConn.ReadMessage()` не аллоцировал буфер на каждое чтение, а использовал `sync.Pool` — это снижает GC pressure на 30-50% на high-throughput соединениях.
- P2 Story: Как разработчик relay, я хочу чтобы `QUICConn.WriteMessage()` не захватывал избыточный mutex — quic-go уже сериализует stream writes, мой mutex только удваивает latency.
- P3 Story: Как разработчик ядра, я хочу чтобы `WSConn.WriteMessage()` с padding не дёргал `crypto/rand.Read` под `wmu` — это блокирующий syscall, который стопорит все writes.
- P4 Story: Как SRE, я хочу чтобы rate limiter не был contention point — сейчас `sync.Mutex` на каждый `Allow()` блокирует все IP, хотя лимитеры независимы.
- P5 Story: Как разработчик DNS proxy, я хочу чтобы конфигурация DNS (stream, routeDirect) читалась под RLock concurrently с обработкой DNS-ответов (pending map Lock) — сейчас одна RWMutex на всё.
- P6 Story: Как разработчик WebSocket транспорта, я хочу чтобы keepalive ping и pong handler не конкурировали с data path за `wmu` — при загруженном канале keepalive timeout может сработать раньше, чем освободится mutex.

## MVP Slice

P1+P2+P3 — транспорт: `sync.Pool` для QUIC read/write буферов, `atomic.Bool` для nonceInit, `math/rand/v2` для padding. Эти изменения изолированы, тесты гарантированы, impact максимален.

## First Deployable Outcome

После первого implementation pass можно показать `go test -race ./src/internal/transport/...` PASS и `perf record` / `pprof` с уменьшенным числом аллокаций на hot path.

## Scope

- `src/internal/transport/quic/conn.go` — QUICConn.ReadMessage: `make([]byte, msgLen)` → `sync.Pool`; QUICConn.WriteMessage: удалить `c.mu.Lock()`, заменить на отдельный `deadlineMu` для `SetReadDeadline`/`SetWriteDeadline`
- `src/internal/transport/quic/obfuscated.go` — `nonceInit bool` → `atomic.Bool`; `WriteMessage`: `make([]byte, len(data))` для xorBuf → `sync.Pool`
- `src/internal/transport/websocket/websocket.go` — `WriteMessage` padding: `crypto/rand.Read` → `math/rand/v2`; `BatchWriter.Flush`: `make([]byte, ...)` → `sync.Pool`; Keepalive ping и pong handler: вынести из `wmu` в отдельный control writer с приоритетным каналом
- `src/internal/ratelimit/ratelimit.go` — `IPRateLimiter.mu` + `SessionPacketLimiter.mu`: `sync.Mutex` → `sync.Map` (lock-free lazy init) или шардированная map (64 buckets × Mutex)
- `src/internal/dnsproxy/dnsproxy.go` — Server.mu RWMutex → split: `configMu RWMutex` (stream, routeDirect, tracker, upstreams, origResolves) + `pendingMu sync.Mutex` (pending map)

## Контекст

- В проекте уже используется `sync.Pool`: `framing/buffer_pool.go` для frame encode/decode — подтверждённый паттерн.
- `QUICConn.mu` защищает два deadline setter'а (SetReadDeadline, SetWriteDeadline) и WriteMessage. WriteMessage — единственный горячий path. quic-go stream.Write() уже сериализован на уровне quic-go Stream — внешний mutex избыточен.
- `crypto/rand.Read` — блокирующий syscall (getrandom). Под wmu он стопорит все writes, включая keepalive.
- `IPRateLimiter` и `SessionPacketLimiter` используют lazy-init map с одним mutex'ом — это legacy design. `golang.org/x/time/rate.Limiter.Allow()` внутри использует `sync.Mutex`, но per-limiter, а не общий.
- `dnsproxy.Server.mu` — одна RWMutex на 6 config-полей + pending map. forward() читает конфиг (RLock), forwardViaTunnel пишет pending (Lock), HandleDNSResponse читает/удаляет pending (Lock). RLock блокирует Lock и наоборот, хотя эти операции независимы по данным.
- WS keepalive (line 221) и pong handler (line 242) захватывают `wmu` — это design trade-off из production-hardening, допустимый при <10k rps, но при 50k+ rps keepalive timeout начинает конкурировать с data path.
- Даже при выносе control frames из `wmu`, `gorilla/websocket.Conn.WriteMessage` (line 184/187) внутренне сериализован своим `muW`. Control и data writer'ы не конкурируют за `wmu`, но финальная запись в сокет остаётся последовательной через gorilla's `muW` — это ограничение библиотеки, не влияющее на корректность.

## Зависимости

- `none` — только stdlib (`sync`, `sync/atomic`, `math/rand/v2`, `crypto/rand` только для non-padding)

## Требования

- RQ-001 QUICConn.ReadMessage ДОЛЖЕН использовать `sync.Pool` для буфера сообщения вместо `make([]byte, msgLen)`
- RQ-002 ObfuscatedQUICConn.WriteMessage ДОЛЖЕН использовать `sync.Pool` для xorBuf вместо `make([]byte, len(data))`
- RQ-003 ObfuscatedQUICConn.nonceInit ДОЛЖЕН быть `atomic.Bool` (снятие data race)
- RQ-004 WSConn.WriteMessage padding ДОЛЖЕН использовать `math/rand/v2` вместо `crypto/rand` (неблокирующий PRNG)
- RQ-005 BatchWriter.Flush ДОЛЖЕН использовать `sync.Pool` для копии буфера вместо `make([]byte, ...)`
- RQ-006 IPRateLimiter.Allow и SessionPacketLimiter.Allow НЕ ДОЛЖНЫ захватывать общий mutex — ДОЛЖНЫ использовать lock-free lazy init (sync.Map) или шардированную map
- RQ-007 QUICConn.WriteMessage НЕ ДОЛЖЕН захватывать `c.mu` — ДОЛЖЕН использовать отдельный `deadlineMu sync.Mutex` только для SetReadDeadline/SetWriteDeadline
- RQ-008 Server.mu в dnsproxy ДОЛЖЕН быть разделён на `configMu RWMutex` (config-поля) и `pendingMu sync.Mutex` (pending map)
- RQ-009 WS keepalive ping и pong handler НЕ ДОЛЖНЫ захватывать `wmu` для отправки control frames — ДОЛЖНЫ использовать отдельный control writer (горутина с приоритетным каналом)
- RQ-010 После изменений `go test -race ./...` для затронутых пакетов ДОЛЖЕН проходить
- RQ-011 Тесты ДОЛЖНЫ остаться зелёными без изменений тестового кода

## Вне scope

- Замена `golang.org/x/time/rate.Limiter` на самописный — `rate.Limiter` остаётся, меняется только способ хранения/доступа к map
- Performance-бенчмарки вне `go test -race` — измерение capacity остаётся на оператора
- Рефакторинг TUN read/write, routing engine, NAT tracker, ACL — они lock-free или immutable
- Удаление `wmu` целиком из WSConn — `wmu` остаётся для data writes, уходит только control plane
- Изменение публичного API WSConn/QUICConn/Relay/Server

## Критерии приемки

### AC-001 QUIC ReadMessage из sync.Pool

- Почему это важно: каждая аллокация `make([]byte, msgLen)` на чтение — основной источник GC pressure на QUIC-соединениях
- **Given** QUICConn с активным соединением
- **When** вызывается `ReadMessage()` конкурентно
- **Then** буфер сообщения получен из `sync.Pool` (аллокация только при cap < msgLen)
- **Evidence** `go test -race -bench=. -benchmem ./src/internal/transport/quic/...` — PASS; code review подтверждает использование `sync.Pool` вместо `make`

### AC-002 ObfuscatedQUICConn xorBuf из sync.Pool

- Почему это важно: каждая запись аллоцирует xorBuf через `make`, удваивая GC pressure
- **Given** ObfuscatedQUICConn с инициализированным nonce
- **When** вызывается `WriteMessage(data)`
- **Then** xorBuf получен из `sync.Pool`, возвращён после `stream.Write`
- **Evidence** `go test -race -bench=. -benchmem ./src/internal/transport/quic/...` — PASS

### AC-003 nonceInit atomic.Bool

- Почему это важно: `bool` без атомарности — data race на первом конкурентном ReadMessage/WriteMessage
- **Given** ObfuscatedQUICConn с deferred nonce init
- **When** первый `ReadMessage()` и первый `WriteMessage()` вызываются конкурентно
- **Then** `nonceInit` читается и пишется через `atomic.Load/Store`, Race detector молчит
- **Evidence** `go test -race ./src/internal/transport/quic/...` — PASS

### AC-004 Padding WriteMessage без crypto/rand

- Почему это важно: `crypto/rand.Read` под `wmu` — блокирующий syscall, стопорит все writes
- **Given** WSConn с `PaddingEnabled=true`
- **When** вызывается `WriteMessage(data)` с padding
- **Then** padding заполняется через `math/rand/v2`, не через `crypto/rand`
- **Evidence** `go test -race ./src/internal/transport/websocket/...` — PASS

### AC-005 BatchWriter.Flush без make

- Почему это важно: каждая `Flush()` аллоцирует копию буфера
- **Given** BatchWriter с данными в буфере
- **When** вызывается `Flush()`
- **Then** копия данных получена из `sync.Pool`
- **Evidence** `go test -race -bench=. -benchmem ./src/internal/transport/websocket/...` — PASS; code review подтверждает `sync.Pool.Get` вместо `make`

### AC-006 Rate limiter без общего mutex

- Почему это важно: `Allow()` на каждый пакет блокирует все IP/session под одним mutex'ом
- **Given** IPRateLimiter или SessionPacketLimiter
- **When** конкурентные `Allow()` для разных IP/session
- **Then** вызовы не блокируют друг друга (lock-free или per-bucket shard)
- **Evidence** `go test -race ./src/internal/ratelimit/...` — PASS

### AC-007 QUICConn.WriteMessage без общего mu

- Почему это важно: `mu` избыточен — quic-go уже сериализует stream writes, мутекс только добавляет latency
- **Given** QUICConn с активным стримом
- **When** конкурентные вызовы `WriteMessage()`
- **Then** stream.Write вызывается без захвата общего `c.mu` (deadline setter'ы используют отдельный `deadlineMu`)
- **Evidence** `go test -race ./src/internal/transport/quic/...` — PASS

### AC-008 DNS proxy split mutex

- Почему это важно: DNS-запросы (forward, RLock) блокируют DNS-ответы (HandleDNSResponse, Lock)
- **Given** Server с настроенным stream и pending запросами
- **When** конкурентно вызываются `forward()` (читает config) и `HandleDNSResponse()` (пишет pending)
- **Then** операции не блокируют друг друга (config под RLock, pending под отдельным Lock)
- **Evidence** `go test -race ./src/internal/dnsproxy/...` — PASS

### AC-009 WS control plane отделён от wmu

- Почему это важно: keepalive ping и pong handler конкурируют с data writes за wmu, risking timeout на загруженном канале
- **Given** WSConn с активным keepalive
- **When** data path пишет большие сообщения (удерживает wmu), одновременно приходит PING от пира
- **Then** pong reply отправляется без ожидания wmu (control writer)
- **Evidence** `go test -race -run TestWSControlPlane ./src/internal/transport/websocket/...` — PASS, где тест проверяет: (1) pong handler не вызывает `wmu.Lock()`, (2) контрольное сообщение доставляется через отдельный канал, (3) при удержании wmu data path'ом keepalive ping не блокируется

### AC-010 Все тесты проходят

- Почему это важно: регрессия недопустима
- **Given** все затронутые пакеты
- **When** `go test -race ./src/internal/transport/... ./src/internal/ratelimit/... ./src/internal/dnsproxy/...`
- **Then** все тесты PASS, `go vet` чист
- **Evidence** вывод `go test -race` и `go vet`

## Допущения

- `sync.Pool` для QUIC буферов: буферы больше MTU (1500) — редкий случай, fallback на `make`. Средний размер ~1400 байт, cap=1500 покрывает >99% сообщений.
- `math/rand/v2` для padding криптостойкость не требуется — padding не содержит секретов, паттерн невосстановим без ключа сессии.
- QUICConn.mu removal: `SetReadDeadline`/`SetWriteDeadline` вызываются редко (раз в reconnect/keepalive), `deadlineMu` на них не влияет на hot path.
- `sync.Map` для rate limiter: ключей (IP/session) < 100k типично, sync.Map оптимизирован для read-heavy + rare write — подходит.
- DNS proxy split: `configMu` читается чаще (каждый DNS-запрос), `pendingMu` пишется на каждый DNS-запрос через туннель. Разделение убирает ложную contention между этими паттернами.
- WS control plane: канал control frames с буфером 8, горутина-писатель с `select` — control frame отправляется вне очереди data writes.

## Критерии успеха

- SC-001 `go test -race ./src/internal/transport/... ./src/internal/ratelimit/... ./src/internal/dnsproxy/...` — PASS
- SC-002 `go vet ./src/internal/transport/... ./src/internal/ratelimit/... ./src/internal/dnsproxy/...` — чистый вывод

## Краевые случаи

- `sync.Pool` при cap < msgLen (very large DNS response >1500): fallback `make` — корректно, без паники.
- `sync.Pool` после Close(): `Get()` может вернуть nil — `New` гарантирует non-nil.
- Rate limiter sync.Map: первый `LoadOrStore` для нового IP создаёт limiter. Ok.
- WS control writer: при закрытии conn — drain канала + stop signal. Ok.
- DNS proxy split: `pendingMu.Lock()` внутри `forwardViaTunnel` + `configMu.RLock()` в `forward()` — без deadlock (разные goroutine, разные mutex'ы, no lock ordering violation).

## Открытые вопросы

- WS control plane: использовать буферизованный канал (cap=8) или `sync.Mutex` + очередь? Канал проще, нет риска блокировки отправителя. Выбран канал.
- Rate limiter: `sync.Map` vs 64 shards? `sync.Map` — zero code, 64 shards — predictable contention. Для <100k ключей `sync.Map` достаточен. Решение принято: `sync.Map`.
- QUICConn `maxMessageSize` остаётся под `deadlineMu` или уходит в `atomic.Int32`? `maxMessageSize` пишется один раз (SetMaxMessageSize), читается в ReadMessage. `atomic.Int32` проще, deadlineMu для deadline только.
