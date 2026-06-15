# Relay-Terminator: План

## Phase Contract

Inputs: spec, inspect (pass), repo context (server bootstrap, session, routing, client upstream dial).
Outputs: plan, data-model stub.
Stop if: spec слишком расплывчата — нет, spec фиксирует scope и AC.

## Цель

Добавить `cmd/relay` — отдельный бинарник, который работает как терминатор туннеля: принимает WS/QUIC клиентов, расшифровывает фреймы, маршрутизирует пакеты по destination IP (direct vs upstream), и для upstream трафика открывает свой туннель к основному VPN-серверу. Bridge-режим (существующий opaque pipe) остаётся в `cmd/client`.

## MVP Slice

Реализовать terminator-режим: relay как сервер + TUN + routing по CIDR и доменам + DNS interception + upstream client tunnel.

**Закрываемые AC:** AC-001 (handshake), AC-002 (direct CIDR), AC-003 (direct domain), AC-004 (upstream route), AC-005 (TUN setup), AC-006 (cleanup).

## First Validation Path

Docker-compose пример: relay-terminator с `routing.direct_ranges: ["10.0.0.0/8"]` и `routing.direct_domains: [".local"]`, upstream сервер. Клиент подключается к relay:
- `ping 10.1.2.3` → relay логирует `route=direct`, пакет уходит через TUN relay  
- `nslookup some.internal.local` + `ping <resolved IP>` → relay логирует `dns intercept` + `route=direct`
- `ping 8.8.8.8` → relay логирует `route=upstream`, пакет виден на upstream сервере

## Scope

- `cmd/relay` — новый entrypoint (bridge + terminator)
- `internal/bootstrap/relay/` — bootstrap relay: bridge (перенос из client/relay.go) + terminator (новый)
- `internal/config` — новый `RelayConfig` (объединяет server-like + client-like поля)
- `internal/routing` — переиспользовать `CIDRMatcher`, `DomainMatcher`, DNS-логику `TunRouter`
- `cmd/client` — удалить `mode: relay` из `Run()`
- TUN, session, IP pool — переиспользовать из `internal/tun`, `internal/session`, `internal/tunnel`
- `internal/transport` — без изменений
- `examples/relay-terminator/` — docker-compose

## Implementation Surfaces

| Пакет | Изменение | Причина |
|---|---|---|
| `src/cmd/relay/main.go` | **Новый** | Отдельный entrypoint для relay-бинарника |
| `src/internal/config/client.go` | Добавить `RelayTerminatorCfg` + `Auth` | relay-terminator config (routing, upstream, TUN, crypto, auth) |
| `src/internal/config/server.go` | Без изменений | Server config не трогаем |
| `src/internal/bootstrap/client/client.go` | Удалить relay-ветку из `Run()` | `mode: relay` больше не в client |
| `src/internal/bootstrap/client/relay.go` | Перенести в `internal/bootstrap/relay/bridge.go` | Bridge-код переезжает в relay bootstrap |
| `src/internal/bootstrap/relay/bootstrap.go` | **Новый** | Server-like bootstrap: TLS, WS/QUIC accept, TUN, session mgmt |
| `src/internal/bootstrap/relay/bridge.go` | **Новый** (перенос из client/relay.go) | Opaque pipe bridge |
| `src/internal/bootstrap/relay/upstream.go` | **Новый** | Client-like upstream tunnel via dialStream |
| `src/internal/bootstrap/relay/router.go` | **Новый** | Routing engine: CIDR + domain match vs upstream fallback |
| `src/internal/routing/matcher.go` | Переиспользовать (без изменений) | CIDRMatcher уже есть |
| `src/internal/session/` | Переиспользовать (без изменений) | IPPool, SessionManager, BoltStore |
| `src/internal/tun/` | Переиспользовать (без изменений) | TunDevice |
| `src/internal/tunnel/session.go` | Небольшая адаптация | Возможно, relay-specific session (или переиспользовать как есть) |

