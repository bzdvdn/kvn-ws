# Arch Fix: Critical Paths — QUIC backoff, DNS, relay session, packet parsing

## Scope Snapshot

- In scope: исправление 7 архитектурных проблем в core/relay путях, выявленных архитектурным обзором: QUIC accept без backoff, DNS без пула соединений, DNS cache без eviction, relay session без SessionManager, uint16 overflow, сырой парсинг IP/DNS-пакетов, отсутствие тестов relay handler.
- Out of scope: рефакторинг архитектуры пакетов, введение codegen для протокола, переписывание transport слоя, Android-клиент, Web UI, изменения entry points.

## Цель

Разработчик и оператор получают надёжный relay-узел и сервер, которые не падают при transient QUIC-ошибках, не утекают память через DNS cache, не открывают UDP-соединение на каждый DNS-запрос, корректно регистрируют relay-сессии в мониторинге и не отправляют битые IP-пакеты при переполнении uint16. Каждая проблема подтверждается тестом до закрытия.

## Основной сценарий

1. Архитектурный обзор выявил 7 concrete проблем с measurable impact.
2. Для каждой проблемы пишется spec → plan → task → implementation → test.
3. После имплементации: `go test -race ./...`, `go vet ./...`, `gosec ./...`, `golangci-lint run ./...` проходят.
4. Регрессионные тесты на каждую проблему включены в CI.

## User Stories

- P1 (провал сервера): QUIC transient error не убивает весь сервер.
- P2 (утечка памяти): DNS cache не растёт бесконечно.
- P3 (производительность): DNS upstream через пул соединений.
- P4 (наблюдаемость): relay-сессии видны в мониторинге.
- P5 (корректность): uint16 overflow не порождает битый IP-пакет.
- P6 (устойчивость): сырой парсинг пакетов защищён от краевых случаев.
- P7 (тесты): relay handler покрыт acceptance-тестами.

## MVP Slice

Все 7 AC должны быть закрыты единым implementation pass — проблемы независимы, но находятся в одном bounded context (core/relay). Одна проблема = одна task в плане.

## First Deployable Outcome

После implementation pass: `go test -race ./...` зелёный, `gosec ./...` без новых предупреждений. Для AC-007 — тесты relay handler написаны и проходят.

## Scope

- `src/internal/bootstrap/relay/` — handler.go, router.go, bootstrap.go (QUIC accept, DNS cache, session, packet parsing)
- `src/internal/bootstrap/server/server.go` — QUIC accept loop
- `src/internal/bootstrap/server/handler.go` — только если требуется изменение сигнатуры StreamConn / session integration
- `src/internal/transport/quic/listen.go` — если требуется backoff helper
- `src/internal/session/` — если требуется общий helper для relay-сессий
- `src/internal/protocol/handshake/` — только bugfix при необходимости

## Контекст

- QUIC accept loop в server.go и relay/bootstrap.go идентичны по структуре: for { Accept → handle } без backoff при ошибке.
- DNS upstream в relay/router.go: `net.DialTimeout("udp")` на каждый запрос.
- DNS cache: `map[netip.Addr]time.Time` без LRU, без лимита размера.
- Relay handler (handleTerminatorStream) выделяет IP из pool, но не вызывает sm.Create/SetCancel — сессия невидима для мониторинга.
- buildDNSRespPacket: `binary.BigEndian.PutUint16(out[2:4], uint16(totalLen))` — при totalLen > 65535 тихо обрезается.
- Парсинг DNS-ответов и IP-заголовков выполняется вручную без валидации границ и helper-функций.
- Существующий `tunnel_integration_test.go` — шаблон для интеграционных тестов; relay test можно построить аналогично.

## Требования

- RQ-001 При transient ошибке QUIC Accept сервер/relay ДОЛЖЕН делать backoff (экспоненциальный, capped) и продолжать слушать.
- RQ-002 При context.Canceled QUIC Accept ДОЛЖЕН завершать goroutine без backoff.
- RQ-003 DNS upstream ДОЛЖЕН использовать пул переиспользуемых UDP-соединений (или dial per-request с sync.Pool).
- RQ-004 DNS cache ДОЛЖЕН иметь максимальный размер (TTL-based eviction не считается — нужен limit по количеству записей).
- RQ-005 Relay terminator ДОЛЖЕН регистрировать сессию через SessionManager.Create и SetCancel.
- RQ-006 buildDNSRespPacket ДОЛЖЕН проверять totalLen > 65535 и возвращать nil.
- RQ-007 Парсинг DNS-ответов и IPv4/IPv6 заголовков ДОЛЖЕН быть вынесен в функции с явной валидацией границ.
- RQ-008 Для каждого исправления ДОЛЖЕН быть тест: unit или integration.
- RQ-009 После изменений `go test -race ./...`, `go vet ./...`, `gosec ./...`, `golangci-lint run ./...` ДОЛЖНЫ проходить.

