# Performance & Polish — План

## Цель

Форма реализации: 8 оптимизаций (sync.Pool, TCP_NODELAY, batch writes, MTU/PMTU, compression, multiplex) + load testing gate. Все изменения локальны — surfaces транспорта/фрейминга/конфига, домен не затронут.

## MVP Slice

Buffer pooling + TCP_NODELAY + batch writes + load testing gate. Закрывает AC-001, AC-002, AC-003, AC-008 до расширения на остальные AC.

## First Validation Path

1. `go test -bench=. -benchmem ./src/internal/transport/framing/` — аллокации encode/decode снижены ≥80%
2. `go test ./src/internal/transport/websocket/` — TCP_NODELAY и batch writes тесты pass
3. Gatetest с 1000+ сессий: throughput ≥80%, latency ≤15%

## Scope

- `src/internal/transport/framing/` — sync.Pool + PMTU segmentation
- `src/internal/transport/websocket/` — TCP_NODELAY, batch writes, permessage-deflate, multiplex subprotocol
- `src/internal/protocol/handshake/` — MTU negotiation (новое поле в handshake)
- `src/internal/config/` — новые поля конфига (compression, multiplex, MTU)
- `src/cmd/client/main.go`, `src/cmd/server/main.go` — передача новых конфигов
- `src/cmd/gatetest/` — нагрузочное тестирование
- `configs/loadtest.yaml` — конфиг load testing стенда
- Домен (session, routing, crypto, auth, nat, dns) — не затронут

## Implementation Surfaces

- **frame-buffer** (`framing.go`) — sync.Pool для `Encode()`/`Decode()`, PMTU segmentation
- **ws-conn** (`websocket.go`) — `SetNoDelay()` после upgrade, batch write buffer, `EnableCompression`/`SetCompressionLevel`, multiplex subprotocol handler
- **handshake** (`handshake/`) — новое поле MTU в ClientHello/ServerHello
- **config** (`config/client.go`, `config/server.go`) — поля: `Compression`, `Multiplex`, `MTU`
- **client-entry** (`cmd/client/main.go`) — инициализация WS с новыми конфигами
- **server-entry** (`cmd/server/main.go`) — инициализация WS с новыми конфигами
- **gatetest** (`cmd/gatetest/main.go`) — load testing скрипт
- **loadtest-config** (`configs/loadtest.yaml`) — конфиг для gatetest

## Bootstrapping Surfaces

- `configs/loadtest.yaml` — новый файл (шаблон конфига load testing)

## Влияние на архитектуру

- Локальное: изменения в infrastructure/transport, config — domain не затронут
- Handshake: обратно совместим (MTU field optional, default 1500)
- Multiplex/compression: feature-флаги, поведение по умолчанию выключено
- Load testing: новый бинарник/скрипт, не влияет на production

## Acceptance Approach

- AC-001: sync.Pool в `Encode()`/`Decode()`. Proof: `go test -bench=. -benchmem` покажет drop аллокаций.
- AC-002: `tcpConn.SetNoDelay(true)` после `websocket.UnderlyingConn()`. Proof: unit test через mock TCP conn.
- AC-003: buffer с coalescing перед `WriteMessage`. Proof: mock conn counters до/после.
- AC-004: новое поле MTU в handshake, min(client, server). Proof: handshake test.
- AC-005: segmentation в `Encode()` при payload > MTU. Proof: тест отправки >MTU.
- AC-006: `Upgrader/Dialer.EnableCompression = true`. Proof: тест compressed size < original.
- AC-007: subprotocol negotiation + channel routing. Proof: интеграционный тест 2 канала.
- AC-008: gatetest с configs/loadtest.yaml. Proof: CI pass с метриками.

## Данные и контракты

- Data model: не меняется — `data-model.md` со статусом no-change
- Handshake: добавляется опциональное поле MTU (uint16, default 1500) — обратная совместимость
- Config: новые опциональные поля — обратная совместимость (defaults false/0)
- IPC/внешние контракты: не меняются

## Стратегия реализации

### DEC-001: sync.Pool для буферов encode/decode

Why: каждая аллокация на hot path создаёт GC pressure; sync.Pool переиспользует буферы.
Tradeoff: небольшой overhead на Get/Put, но на порядок меньше аллокаций.
Affects: `framing.go`
Validation: benchstat снижение аллокаций ≥80%

### DEC-002: TCP_NODELAY через UnderlyingConn()