Дата-модели и контракты:
- Routing CIDR — переиспользуем существующий `CIDRMatcher` из `internal/routing/matcher.go`
- RelayConfig отдельный от ClientConfig/ServerConfig

## Bootstrapping Surfaces

- `src/cmd/relay/main.go`
- `src/internal/bootstrap/relay/`
- `examples/relay-terminator/docker-compose.yml`
- `examples/relay-terminator/relay.yaml` config
- `examples/relay-terminator/client.yaml` config

## Влияние на архитектуру

- Существующий `mode: relay` в `cmd/client` (bridge) не трогается — обратная совместимость.
- `cmd/relay` — третий бинарник наравне с `cmd/client` и `cmd/server`.
- Relay-terminator переиспользует server-компоненты (session, IP pool, TUN) и client-компоненты (dialStream, upstream obfuscation).
- Docker multi-stage build добавляет target `relay`.

## Acceptance Approach

- **AC-001** (handshake): relay принимает WS/QUIC → handshake → `ClientHello` декодируется → `handshake complete`. Проверка: лог relay, клиент получает IP.
- **AC-002** (direct CIDR): клиент шлёт пакет на direct CIDR → relay пишет в TUN напрямую. Проверка: tcpdump на relay видит пакет + лог `route=direct`.
- **AC-003** (direct domain): клиент делает DNS-запрос на `.local` домен → relay перехватывает, запоминает IP → пакет на этот IP идёт direct. Проверка: лог `dns intercept` + `route=direct (domain=.local)`.
- **AC-004** (upstream route): клиент шлёт пакет вне direct CIDR/domain → relay шлёт через upstream tunnel. Проверка: upstream сервер видит трафик + лог `route=upstream`.
- **AC-005** (TUN setup): relay стартует → создаёт TUN, настраивает IP. Проверка: `ip addr` на relay.
- **AC-006** (cleanup): клиент отключается → relay освобождает IP, закрывает upstream. Проверка: IP пул, нет утечки соединений.

## Данные и контракты

- RelayConfig — новая структура, не конфликтует с ClientConfig/ServerConfig.
- Никаких изменений в wire protocols (frame format, handshake).
- `data-model.md` — stub `no-change` для wire protocols, `extended` для конфига.

## Стратегия реализации

### DEC-001 Отдельный cmd/relay

- **Why**: независимый деплой, изолированное тестирование, чистое разделение bridge (client) и terminator (relay) логики.
- **Tradeoff**: дублирование Dockerfile target и entrypoint boilerplate.
- **Affects**: `cmd/relay/`, `Dockerfile`, `internal/bootstrap/relay/`.
- **Validation**: `go build ./src/cmd/relay` собирает бинарник.

### DEC-002 Конфиг: новый RelayConfig в internal/config

- **Why**: relay имеет свой набор полей (server-like + routing + client-like upstream), втискивание в ClientConfig раздувает его.
- **Tradeoff**: небольшое дублирование с ServerConfig (TLS, Network, Session).
- **Affects**: `internal/config`, `internal/bootstrap/relay/`.
- **Validation**: загрузка relay.yaml с полным конфигом не падает.

### DEC-003 Routing: CIDRMatcher + DomainMatcher из internal/routing

- **Why**: оба уже реализованы и протестированы. DomainMatcher поддерживает suffix (`.ru`, `.local`) и exact домены, DNS resolution с кэшированием.
- **Tradeoff**: DNS interception на relay добавляет complexity, но без доменов routing неполный.
- **Affects**: `internal/routing` (без изменений), `internal/bootstrap/relay/router.go` (DNS-intercept loop).
- **Validation**: unit test CIDR + domain матчинга, DNS-intercept integration test.

### DEC-004 Upstream: dialStream из bootstrap/client

