# Context — src/internal/bootstrap/relay

## Purpose

Промежуточный узел KVN в двух режимах: `bridge` — прозрачно проксирует уже установленный туннель к вышестоящему серверу (WS→WS), `terminator` — сам принимает клиентов, выдаёт им IP из своего пула и маршрутизирует трафик: напрямую или через upstream-туннель.

## Responsibilities

- Владеет: TLS/QUIC-листенерами relay, bridge-копированием сообщений, terminator-сессиями (пул IP, handshake, TUN), маршрутизацией direct/upstream, NAT-трекером (SNAT/DNAT), DNS-перехватом и кэшем direct-маршрутов.
- Не владеет: протоколом handshake/кадрами (`protocol`, `framing`), пересылкой внутри сессии (`tunnel`), пулом/IP-логикой (`session`), TLS-билдерами (`transport/tls`).

## How it works

- `Run(ctx)`: `SIGTERM/SIGINT` → по `relay.mode`: `"terminator"` → `runTerminatorMode`, иначе `runBridgeMode`.
- Bridge: `relayTLSConfig` (сертификат из `relay.tls` или **самоподписанный** RSA-2048 на 10 лет) → `tls.Listen` + опционально QUIC → `http.Server` с `relayServeMux`; семафор `relay.max_connections`, allowlist `ws_paths`, проверка `Upgrade: websocket`; `bridgeRelayConn` читает ClientHello клиента (30с), диалит upstream (`relay.server`) **по WS**, пересылает hello/ServerHello как есть, затем две горутины копируют сообщения до ошибки.
- Terminator `initTerminator`: обязателен `relay.network`; `sysctl net.ipv4.ip_forward=1`; пулы IPv4/IPv6 + `SessionManager` (300с/24ч/10с); TUN + `TunDemux`; TLS; `natTracker`; при `relay.routing` — `RuleSet`, DNS-кэш (`dnsCache`, TTL из `cache_ttl`, пул UDP-коннектов).
- Upstream: при заданном `relay.transport` подключается сразу, иначе лениво на первом клиенте, выбирая транспорт по hint клиента (`quic`/иначе `tcp`→WS); QUIC при неудаче откатывается на WS; `doHandshake` отправляет ClientHello с `upstream_token` (или `KVN_RELAY_AUTH_TOKEN`) и получает `AssignedIp`.
- `handleTerminatorStream`: авторизует клиента по `relay.auth.tokens`, создаёт сессию через `SessionManager`, отвечает ServerHello с выданным IP, запускает `tunnel.NewSession` с демаксом; при наличии routing ставит `SetOutgoingInterceptor(r.routeOutgoing)`.
- `routeOutgoing`: DNS-перехват (`ParseDNSQuestion` + `forwardDNSQuery`, кэш для direct) → проверка DNS-кэша → `RuleSet.Route`: `RouteDirect` отдаёт пакет TUN, `RouteServer` шлёт в upstream, при необходимости предварительно делая SNAT в выданный IP; при отсутствии/обрыве upstream — асинхронный `reconnectUpstream`.
- `upstreamSession.receiveLoop` читает `FrameTypeData`, применяет DNAT и пишет пакет в общий TUN.
- NAT: in-process трекер по `(proto, clientPort, dstIP, dstPort)` для IPv4, перезапись source IP (SNAT) и обратно (DNAT) с пересчётом checksum; TTL записи — 5 минут.

## Key files

- `src/internal/bootstrap/relay/bootstrap.go` — `initTerminator`, upstream lifecycle, terminator listeners.
- `src/internal/bootstrap/relay/bridge.go` — `Relay`, `Run`, bridge-режим, TLS/self-signed, копирование.
- `src/internal/bootstrap/relay/handler.go` — terminator handshake/auth, `routeOutgoing`.
- `src/internal/bootstrap/relay/upstream.go` — dial/handshake upstream и `upstreamSession`.
- `src/internal/bootstrap/relay/router.go` — DNS-перехват/кэш, direct ruleset, парсинг пакетов.
- `src/internal/bootstrap/relay/nat.go` — SNAT/DNAT-трекер.

## Public contracts

- `New()` (CLI, `--config`) и `NewFromConfig(cfg, logger, configPath)`.
- `(*Relay).Run(ctx) error`.
- Конфиг `config.RelayConfig`: `relay.mode`, `relay.listen`, `relay.ws_paths`, `relay.max_connections`, `relay.tls`, `relay.quic`, `relay.network`, `relay.routing`, `relay.auth.tokens`, `server`, `transport`, `upstream_token`, `tls{ca_file,server_name,verify_mode,sni}`.
- Bridge-режим прозрачен для протокола: клиент и сервер видят обычный KVN handshake.

## Dependencies

- `src/internal/{config,session,tunnel,transport,transport/quic,transport/tls,transport/websocket,protocol/handshake,protocol/auth,protocol/control,routing,tun,logger}`; `quic-go`, `errgroup`.

## Consumed by

- `src/cmd/relay/main.go` — единственный entrypoint.

## Gotchas

- Terminator без `relay.network` не стартует (`relay.network is required for terminator mode`); bridge этот блок не требует.
- `enableIPForward` вызывает `sysctl` без проверки прав: ошибка только логируется, но без ip_forward ответы DNAT могут не дойти.
- Bridge всегда диалит upstream по **WS**, игнорируя `transport` в `dialRelayUpstream` (QUIC используется только на входящей стороне, если `relay.quic` задан).
- Самоподписанный сертификат генерируется при каждом старте, если `relay.tls.cert/key` не заданы — клиенты с `verify` его не примут.
- `natTracker` хранит `expires`, но нигде не проверяет и не удаляет записи: карта растёт неограниченно, `natTimeout` фактически не работает.
- NAT/SNAT/DNAT поддерживают только IPv4; IPv6-пакеты уходят в upstream без перезаписи адреса.
- `relaySessionID(token)` игнорирует токен (`token` используется только как fallback при ошибке `rand.Read`), поэтому ID сессии всегда случайный.
- В terminator DNS-запросы перехватываются только при `relay.routing.dns` и наличии routing; иначе `FrameTypeDNS` уходит по общему пути `tunnel.Session`.
- При `relay.transport` пустом и без клиентов upstream не подключён — `routeOutgoing` для `RouteServer` будет дропать пакеты до первого клиента/реконнекта.
- Ошибки чтения/записи в bridge закрывают обе стороны через `closeBoth` (`sync.Once`), но без логики переподключения — клиент должен переподключиться сам.
