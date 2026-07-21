# Lock Optimization: sync.Mutex → sync/atomic + sync.RWMutex

## Scope Snapshot

- In scope: замена Mutex на atomic load/store/add (счётчики, bool, ID) и Mutex → RWMutex для read-heavy структур с редкой записью
- Out of scope: рефакторинг Mutex, защищающих map'ы/срезы с частыми writes, bytes.Buffer или stream-записи

## Цель

Разработчики получают более лёгкую синхронизацию без блокировок на горячих путях (per-packet счётчики трафика, флаги closed/upstream, ID-генерация, DNS-конфиг, proxy map reads), сохраняя корректность при параллельном доступе. Успех — `go test -race ./...` проходит, производительность Collector (AddTX/AddRX) не деградирует.

## Основной сценарий

1. Разработчик ревьюит код и находит `sync.Mutex`, защищающий одиночное поле простого типа (int64, bool, uint32).
2. Разработчик заменяет его на `sync/atomic` операцию без изменения публичного API, либо Mutex → RWMutex с RLock в read-методах.
3. Все тесты (включая `-race`) проходят, производительность на per-packet пути не падает.
4. Race detector не находит новых гонок.

## User Stories

- P1 Story: Как разработчик ядра, я хочу заменить Mutex в Collector (txBytes/rxBytes/reconnects/latencyMs) на atomic каунтеры, чтобы не блокировать per-packet запись метрик.
- P2 Story: Как разработчик relay, я хочу заменить Mutex в upstreamSession.closed на atomic.Bool, чтобы isClosed() не дёргал блокировку.
- P3 Story: Как разработчик DNS proxy, я хочу заменить Mutex в Server.nextID на atomic.AddUint32, чтобы генерация streamID не создавала конкуренцию.
- P4 Story: Как разработчик DNS proxy, я хочу заменить Mutex→RWMutex для Server.mu, чтобы параллельные DNS-запросы не блокировали друг друга при чтении конфигурации.
- P5 Story: Как разработчик proxy, я хочу заменить Lock→RLock в SessionStreams.Load() и Manager.Get(), чтобы читатели map не блокировали друг друга.

## MVP Slice

Collector в `metrics/client/buffer.go` — это горячий путь и P1. Остальное — следом, если не сломает тесты.

## First Deployable Outcome

После первого implementation pass можно показать `go test -race ./src/internal/metrics/...` и сравнение benchmark: `/src/internal/metrics/client/` без изменений API.

## Scope

- `src/internal/metrics/client/buffer.go` — Collector.AddTX, AddRX, IncReconnects, SetLatency, Snapshot
- `src/internal/dnsproxy/dnsproxy.go` — Server.nextID → `atomic.AddUint32`
- `src/internal/bootstrap/relay/bridge.go` — Relay.upstreamConn → `atomic.Bool`
- `src/internal/bootstrap/relay/upstream.go` — upstreamSession.closed → `atomic.Bool`
- `src/internal/dnsproxy/dnsproxy.go` — Server.mu Mutex → RWMutex (RLock для чтения конфига в forward/resolveDirect, Lock для pending map и сеттеров)
- `src/internal/proxy/stream.go` — SessionStreams.mu Lock→RLock в Load(); Manager.mu Lock→RLock в Get()

## Контекст

- В проекте уже используется `sync/atomic`: `proxy/stream.go` использует `atomic.AddUint32` для `nextStreamID` — подтверждённый паттерн.
- `metrics/client/Collector` вызывается на каждый пакет. Замена Mutex на atomic убирает contention на этом пути.
- `upstreamSession.closed` читается из `isClosed()` и пишется в `Send()` — классический паттерн под `atomic.Bool`.
- `Server.nextID` монотонно возрастает, никакой логики кроме инкремента — чистый `atomic.AddUint32`.
- `Server.mu` в dnsproxy защищает две категории: конфиг-поля (stream, routeDirect, tracker и др.) — читаются на каждый DNS-запрос, но меняются редко; и pending map — пишется в forwardViaTunnel/HandleDNSResponse. RWMutex позволит читателям не блокировать друг друга.
- `SessionStreams.Load()` и `Manager.Get()` в proxy читают map, но используют Lock — это избыточно, RLock достаточно.