Why: Nagle's algorithm добавляет 40-200ms latency для мелких пакетов.
Tradeoff: больше мелких TCP-сегментов.
Affects: `websocket.go`
Validation: юнит-тест на NoDelay()

### DEC-003: Batch writes — буферизация перед WriteMessage

Why: каждый WriteMessage — syscall; coalescing снижает их количество.
Tradeoff: задержка на накопление (max batch interval или flush по таймеру).
Affects: `websocket.go`
Validation: mock conn проверяет количество вызовов WriteMessage

### DEC-004: MTU negotiation в handshake

Why: фреймы под размер сети предотвращают IP-фрагментацию.
Tradeoff: +4-8 байт к handshake.
Affects: `handshake/`, `config/`, `framing.go`
Validation: handshake тест проверяет min MTU на обоих концах

### DEC-005: PMTU strategy — segmentation

Why: без сегментации пакеты > MTU фрагментируются на IP уровне.
Tradeoff: логика сборки/разборки сегментов.
Affects: `framing.go`
Validation: тест отправляет >MTU и проверяет количество сегментов

### DEC-006: Permessage-deflate через EnableCompression

Why: proxy-compatible; WebSocket compression extension (RFC 7692).
Tradeoff: CPU overhead на compress/decompress.
Affects: `websocket.go`, `config/client.go`, `config/server.go`
Validation: compressed size < original

### DEC-007: Multiplex через WebSocket subprotocol

Why: стандартный механизм; не ломает прокси.
Tradeoff: subprotocol имеет ограничения (один активный subprotocol).
Affects: `websocket.go`, `config/`, `protocol/`
Validation: интеграционный тест с 2 каналами

### DEC-008: Load testing через отдельный конфиг

Why: гибкость параметров без пересборки.
Tradeoff: дополнительный файл.
Affects: `cmd/gatetest/`, `configs/loadtest.yaml`
Validation: CI-шаг выводит pass/fail

## Incremental Delivery

### MVP (Первая ценность)

1. sync.Pool для буферов
2. TCP_NODELAY
3. Batch writes
4. Load testing gate
**AC:** AC-001, AC-002, AC-003, AC-008
**Validation:** benchstat + unit tests + gatetest pass

### Итеративное расширение

5. MTU negotiation + PMTU strategy (AC-004, AC-005) — следующий инкремент после MVP
6. Permessage-deflate compression (AC-006) — после MTU
7. Multiplex channels (AC-007) — опционально, в последнюю очередь

## Порядок реализации

1. sync.Pool — изолировано, наименьший риск
2. TCP_NODELAY + Batch writes — меняют ws-conn поверхность
3. Load testing скрипт + конфиг — без изменений продакшна
4. MTU negotiation + PMTU — меняют handshake и framing
5. Compression — после MTU, чтобы MTU-сегменты сжимались корректно
6. Multiplex — последним, опционально за флагом

Параллельно безопасно: sync.Pool + TCP_NODELAY + load testing скрипт

## Риски

- **Gorilla/websocket UnderlyingConn() может вернуть non-TCP conn** (например, TLS поверх памяти). Mitigation: type assert + fallback.
- **Batch writes увеличивают latency для интерактивного трафика**. Mitigation: таймаут на накопление (max 5ms) или flush по заполнению буфера.
- **permessage-deflate CPU overhead на слабых устройствах**. Mitigation: feature-флаг, выключен по умолчанию.
- **MTU mismatch при разных сетевых путях**. Mitigation: PMTU fallback на 1500 при недоступности ICMP.

## Rollout и compatibility

- Все оптимизации (кроме sync.Pool) за feature-флагами — выключены по умолчанию
- sync.Pool не меняет поведение — безопасен всегда
- MTU поле в handshake — optional, default 1500 — обратная совместимость
- Load testing отдельный бинарник — не влияет на production
- Специальных rollout-действий не требуется

## Проверка

- `go test -bench=. -benchmem ./src/internal/transport/framing/` — AC-001
- `go test ./src/internal/transport/websocket/` — AC-002, AC-003, AC-006, AC-007
- `go test ./src/internal/protocol/handshake/` — AC-004
- `go test ./src/internal/transport/framing/` (PMTU tests) — AC-005
- `go test ./src/cmd/gatetest/` — AC-008
- `go test -race ./...` — race detector

## Соответствие конституции

- нет конфликтов: все изменения в infrastructure/transport/config, domain не затронут
- Go без глобального состояния: sync.Pool — concurrent-safe, без глобальных переменных
- Traceability: `@sk-task` и `@sk-test` маркеры будут добавлены на implement
