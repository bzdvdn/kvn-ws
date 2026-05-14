# Local Proxy Mode — План

## Цель

Добавить режим работы клиента как локального SOCKS5/HTTP CONNECT прокси. Единый бинарник, mode `proxy`/`tun` в конфиге. TCP-потоки через WS-фреймы нового типа FrameTypeProxy.

## MVP Slice

SOCKS5 listener + WS forwarding + mode config. Закрывает AC-001, AC-003, AC-004, AC-006 до расширения.

## First Validation Path

1. `kvn-ws --mode proxy --config configs/client.yaml` — поднимает SOCKS5 на 127.0.0.1:2310
2. `curl --socks5 127.0.0.1:2310 http://example.com` — проходит через туннель
3. `go test ./src/internal/proxy/` — SOCKS5 round-trip

## Scope

- Новый пакет `src/internal/proxy/` — SOCKS5 listener, HTTP CONNECT handler, stream management
- `src/internal/transport/framing/` — новый тип фрейма FrameTypeProxy = 0x05
- `src/internal/config/client.go` — поля Mode, ProxyListen, ProxyAuth
- `src/cmd/client/main.go` — mode-переключатель (proxy vs tun)
- `src/cmd/server/main.go` — обработчик FrameTypeProxy (TCP-forwarder)
- Существующие пакеты (handshake, session, routing, tun) — не затронуты

## Implementation Surfaces

- **frame-type** (`framing.go`) — `FrameTypeProxy = 0x05` константа
- **proxy-listener** (`internal/proxy/listener.go`) — SOCKS5 (RFC 1928) + HTTP CONNECT handler
- **proxy-stream** (`internal/proxy/stream.go`) — структура Stream (streamID, dst, conn, dataCh)
- **mode-config** (`config/client.go`) — `Mode string`, `ProxyListen string`, `ProxyAuth *ProxyAuthCfg`
- **client-entry** (`cmd/client/main.go`) — `if cfg.Mode == "proxy" { runProxyMode() } else { runTunMode() }`
- **server-entry** (`cmd/server/main.go`) — `case FrameTypeProxy: handleProxyStream()` — dial target + bidirectional copy

## Bootstrapping Surfaces

- `src/internal/proxy/` — новый пакет

## Влияние на архитектуру

- Новый infra-слой proxy не зависит от domain (session, routing — опционально для exclusion)
- FrameTypeProxy — обратно совместим (старый сервер игнорирует неизвестный тип фрейма)
- TUN-режим не меняется
- Server-side: новый handler в handleTunnel — горутина на поток, map streamID→conn

## Acceptance Approach

- AC-001: SOCKS5 listener → тест с `net.Dial("tcp", listener) + SOCKS5 handshake → echo`
- AC-002: HTTP CONNECT → тест с `http.Transport{Dial: connectProxyDial}`
- AC-003: Mode config → тест с mock config проверяет вызов runProxyMode/runTunMode
- AC-004: proxy_listen → тест проверяет `net.Listen` на указанном адресе
- AC-005: Auth → тест RFC 1929 authentication (valid/invalid credentials)
- AC-006: Cross-platform → CI build matrix (windows, linux, darwin) + smoke
- AC-007: Exclusion → mock DNS + Route() для доменов, verify direct vs tunnel

## Данные и контракты

- `FrameTypeProxy = 0x05` — новый тип фрейма, не ломает существующие
- Протокол внутри FrameTypeProxy: `[streamID:4][len:2][dst_len:2][dst][data]`
- Server получает фрейм, парсит streamID + dst, коннектится, форвардит данные
- Data model: не меняется — `data-model.md` со статусом no-change

## Стратегия реализации

### DEC-001: SOCKS5 через stdlib net

Why: нулевая внешняя зависимость, полный контроль над протоколом.
Tradeoff: ручная реализация RFC 1928 (несложно, ~100 строк).
Affects: `internal/proxy/listener.go`
Validation: тест SOCKS5 round-trip

### DEC-002: FrameTypeProxy + streamID

Why: мультиплексирование нескольких TCP-соединений через один WS.
Tradeoff: 4 байта overhead на фрейм для streamID.
Affects: `framing.go`, `internal/proxy/stream.go`
Validation: 2 параллельных потока не интерферируют

### DEC-003: Серверный TCP-forwarder

Why: каждый прокси-поток — отдельная горутина, изолирована.
Tradeoff: память на горутину, лимит на max streams.
Affects: `cmd/server/main.go`
Validation: тест с 2 параллельными CONNECT

### DEC-004: Exclusion через Route()

Why: переиспользование существующего DNS + RuleSet без дублирования.
Tradeoff: DNS resolution на каждый запрос (с TTL-кешем).
Affects: `internal/proxy/listener.go`, `internal/routing/`
Validation: тест с mock DNS

## Incremental Delivery

### MVP

1. FrameTypeProxy + mode config
2. SOCKS5 listener (без auth)
3. WS forwarding + server TCP-forwarder
**AC:** AC-001, AC-003, AC-004, AC-006

### Итеративное расширение

4. HTTP CONNECT handler (AC-002)
5. SOCKS5 auth (AC-005)
6. CIDR/domain exclusion (AC-007)

## Порядок реализации

1. FrameTypeProxy + mode config → независимо
2. Server TCP-forwarder → нужно для тестов
3. SOCKS5 listener + WS forwarding (MVP)
4. HTTP CONNECT handler
5. SOCKS5 auth
6. CIDR/domain exclusion
7. Cross-platform CI

## Риски

- **SOCKS5-клиенты не поддерживают UDP notification** — в spec заявлен только TCP
- **Server-side TCP-forwarder может утекать** — mitigation: map с контекстом, cleanup при закрытии WS
- **DNS resolution latency в exclusion** — mitigation: TTL-кеш уже есть в `internal/dns`

## Rollout и compatibility

- FrameTypeProxy (0x05) — старые серверы просто не знают такого типа и закрывают соединение
- Mode config — дефолт `tun` для обратной совместимости
- Proxy-режим не требует изменений на сервере (достаточно нового handler)
- Специальных rollout-действий не требуется

## Проверка

- `go test ./src/internal/proxy/` — SOCKS5 handshake, CONNECT, auth
- `go test ./src/cmd/client/` — mode switching
- `go test ./src/cmd/server/` — FrameTypeProxy handler
- `curl --socks5 127.0.0.1:2310 http://example.com` — smoke test
- `go test -race ./...` — race detector

## Соответствие конституции

- нет конфликтов: новый infra-слой proxy; domain (session, routing) не затронут
- Go без внешних зависимостей: SOCKS5 через stdlib net
- Traceability: `@sk-task` и `@sk-test` маркеры будут добавлены на implement
