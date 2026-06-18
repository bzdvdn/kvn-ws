# Relay-Terminator: План (post-MVP)

## Phase Contract

Inputs: spec, inspect (pass), repo context.
Outputs: plan, data-model stub.

## Цель

Добавить `cmd/relay` — отдельный бинарник, который работает как терминатор туннеля: принимает WS/QUIC клиентов, расшифровывает фреймы, маршрутизирует пакеты по destination IP (direct vs upstream), выполняет NAT в userspace, и для upstream трафика открывает свой туннель к основному VPN-серверу с поддержкой reconnect. Bridge-режим (существующий opaque pipe) перенесён в `cmd/relay` как legacy.

## MVP Slice

Реализован terminator-режим: relay как сервер + TUN + routing по CIDR и доменам + DNS interception + NAT + upstream reconnect + lazy connect + QUIC upstream с обфускацией.

**Закрытые AC:** AC-001–AC-009 (все).

## First Validation Path

Docker-compose пример: relay-terminator с `routing.direct_ranges: ["10.0.0.0/8"]` и `routing.direct_domains: [".local"]`, upstream сервер.

## Scope

- `cmd/relay` — entrypoint (bridge + terminator)
- `internal/bootstrap/relay/` — bootstrap relay: bridge (перенос из client/relay.go) + terminator (TUN, routing, NAT, reconnect)
- `internal/config` — `RelayConfig` (объединяет server-like + client-like поля)
- `internal/routing` — переиспользован `CIDRMatcher`, `DomainMatcher`
- `cmd/client` — удалён `mode: relay` из `Run()`
- TUN, session, IP pool — переиспользованы из `internal/tun`, `internal/session`, `internal/tunnel`
- `internal/bootstrap/relay/nat.go` — новый: userspace SNAT/DNAT
- `examples/relay-terminator/` — docker-compose
- `docs/` — обновлены

## Implementation Surfaces (фактические)

| Пакет | Изменение | Статус |
|---|---|---|
| `src/cmd/relay/main.go` | **Новый** — entrypoint | Done |
| `src/internal/config/client.go` | `RelayConfig` + `RelayTermCfg` + `RelayRoutingCfg` + `RelayDNSCfg` | Done |
| `src/internal/bootstrap/client/client.go` | Удалена relay-ветка из `Run()` | Done |
| `src/internal/bootstrap/client/relay.go` | Перенесён в relay/bridge.go | Done |
| `src/internal/bootstrap/relay/bootstrap.go` | **Новый** — TUN, WS/QUIC accept, session mgmt, lazy connect, reconnect, ip_forward | Done |
| `src/internal/bootstrap/relay/bridge.go` | **Новый** (перенос) — opaque pipe bridge | Done |
| `src/internal/bootstrap/relay/handler.go` | **Новый** — WS handler, routeOutgoing, NAT, DNS, reconnect trigger | Done |
| `src/internal/bootstrap/relay/upstream.go` | **Новый** — upstream QUIC/WS dial, ObfuscatedQUICConn, buildSession, receiveLoop, reconnect | Done |
| `src/internal/bootstrap/relay/router.go` | **Новый** — routing engine + DNS intercept helpers | Done |
| `src/internal/bootstrap/relay/nat.go` | **Новый** — userspace SNAT/DNAT | Done |
| `src/internal/tunnel/session.go` | OutgoingInterceptor hook, recover(), timeout retry limit | Adapted |

## Ключевые решения (factual vs planned)

### DEC-001 Отдельный cmd/relay ✅
Выполнено.

### DEC-002 Конфиг: новый RelayConfig ✅
Выполнено.

### DEC-003 Routing: CIDRMatcher + DomainMatcher из internal/routing ✅
Выполнено. DNS interception: ВСЕ DNS запросы перехватываются, direct-кэшируются, non-direct резолвятся без кэша.

### DEC-004 Upstream: dialStream из bootstrap/client ✅
Выполнено. С дополнительной обфускацией QUIC (`ObfuscatedQUICConn`).

### DEC-005 Session: переиспользовать internal/session + internal/tunnel ✅
Выполнено. Добавлен `OutgoingInterceptor` hook для routing.

### DEC-006 NAT в userspace (не DEC в оригинальном плане)
SNAT в `routeOutgoing`, DNAT в `receiveLoop`. Избегает iptables на relay.

### DEC-007 Upstream reconnect (не DEC в оригинальном плане)
Асинхронный reconnect из data path, serialised `upstreamMu`, not blocking hot path.

### DEC-008 ip_forward auto-enable
`sysctl -w net.ipv4.ip_forward=1` в `initTerminator` (non-fatal).

### DEC-009 Timeout hardening
`net.Error.Timeout()` вместо `os.ErrDeadlineExceeded`; max 10 consecutive timeouts.

## Incremental Delivery (фактическое)

### MVP (Реализовано)
- `cmd/relay` + `internal/bootstrap/relay/` + конфиг
- Bridge-режим: перенос из `client/relay.go` в `relay/bridge.go`
- TUN setup + WS/QUIC accept + handshake (terminator)
- Routing: direct CIDR + direct domains (DomainMatcher + DNS intercept)
- Upstream tunnel через dialStream (WS + QUIC)
- DNS interception: ВСЕ DNS запросы перехватываются
- Userspace NAT (SNAT/DNAT)
- Upstream reconnect (async)
- Lazy connect
- ip_forward auto-enable
- Timeout hardening
- QUIC upstream с обфускацией
- Cleanup: IP pool release, session close
- Dockerfile target + docker-compose пример

### P2 (Реализовано)
- QUIC upstream — relay подключается к серверу через QUIC + авто-выбор по транспорту клиента
- Upstream reconnect
- DNS interception full (R8, R11)

### P3 (Не реализовано)
- Несколько upstream серверов
- Web UI / metrics для relay

## Риски (post-MVP)

- **Server WS keepalive missing**: сервер не зовёт `SetKeepalive()` → 30s tunnelTimeout → session ends. Открытый вопрос.
- **IPv6 not handled**: relay TUN без IPv6 → клиенты с IPv6-first терпят неудачу. Открытый вопрос.

## Rollout и compatibility

- Bridge relay (legacy) — в `cmd/relay` как `relay.mode: bridge`.
- Terminator — основной режим `cmd/relay`.
- `mode: relay` удалён из `cmd/client`.

## Проверка

- `go build ./src/cmd/...` — OK
- `go test ./src/internal/config/...` — PASS (15 тестов)
- `go test ./src/internal/bootstrap/relay/...` — PASS (5 тестов)
- docker-compose logs содержат ожидаемые маркеры `route=direct`, `route=upstream`, `dns intercept`
