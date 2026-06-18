# Relay-Terminator: маршрутизация трафика на relay

## Scope Snapshot

- In scope: relay, который принимает клиентский туннель, расшифровывает фреймы, принимает решение direct vs upstream на основе destination CIDR/домена, и отправляет external трафик через upstream VPN-туннель.
- Out of scope: SOCKS5/HTTP прокси-режим на relay; балансировка upstream; репликация состояния между relay.

## Цель

Пользователь поднимает relay в регионе, где есть доступ к internal ресурсам. Клиенты подключаются к relay, relay сам решает: internal CIDR/домены — direct (через сеть региона), всё остальное — через upstream VPN-сервер. Клиенту не нужны белые списки и split-tunnel — relay централизованно управляет маршрутизацией.

## Основной сценарий

1. Relay запускается с конфигом: `mode: relay`, `relay.routing` с direct CIDR/доменами, `server` для upstream.
2. Relay поднимает TUN, слушает WS (TCP) и опционально QUIC (UDP).
3. Клиент подключается к relay через WS или QUIC, шлёт ClientHello.
4. Relay выступает как сервер: принимает handshake, назначает IP, создаёт сессию.
5. Когда от клиента приходит пакет через туннель, relay определяет destination IP:
   - DNS-запросы (порт 53) перехватываются и резолвятся локально.
   - Если IP попадает в `relay.routing.direct` (CIDR или закэшированный domain) — пакет SNAT'ится и отправляется напрямую через TUN relay.
   - Если IP не в direct — relay SNAT'ит и шлёт через upstream tunnel на сервер.
6. Ответы идут обратно: external → upstream → relay → DNAT → клиент; internal → TUN relay → DNAT → клиент.

## User Stories

- P1 Story (MVP): relay с routing по CIDR + доменам, DNS interception на relay, один upstream сервер, NAT в userspace.
- P2 Story: QUIC upstream — relay подключается к upstream серверу через QUIC (не только WS), с обфускацией; upstream reconnect; lazy connect.

## MVP Slice

- Relay-терминатор с routing по CIDR и доменам (suffix и exact).
- DNS interception: relay перехватывает ВСЕ DNS-запросы клиента, direct-домены кэширует, остальные — нет.
- NAT в userspace (SNAT при отправке, DNAT при получении).
- Upstream reconnect: асинхронный, сериализованный mutex.
- ip_forward auto-enable на relay.
- Один upstream сервер, lazy connect.
- WS + QUIC клиенты подключаются к relay.

## First Deployable Outcome

Docker-compose пример: relay-terminator с direct CIDR `10.0.0.0/8` и direct domain `.local`. Клиент подключается к relay, `ping 10.1.2.3` и `ping some.internal.local` идут direct, `ping 8.8.8.8` — через upstream.

## Scope

- `cmd/relay` — отдельный бинарник, объединяет оба подрежима relay
- Два подрежима: `bridge` (существующий opaque pipe) и `terminator` (TUN + routing)
- Config: `relay.mode: bridge | terminator`
- **terminator**: Relay как server: accept WS/QUIC, session management, handshake, token validation
- **terminator**: TUN на relay: open, IP assignment, routing
- **terminator**: Routing engine: match destination IP против `routing.direct_ranges` (CIDR) + `routing.direct_domains` (suffix/exact) + DNS cache
- **terminator**: DNS interception: relay перехватывает ВСЕ DNS-запросы, direct-кэширует, non-direct резолвит без кэша
- **terminator**: NAT в userspace: SNAT на `routeOutgoing`, DNAT на `receiveLoop`
- **terminator**: Upstream reconnect: async, serialised by mutex
- **terminator**: ip_forward auto-enable на старте
- **terminator**: Relay как client: upstream tunnel для non-direct трафика (WS или QUIC, с обфускацией)
- `mode: relay` удаляется из `cmd/client`
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
                  │  │  TUN + routing engine        │──direct──▶ Internet
                  │  │  decrypt → route → encrypt   │   │
                  │  │  client session + IP pool    │──upstream──▶ Server
                  │  │  userspace NAT (SNAT/DNAT)   │   │
                  │  │  upstream reconnect (async)  │   │
                  │  └──────────────────────────────┘   │
                  └───────────────────────────────────┘