## Вне scope

- DNS cache с полноценным LRU (достаточно max-size + TTL eviction).
- Переход на connection pool library (используем простой sync.Pool с dial per-bucket).
- Рефакторинг SessionManager или его интерфейса.
- Полноценный parser для raw IP/UDP/DNS (исправляем границы и выносим в функции, без codegen).
- QUIC backoff для client-side dial.
- Тесты для всего relay (только handler, не upstream/NAT).

## Критерии приемки

### AC-001 QUIC accept с backoff при transient error

- **Given** сервер/relay запущен с QUIC listener
- **When** Accept возвращает transient error (например, `context.DeadlineExceeded` или EACCES)
- **Then** сервер делает backoff (не менее 10ms, не более 5s) и повторяет Accept
- **And** при `context.Canceled` goroutine завершается немедленно
- **Evidence**: unit-тест, который симулирует N временных ошибок Accept и проверяет, что accept loop жив после них

### AC-002 DNS upstream через dial pool

- **Given** relay в terminator mode с DNS routing
- **When** приходит DNS-запрос от клиента
- **Then** forwardDNSQuery использует переиспользуемое соединение из пула (dial или sync.Pool)
- **Evidence**: бенчмарк или unit-test с подсчётом созданных соединений: при 10 последовательных DNS-запросах число dial'ов < 10

### AC-003 DNS cache с ограничением размера

- **Given** relay в terminator mode с DNS caching
- **When** количество закэшированных записей превышает maxCacheSize (конфигурируемый, по умолчанию 10000)
- **Then** старые записи (по TTL) удаляются до снижения ниже maxCacheSize
- **Evidence**: unit-тест: вставить 10001 запись → cache не превышает 10000

### AC-004 Relay session зарегистрирована в SessionManager

- **Given** relay terminator принимает клиента и успешно проходит handshake
- **When** сессия создаётся
- **Then** она зарегистрирована в sm.Create() с IP allocation и SetCancel
- **And** при disconnect сессия удаляется через sm.Remove()
- **Evidence**: интеграционный тест: подключиться к relay → проверить sm.List() > 0 → отключиться → sm.List() == 0

### AC-005 buildDNSRespPacket с защитой от uint16 overflow

- **Given** buildDNSRespPacket получает payload, при котором totalLen > 65535
- **When** функция вызывается
- **Then** она возвращает nil
- **Evidence**: unit-тест с oversized payload

### AC-006 Валидация границ при сыром парсинге

- **Given** функции парсинга пакетов (extractDestIP, isDNSQuery, cacheDNSResponse, buildDNSRespPacket)
- **When** на вход подаётся некорректный/truncated пакет
- **Then** функция не паникует и возвращает корректный zero-value / false
- **Evidence**: fuzz-тест или набор unit-тестов с граничными случаями (len=0, len=1, truncated header)

### AC-007 Тесты relay handler

- **Given** существует handler relay terminator (handleTerminatorStream, routeOutgoing, forwardDNSQuery)
- **When** изменения внесены
- **Then** каждый public/package-level метод покрыт unit-тестом (минимум happy path + error path)
- **Evidence**: `go test -race -count=1 -v ./src/internal/bootstrap/relay/` проходит и показывает >0 тестов именно для handler

## Допущения

- Backoff параметры (начальная задержка 10ms, max 5s, множитель 2) зашиты в код, не требуют конфигурации.
- DNS pool использует простую sync.Pool с net.DialTimeout — достаточно для текущих нагрузок.
- maxCacheSize = 10000 — разумный default, достаточный для типичного relay.
- fuzz-тест для парсинга пакетов создаётся в отдельном _test.go файле, не в integration.
- Существующий `handleTerminatorStream` в relay handler не использует SessionManager — это баг, а не design decision.

## Критерии успеха

- SC-001 После фикса relay QUIC: relay переживает 100 последовательных transient Accept ошибок без падения (проверяется скриптом или тестом).
- SC-002 DNS upstream: при 1000 DNS-запросах от 10 клиентов создаётся не более 10 параллельных UDP-соединений.
- SC-003 Codebase: `gosec ./...` не выдаёт новых предупреждений.

## Краевые случаи

- QUIC Accept: context cancelled до начала backoff — goroutine завершается без задержки.
- DNS cache: maxCacheSize=0 означает отключение кэширования.
- SessionManager.Create в relay: если pool исчерпан — корректная ошибка клиенту.
- buildDNSRespPacket: udpLen > 65535 — возврат nil (уже проверяется, но дублируется защита).

## Открытые вопросы

- `none`
