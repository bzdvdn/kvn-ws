# Relay-Terminator: маршрутизация трафика на relay

## Scope Snapshot

- In scope: relay, который принимает клиентский туннель, расшифровывает фреймы, принимает решение direct vs upstream на основе destination CIDR/домена, и отправляет external трафик через upstream VPN-туннель.
- Out of scope: SOCKS5/HTTP прокси-режим на relay; балансировка upstream; кэширование DNS; репликация состояния между relay.

## Цель

Пользователь поднимает relay в регионе, где есть доступ к internal ресурсам. Клиенты подключаются к relay, relay сам решает: internal CIDR/домены — direct (через сеть региона), всё остальное — через upstream VPN-сервер. Клиенту не нужны белые списки и split-tunnel — relay централизованно управляет маршрутизацией.

## Основной сценарий

1. Relay запускается с конфигом: `mode: relay`, `relay.routing` с direct CIDR/доменами, `server` для upstream.
2. Relay поднимает TUN, слушает WS (TCP) и опционально QUIC (UDP).
3. Клиент подключается к relay через WS или QUIC, шлёт ClientHello.
4. Relay выступает как сервер: принимает handshake, назначает IP, создаёт сессию.
5. Когда от клиента приходит пакет через туннель, relay определяет destination IP:
   - Если IP попадает в `relay.routing.direct` — пакет отправляется напрямую через TUN relay (маршрут сети региона).
   - Если IP не в direct — relay шифрует пакет в новый туннель и шлёт на upstream VPN-сервер.
6. Ответы идут обратно: external → upstream → relay → клиент; internal → напрямую через TUN relay → клиент.

## User Stories

- P1 Story (MVP): relay с routing по CIDR + доменам, DNS interception на relay, один upstream сервер.
- P2 Story: несколько upstream серверов с выбором по признаку (client_ip, токен, path).

## MVP Slice

- Relay-терминатор с routing по CIDR и доменам (suffix `.ru`, `.local` и exact).
- DNS interception: relay перехватывает DNS-запросы клиента, запоминает domain→IP, маршрутит пакеты по domain-правилам.
- Переиспользовать `DomainMatcher` и DNS-логику из `internal/routing/`.
- Один upstream сервер.
- Relay поднимает TUN + становится клиентом для upstream.
- WS + QUIC клиенты подключаются к relay.

## First Deployable Outcome

Docker-compose пример: relay-terminator с direct CIDR `10.0.0.0/8` и direct domain `.local`. Клиент подключается к relay, `ping 10.1.2.3` и `ping some.internal.local` идут direct, `ping 8.8.8.8` — через upstream.

## Scope

- `cmd/relay` — отдельный бинарник, объединяет оба подрежима relay
- Два подрежима: `bridge` (существующий opaque pipe, переносится из `cmd/client`) и `terminator` (новый, с routing)
- Config: `relay.mode: bridge | terminator`
- **bridge** (бывший `mode: relay` в client): без TUN, без расшифровки, opaque pipe — переносится из `internal/bootstrap/client/relay.go` в `internal/bootstrap/relay/`
- **terminator**: Relay как server: accept WS/QUIC, session management, handshake, token validation
- **terminator**: TUN на relay: open, IP assignment, routing
- **terminator**: Routing engine: match destination IP против `routing.direct_ranges` (CIDR) + `routing.direct_domains` (suffix/exact)
- **terminator**: DNS interception: relay перехватывает DNS-запросы через туннель для domain→IP маппинга
- **terminator**: Relay как client: upstream tunnel для non-direct трафика
- **bridge**: без изменений в поведении, только перенос кода
- `mode: relay` удаляется из `cmd/client` — `Run()` содержит только `tun` и `proxy`
- Dockerfile + docker-compose пример для terminator
- Документация (docs/ru/relay.md, docs/en/relay.md)

### Общая архитектура

```
                  ┌───────────────────────────────────┐
                  │            Relay                   │
                  │  ┌──────────────┐  ┌────────────┐  │
Клиент ──tunnel──▶│  │ mode: bridge  │  │ upstream  │──▶── Server
                  │  │ (opaque pipe) │  │ (WS/QUIC) │  │
                  │  └──────────────┘  └────────────┘  │
                  │         ИЛИ                         │
                  │  ┌──────────────────────────────┐   │
                  │  │ mode: terminator             │   │
                  │  │  TUN + routing engine        │   │──direct──▶ Internal
                  │  │  decrypt → route → encrypt   │   │
                  │  │  client session + IP pool    │   │──upstream──▶ Server
                  │  └──────────────────────────────┘   │
                  └───────────────────────────────────┘
```

## Контекст

- Сейчас relay в `mode: relay` внутри `client` пакета (bridge). Архитектура нового relay будет другой — terminal + client upstream.
- Пакеты `internal/bootstrap/server`, `internal/tunnel`, `internal/tun` могут быть переиспользованы.
- Session handshake, IP pool, BoltDB persistence — уже есть в сервере, можно переиспользовать.
- TLS сертификат для входящих подключений — обязателен (на relay).
- Upstream соединение от relay использует те же опции (`transport`, `obfuscation`), что и обычный клиент.
- TUN на relay требует NET_ADMIN capability в Docker.
- Relay ДОЛЖЕН проверять токен клиента через `auth.tokens` (как сервер) для защиты от неавторизованных подключений.

## Требования