```

## Контекст

- Relay в `mode: bridge` перенесён из `cmd/client` в `cmd/relay`.
- Пакеты `internal/bootstrap/server`, `internal/tunnel`, `internal/tun` переиспользованы.
- Session handshake, IP pool — переиспользованы из сервера.
- TLS сертификат для входящих подключений — обязателен (на relay).
- Upstream соединение использует `transport` и `obfuscation` из конфига relay.
- TUN на relay требует NET_ADMIN capability в Docker.
- Relay проверяет токен клиента через `auth.tokens` (как сервер).
- NAT в userspace: SNAT в `routeOutgoing`, DNAT в `receiveLoop`, без iptables.

## Требования

- RQ-001 Relay ДОЛЖЕН принимать WS и QUIC подключения от клиентов (как сервер).
- RQ-002 Relay ДОЛЖЕН назначать IP клиенту из пула и выполнять handshake.
- RQ-003 Relay ДОЛЖЕН поднимать TUN устройство для маршрутизации direct-трафика.
- RQ-004 Relay ДОЛЖЕН открывать upstream tunnel к `server` для external трафика.
- RQ-005 Relay ДОЛЖЕН маршрутизировать пакеты: destination IP в `routing.direct` → direct (TUN relay), остальное → upstream tunnel.
- RQ-006 Relay ДОЛЖЕН поддерживать `routing.direct_ranges` (CIDR) и `routing.direct_domains` (suffix `.ru` / exact `internal.corp`).
- RQ-007 Relay ДОЛЖЕН корректно обрабатывать disconnect клиента (cleanup сессии, IP pool).
- RQ-008 Relay ДОЛЖЕН перехватывать DNS-запросы из туннеля клиента. Для direct-доменов — форвардить на upstream DNS и кэшировать resolved IP. Для non-direct доменов — резолвить локально без кэширования.
- RQ-009 Relay ДОЛЖЕН использовать `DomainMatcher` из `internal/routing/` для domain-матчинга (suffix и exact).
- RQ-010 Relay ДОЛЖЕН проверять токен клиента при handshake через `auth.tokens` (как сервер).
- RQ-011 Relay ДОЛЖЕН поддерживать конфигурацию upstream DNS-сервера (`relay.routing.dns`) с дефолтами.
- RQ-012 Relay ДОЛЖЕН выполнять NAT в userspace (SNAT/DNAT) для direct-трафика.
- RQ-013 Relay ДОЛЖЕН автоматически переподключаться к upstream при обрыве соединения.
- RQ-014 Relay ДОЛЖЕН поддерживать lazy connect: не падать при недоступности upstream на старте.
- RQ-015 Relay ДОЛЖЕН включать `net.ipv4.ip_forward=1` при старте (non-fatal).
- RQ-016 Relay ДОЛЖЕН корректно обрабатывать timeout в WS-сессии (не более 10 последовательных таймаутов подряд).

## Вне scope

- Балансировка между upstream серверами
- Relay-to-relay цепочки (многоуровневые relay)
- Кэширование DNS на relay (кэшируются только direct-домены)
- Relay без TUN (bridge-mode остаётся для простого pipe)
- Web UI для relay
- IPv6

## Критерии приемки

### AC-001 Relay принимает клиента и назначает IP

- **Given** relay запущен с `relay.mode: terminator` и routing direct CIDR
- **When** клиент (WS или QUIC) подключается к relay
- **Then** relay проверяет токен клиента, выполняет handshake, назначает клиенту IP из пула, клиент получает `handshake complete`

### AC-002 Direct трафик (CIDR) идёт через TUN relay

- **Given** relay с direct CIDR `10.0.0.0/8`, клиент подключён
- **When** клиент отправляет пакет на `10.1.2.3`
- **Then** relay SNAT'ит пакет и отправляет напрямую через свой TUN (не через upstream)

### AC-003 Direct трафик (domain) идёт через TUN relay

- **Given** relay с direct domain `.local`, клиент подключён
- **When** клиент делает DNS-запрос `some.internal.local`, затем шлёт пакет на resolved IP
- **Then** relay перехватывает DNS-запрос, запоминает IP, и пакет на этот IP идёт direct

### AC-004 External трафик идёт через upstream

- **Given** relay с upstream server, клиент подключён
- **When** клиент отправляет пакет на `8.8.8.8` (не в direct CIDR и не в direct domain)
- **Then** relay SNAT'ит и пересылает пакет через upstream tunnel

### AC-005 Relay поднимает TUN

- **Given** relay запускается
- **Then** relay создаёт TUN устройство, настраивает IP и маршруты, включает ip_forward

### AC-006 Cleanup при дисконнекте

- **Given** клиент подключён к relay
- **When** клиент отключается (graceful или timeout)
- **Then** relay освобождает IP сессии, закрывает upstream tunnel

### AC-007 Upstream reconnect

- **Given** relay с upstream server, клиент подключён, upstream active
- **When** upstream соединение обрывается
- **Then** relay асинхронно переподключается, direct-трафик продолжает работать

### AC-008 DNS interception (non-direct)

- **Given** relay с DNS interception, клиент подключён
- **When** клиент делает DNS-запрос на external домен (не direct)
- **Then** relay резолвит запрос локально, НЕ кэширует IP, последующие пакеты на этот IP идут upstream

### AC-009 Userspace NAT

- **Given** relay с direct CIDR, клиент подключён
- **When** клиент отправляет пакет на direct IP и получает ответ
- **Then** source IP пакета SNAT'ится на gateway IP relay, ответ DNAT'ится обратно клиенту

## Допущения

- Relay поднимается в регионе с прямым доступом к internal CIDR
- Upstream сервер — существующий KvN сервер (не требует изменений)
- TUN устройство на relay обязательно (NET_ADMIN)
- Relay использует app-layer encryption для расшифровки трафика клиента (crypto.key)
- Server-side NAT (nftables masquerade) для upstream трафика — ответственность сервера

## Решённые вопросы

1. **Отдельный cmd или mode?** Решено: отдельный `cmd/relay`. Bootstrap в `internal/bootstrap/relay/`.
2. **Как relay получает session key для расшифровки?** Relay использует свой `crypto.key` и генерирует `CryptoSalt` при handshake (как сервер).
3. **Нужен ли routing по доменам в MVP?** Решено: да. DNS interception на relay.
4. **Bridge или full terminator?** Решено: оба, выбор через `relay.mode`.
5. **NAT в iptables или userspace?** Userspace SNAT/DNAT в `nat.go`.
6. **Upstream reconnect?** Async reconnect, serialised by mutex, triggered from data path.
7. **QUIC upstream обфускация?** Да, `ObfuscatedQUICConn` при `obfuscation.enabled: true`.

## Открытые вопросы

1. **IPv6:** Relay TUN не имеет IPv6-маршрута. Клиенты с IPv6-first поведением (curl) падают. Нужно либо force ipv4 на клиенте, либо добавить IPv6-адрес на TUN relay.
2. **Server WS keepalive:** Сервер (`server/handler.go`) не вызывает `SetKeepalive()` на принятых WS-соединениях — 30s tunnelTimeout может срабатывать при активном клиенте.