## Зависимости

- `none` — только stdlib

## Требования

- RQ-001 Collector ДОЛЖЕН использовать `atomic.Load/Store/AddInt64` для txBytes, rxBytes, reconnects вместо Mutex
- RQ-002 Collector ДОЛЖЕН использовать `atomic.Store/LoadUint64` + `math.Float64bits/frombits` для latencyMs
- RQ-003 Server.nextID ДОЛЖЕН использовать `atomic.AddUint32` вместо захвата Server.mu
- RQ-004 Relay.upstreamConn ДОЛЖЕН использовать `atomic.Bool` для чтения/записи флага upstreamConn
- RQ-005 upstreamSession.closed ДОЛЖЕН использовать `atomic.Bool` для isClosed() и Send()
- RQ-006 Server.mu в dnsproxy ДОЛЖЕН быть заменён на RWMutex: forward() и resolveDirect() используют RLock, сеттеры и pending map операции Lock
- RQ-007 SessionStreams.Load() ДОЛЖЕН использовать RLock вместо Lock
- RQ-008 Manager.Get() ДОЛЖЕН использовать RLock вместо Lock
- RQ-009 После изменений `go test -race ./...` для затронутых пакетов ДОЛЖЕН проходить
- RQ-010 Тесты ДОЛЖНЫ остаться зелёными без изменений тестового кода

## Вне scope

- Замена Mutex в IPPool, SessionManager, RateLimiter, TunDemux — защищают map'ы/сложное состояние
- Замена RWMutex в dns.Cache, dns.Tracker, natTracker, DomainMatcher — защищают map'ы
- Замена Mutex в QUICConn.wmu/WSConn.wmu — защищают потоковые записи
- Замена Mutex в BatchWriter — защищает bytes.Buffer
- Разделение Server.mu на два отдельных mutex'а (один для конфига, другой для pending) — это oversplit, один RWMutex достаточен
- Изменение публичного API Collector или других структур
- Бенчмарки или performance-тесты для не-hot-path замен (nextID, closed, upstreamConn, RWMutex)

## Критерии приемки

### AC-001 Collector счётчики используют atomic вместо Mutex

- Почему это важно: per-packet путь не должен блокироваться на Mutex
- **Given** Collector с ненулевым txBytes, rxBytes, reconnects
- **When** вызывается `Snapshot()` после `AddTX(n)`, `AddRX(n)`, `IncReconnects()`
- **Then** значения корректно суммированы, Race detector молчит
- **Evidence** `go test -race ./src/internal/metrics/...` — PASS

### AC-002 Collector latencyMs использует atomic

- Почему это важно: SetLatency может вызываться конкурентно с Snapshot
- **Given** Collector с установленной latencyMs
- **When** вызывается `SetLatency(v)` и затем `Snapshot()`
- **Then** Snapshot.LatencyMs содержит последнее установленное значение
- **Evidence** `go test -race ./src/internal/metrics/...` — PASS

### AC-003 nextID генерация без Mutex

- Почему это важно: устранение лишней блокировки в DNS forwarder
- **Given** Server с nextID=0
- **When** несколько конкурентных вызовов `forwardViaTunnel`
- **Then** каждый вызов получает уникальный streamID, Race detector молчит
- **Evidence** `go test -race ./src/internal/dnsproxy/...` — PASS

### AC-004 upstreamConn флаг без Mutex

- Почему это важно: изоляция проверки upstream от Mutex в bridge
- **Given** Relay с upstreamConn=false
- **When** upstream устанавливается/сбрасывается конкурентно
- **Then** чтение возвращает корректное значение, Race detector молчит
- **Evidence** `go test -race ./src/internal/bootstrap/relay/...` — PASS