- RQ-001 Relay ДОЛЖЕН принимать WS и QUIC подключения от клиентов (как сервер).
- RQ-002 Relay ДОЛЖЕН назначать IP клиенту из пула и выполнять handshake.
- RQ-003 Relay ДОЛЖЕН поднимать TUN устройство для маршрутизации direct-трафика.
- RQ-004 Relay ДОЛЖЕН открывать upstream tunnel к `server` для external трафика.
- RQ-005 Relay ДОЛЖЕН маршрутизировать пакеты: destination IP в `routing.direct` → direct (TUN relay), остальное → upstream tunnel.
- RQ-006 Relay ДОЛЖЕН поддерживать `routing.direct_ranges` (CIDR) и `routing.direct_domains` (suffix `.ru` / exact `internal.corp`).
- RQ-007 Relay ДОЛЖЕН корректно обрабатывать disconnect клиента (cleanup сессии, IP pool).
- RQ-008 Relay ДОЛЖЕН перехватывать DNS-запросы из туннеля клиента, извлекать domain name и запоминать resolved IP для маршрутизации по domain-правилам.
- RQ-009 Relay ДОЛЖЕН использовать `DomainMatcher` из `internal/routing/` для domain-матчинга (suffix и exact).
- RQ-010 Relay ДОЛЖЕН проверять токен клиента при handshake через `auth.tokens` (как сервер).

## Вне scope

- Балансировка между upstream серверами
- Relay-to-relay цепочки (многоуровневые relay)
- Кэширование DNS на relay
- Relay без TUN (bridge-mode остаётся для простого pipe)
- Web UI для relay

## Критерии приемки

### AC-001 Relay принимает клиента и назначает IP

- Почему важно: клиент должен получить IP и установить сессию, чтобы relay мог маршрутизировать его трафик.
- **Given** relay запущен с `mode: relay` и routing direct CIDR
- **When** клиент (WS или QUIC) подключается к relay
- **Then** relay проверяет токен клиента, выполняет handshake, назначает клиенту IP из пула, клиент получает `handshake complete`
- Evidence: лог relay `handshake completed`, клиент получает IP; при неверном токене — `auth failed`

### AC-002 Direct трафик (CIDR) идёт через TUN relay

- **Given** relay с direct CIDR `10.0.0.0/8`, клиент подключён
- **When** клиент отправляет пакет на `10.1.2.3`
- **Then** relay отправляет пакет напрямую через свой TUN (не через upstream)
- Evidence: tcpdump на relay видит пакет на `10.1.2.3`; лог relay `route=direct`

### AC-003 Direct трафик (domain) идёт через TUN relay

- **Given** relay с direct domain `.local`, клиент подключён
- **When** клиент делает DNS-запрос `some.internal.local`, затем шлёт пакет на resolved IP
- **Then** relay перехватывает DNS-запрос, запоминает IP, и пакет на этот IP идёт direct
- Evidence: лог relay `dns intercept: some.internal.local → 10.10.0.42`; лог relay `route=direct (domain=.local)`

### AC-004 External трафик идёт через upstream

- **Given** relay с upstream server, клиент подключён
- **When** клиент отправляет пакет на `8.8.8.8` (не в direct CIDR и не в direct domain)
- **Then** relay шифрует и пересылает пакет через upstream tunnel
- Evidence: на upstream сервере появляется трафик от relay; лог relay `route=upstream`

### AC-005 Relay поднимает TUN

- **Given** relay запускается
- **Then** relay создаёт TUN устройство, настраивает IP и маршруты
- Evidence: `ip addr` показывает TUN интерфейс с IP из пула relay

### AC-006 Cleanup при дисконнекте

- **Given** клиент подключён к relay
- **When** клиент отключается (graceful или timeout)
- **Then** relay освобождает IP сессии, закрывает upstream tunnel для этого клиента
- Evidence: IP пул показывает IP как свободный; нет утечки upstream соединений

## Допущения

- Relay поднимается в регионе с прямым доступом к internal CIDR
- Upstream сервер — существующий KvN сервер (не требует изменений)
- Relay использует app-layer encryption или session keys для расшифровки трафика клиента (иначе не увидит destination IP пакетов)
- TUN устройство на relay обязательно (NET_ADMIN)
- Relay имеет достаточно прав для настройки сетевых маршрутов

## Открытые вопросы

1. ~~Отдельный cmd или mode?~~ **Решено: отдельный `cmd/relay`.** Bootstrap в `internal/bootstrap/relay/`, общие компоненты через `internal/tunnel`, `internal/session`, `internal/tun`, `internal/transport`. Плюсы: чистое разделение, независимый деплой, упрощённое тестирование.

2. **Как relay получает session key для расшифровки фреймов клиента?** Сейчас handshake между клиентом и сервером устанавливает session key через app-layer encryption (`crypto.key` + `CryptoSalt`). Relay должен знать `crypto.key` клиента, либо relay использует свой собственный `crypto.key` и генерирует свой `CryptoSalt`. Иначе relay не сможет расшифровать фреймы и увидеть destination IP пакетов.

3. ~~Нужен ли routing по доменам в MVP?~~ **Решено: да, домены в MVP.** DNS interception на relay через перехват DNS-запросов клиента (как в существующем `TunRouter`). DomainMatcher переиспользуется из `internal/routing/`. Suffix (`.ru`) и exact (`internal.corp`) форматы.

4. **Как relay узнаёт upstream server?** Через `server` в конфиге (как сейчас). Relay использует `dialStream()` со своим конфигом для upstream tunnel — obfuscation на upstream от relay, не от клиента.

5. ~~Режим bridge-терминатор или full-terminator?~~ **Решено: оба, выбор через `relay.mode`.** bridge (существующий, opaque pipe) и terminator (TUN + routing + upstream client).