- **Why**: relay как клиент upstream использует те же механизмы (TLS, obfuscation, keepalive). Переиспользовать > копировать.
- **Tradeoff**: relay зависит от client-пакета (cyclic import risk? нет — relay импортирует client, не наоборот).
- **Affects**: `internal/bootstrap/client/dial.go` (переиспользовать или вынести dialStream в `internal/transport/`).
- **Validation**: relay открывает upstream WS/QUIC к серверу.

### DEC-005 Session: переиспользовать internal/session + internal/tunnel

- **Why**: IP pool, session manager, session persistence уже реализованы в сервере.
- **Tradeoff**: Session нужно создавать с relay-специфичным TunRouter (relay сам решает direct vs upstream).
- **Affects**: `internal/session`, `internal/tunnel`.
- **Validation**: relay назначает IP, создаёт сессию, чистит при дисконнекте.

## Incremental Delivery

### MVP (Первая ценность)

- `cmd/relay` + `internal/bootstrap/relay/` + конфиг
- Bridge-режим: перенос из `client/relay.go` в `relay/bridge.go`, без изменений поведения
- TUN setup + WS/QUIC accept + handshake (terminator)
- Routing: direct CIDR (CIDRMatcher) + direct domains (DomainMatcher + DNS intercept)
- Upstream tunnel через dialStream
- DNS interception: relay перехватывает DNS-запросы из туннеля, domain→IP mapping
- Cleanup: IP pool release, session close
- Dockerfile target + docker-compose пример

### Итеративное расширение

- P2: несколько upstream серверов
- P3: Web UI / metrics для relay

## Порядок реализации

1. Config: RelayConfig в `internal/config` + загрузчик
2. Bridge перенос: скопировать `client/relay.go` → `relay/bridge.go`, переименовать пакет, починить импорты
3. Bootstrap: `cmd/relay/main.go` + `internal/bootstrap/relay/bootstrap.go` (TUN, WS/QUIC listen, accept)
4. Server-side handshake + session creation (переиспользовать server handler)
5. Routing engine: CIDR match + DomainMatcher + DNS intercept → direct (TUN write) vs upstream
6. Upstream dial: relay как клиент (+ отдельный TUN/tunnel для upstream)
7. Cleanup: session close, IP release, upstream disconnect
8. Удалить `mode: relay` из `cmd/client`
9. Dockerfile + docker-compose пример + документация

Шаги 1-2 можно параллелить. Шаги 3-4 зависят от 2. Шаги 5-6 зависят от 4. Шаг 8 — после верификации bridge.

## Риски

- **Риск 1**: Cyclic dependency — relay импортирует client (для dialStream). **Mitigation**: вынести dialStream в `internal/transport/` как общий `DialUpstream()`, либо relay импортирует client-пакет без cyclic risk (client не импортирует relay).
- **Риск 2**: Session key distribution — как relay расшифровывает фреймы клиента. **Mitigation**: relay использует свой `crypto.key`, генерирует `CryptoSalt` при handshake; клиент должен trust relay CA.
- **Риск 3**: TUN + NET_ADMIN в Docker — relay требует privileged контейнер. **Mitigation**: документировать requirement; bridge-mode (существующий) остаётся без TUN.

## Rollout и compatibility

- Существующий bridge relay (`mode: relay` в `cmd/client`) не меняется.
- Новый terminator — отдельный бинарник `cmd/relay`.
- Для перехода с bridge на terminator: замена конфига и entrypoint.
- Docker: новый `relay` target в multi-stage build.

## Проверка

- Unit tests: config loading, CIDR matching, session creation
- Integration: docker-compose с relay-terminator + клиент + upstream сервер
- AC-001..005: ручная проверка через docker-compose logs + tcpdump
- `go vet ./...`, `go test ./...`

## Соответствие конституции

- Нет конфликтов. Go 1.22+ соблюдается. Trace-маркеры `@sk-task relay-terminator#Tx.y` будут проставлены на новые функции/методы. Документация bilingual.