### AC-005 upstreamSession.closed без Mutex

- Почему это важно: Send() и isClosed() не должны конкурировать за Mutex ради bool
- **Given** upstreamSession с closed=false
- **When** конкурентно вызываются `Send()` и `receiveLoop` (закрывает)
- **Then** Send возвращает ошибку после закрытия, Race detector молчит
- **Evidence** `go test -race ./src/internal/bootstrap/relay/...` — PASS

### AC-006 Server.mu RWMutex для параллельных DNS-запросов

- Почему это важно: DNS-запросы не блокируют друг друга при чтении конфига
- **Given** Server с установленными stream, routeDirect, tracker
- **When** несколько конкурентных вызовов `forward()` читают конфигурацию через RLock, одновременно вызывается `SetStream()` через Lock
- **Then** все читатели получают консистентные значения, Race detector молчит
- **Evidence** `go test -race ./src/internal/dnsproxy/...` — PASS

### AC-007 SessionStreams.Load() использует RLock

- Почему это важно: параллельные чтения map не должны блокировать друг друга
- **Given** SessionStreams с несколькими сохранёнными conn
- **When** конкурентные вызовы `Load()` с разными ключами
- **Then** все вызовы возвращают корректные значения, Race detector молчит
- **Evidence** `go test -race ./src/internal/proxy/...` — PASS

### AC-008 Manager.Get() использует RLock

- Почему это важно: параллельные чтения map не должны блокировать друг друга
- **Given** Manager с несколькими Stream
- **When** конкурентные вызовы `Get()` с разными ID
- **Then** все вызовы возвращают корректные значения, Race detector молчит
- **Evidence** `go test -race ./src/internal/proxy/...` — PASS

- Почему это важно: Send() и isClosed() не должны конкурировать за Mutex ради bool
- **Given** upstreamSession с closed=false
- **When** конкурентно вызываются `Send()` и `receiveLoop` (закрывает)
- **Then** Send возвращает ошибку после закрытия, Race detector молчит
- **Evidence** `go test -race ./src/internal/bootstrap/relay/...` — PASS

## Допущения

- Все замены сохраняют семантику видимости памяти (atomic acquire/release эквивалентны Mutex в этих паттернах)
- Race detector ловит проблемы — дополнительных code review-проверок достаточно
- `Collector.startedAt` остаётся под Mutex (пишется один раз при Start, не hot path)
- RWMutex для Server.mu не split'ится на отдельные mutex'ы для конфига и pending map — достаточный компромисс

## Критерии успеха

- SC-001 `go test -race ./src/internal/metrics/...` — PASS (верификация отсутствия гонок)
- SC-002 `go test -race ./src/internal/dnsproxy/...` — PASS
- SC-003 `go test -race ./src/internal/bootstrap/relay/...` — PASS
- SC-004 `go test -race ./src/internal/proxy/...` — PASS (RLock замены)

## Краевые случаи

- `Collector.Snapshot()` читает все поля атомарно, но не snapshot-консистентно — это существующее поведение (Mutex тоже не давал транзакционности на все поля). OK.
- `upstreamSession.Send()` после `closed=true` должен вернуть ошибку — момент перехода не строго синхронизирован, но достаточно acquire-барьера atomic.
- `Server.nextID` wraparound на практике невозможен (uint32, 4 млрд), но при race не влияет на корректность.
- RWMutex для Server.mu: forward() читает конфиг-поля под RLock, затем может делать Lock для работы с pending map — это штатный паттерн. Deadlock'а нет, т.к. Lock не захватывается повторно в том же goroutine.
- RLock в Server.mu защищает чтение указателей — читатель видит или старое, или новое значение. Для сеттеров это ок (сеттер пишет указатель, читатель получает консистентный nil или объект).

## Открытые вопросы

- `none` — анализ проведён, каждое изменение тривиально и локально
