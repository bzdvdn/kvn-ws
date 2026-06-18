# Relay-Terminator: Задачи (post-MVP)

## Phase Contract

Inputs: plan, data-model, spec.
Outputs: задачи для implement с покрытием AC.

## Surface Map

| Surface                                     | Tasks                            |
| ------------------------------------------- | -------------------------------- |
| `src/internal/config/client.go`             | T1.1, T6.1, T7.1                |
| `src/cmd/relay/main.go`                     | T1.2                             |
| `Dockerfile`                                | T1.2                             |
| `src/internal/bootstrap/client/relay.go`    | T1.3 (перенос)                   |
| `src/internal/bootstrap/relay/bridge.go`    | T1.3 (перенос)                   |
| `src/internal/bootstrap/client/client.go`   | T1.3 (удалить relay-ветку)       |
| `src/internal/bootstrap/relay/bootstrap.go` | T2.1, T2.2, T2.3, T7.2, T8.3, T8.5 |
| `src/internal/bootstrap/relay/handler.go`   | T2.1, T5.2, T8.1, T8.2          |
| `src/internal/bootstrap/relay/router.go`    | T3.1, T6.2, T6.3                |
| `src/internal/bootstrap/relay/upstream.go`  | T3.2, T5.1, T5.2, T7.1, T8.2   |
| `src/internal/bootstrap/relay/nat.go`       | T8.1                             |
| `src/internal/bootstrap/client/dial.go`     | T3.2 (reuse)                     |
| `src/internal/session/`                     | T2.1, T2.2 (reuse)               |
| `src/internal/tun/`                         | T2.2 (reuse)                     |
| `src/internal/tunnel/session.go`            | T2.1, T8.4, T8.6                |
| `src/internal/routing/matcher.go`           | T3.1 (reuse)                     |
| `src/internal/routing/domain_matcher.go`    | T3.1 (reuse)                     |
| `src/internal/routing/router.go`            | T3.1 (reuse)                     |
| `examples/relay-terminator/`                | T4.1, T6.4                       |
| `docs/`                                     | T4.2                             |

## Implementation Context

- **Цель**: relay-terminator как split-tunnel VPN gateway с userspace NAT и upstream reconnect.
- **Инварианты**:
  - Relay использует свой `crypto.key` для расшифровки фреймов клиента (как сервер).
  - NAT в userspace: SNAT при отправке, DNAT при получении. Без iptables.
  - Upstream reconnect async, serialised `upstreamMu`.
  - DNS: ВСЕ запросы перехватываются, direct-кэшируются, non-direct без кэша.
  - Bridge-режим (`relay.mode: bridge`) — legacy, не развивается.
- **Контракты/протокол**: frame format, handshake — без изменений.
- **Ошибки**:
  - No upstream: direct traffic works, upstream drops with warn.
  - TUN fail: relay не стартует.
  - DNS intercept fail: пакет идёт upstream (fallback).
  - Timeout: max 10 consecutive, затем session end.

## Фаза 1–7: Реализовано (MVP + P2)

Все задачи T1–T7 выполнены. Подробности в предыдущей версии документа.

- [x] T1.1 — RelayConfig структуры + LoadRelayConfig
- [x] T1.2 — cmd/relay/main.go + Dockerfile target
- [x] T1.3 — Bridge перенос + удаление relay-mode из client
- [x] T2.1 — Bootstrap: WS/QUIC accept, handshake, session, token validation
- [x] T2.2 — TUN setup
- [x] T2.3 — Cleanup at disconnect
- [x] T3.1 — Routing engine: CIDR + domain match + DNS intercept
- [x] T3.2 — Upstream tunnel via dialStream
- [x] T4.1 — examples/relay-terminator/ + docker-compose
- [x] T4.2 — Unit tests + docs
- [x] T5.1 — QUIC upstream dial + fallback
- [x] T5.2 — Upstream transport auto-select
- [x] T6.1 — RelayDNSCfg в конфиге
- [x] T6.2 — DNS interception в routing
- [x] T6.3 — Wire DNS config через initTerminator
- [x] T6.4 — DNS секция в примере relay.yaml
- [x] T7.1 — UpstreamToken в конфиге
- [x] T7.2 — Lazy connect

## Фаза 8: Post-MVP улучшения

Цель: NAT, reconnect, timeout hardening, QUIC обфускация, DNS fixes.

- [x] T8.1 (AC-009) Userspace NAT: добавить `nat.go` с SNAT/DNAT для TCP/UDP/ICMP. SNAT в `routeOutgoing`, DNAT в `receiveLoop`. Touches: `src/internal/bootstrap/relay/nat.go`, `src/internal/bootstrap/relay/handler.go`.
- [x] T8.2 (AC-007) Upstream reconnect: `isClosed()` check + `reconnectUpstream()` async. Serialised by `upstreamMu`. Trigger from `routeOutgoing` on `Send` failure. Touches: `src/internal/bootstrap/relay/handler.go`, `src/internal/bootstrap/relay/upstream.go`, `src/internal/bootstrap/relay/bootstrap.go`.
- [x] T8.3 (RQ-015) ip_forward auto-enable: `sysctl -w net.ipv4.ip_forward=1` в `initTerminator` (non-fatal). Touches: `src/internal/bootstrap/relay/bootstrap.go`.
- [x] T8.4 (RQ-016) Timeout hardening в `wsToTun`: заменить `os.ErrDeadlineExceeded` на `net.Error.Timeout()`. Max 10 consecutive timeout retries. Touches: `src/internal/tunnel/session.go`.
- [x] T8.5 (RQ-003) Server panic fix: `recover()` в `Run()` и errgroup goroutines. Touches: `src/internal/tunnel/session.go`, `src/internal/bootstrap/relay/bootstrap.go`.
- [x] T8.6 (AC-001) receiveLoop context decouple: `Relay.ctx` для upstream session, не зависящий от контекста первого WS-клиента. Touches: `src/internal/bootstrap/relay/bridge.go`, `src/internal/bootstrap/relay/bootstrap.go`, `src/internal/bootstrap/relay/upstream.go`.
- [x] T8.7 (AC-004) QUIC upstream obfuscation: оборачивать upstream QUIC conn в `ObfuscatedQUICConn` при `obfuscation.enabled: true`. Touches: `src/internal/bootstrap/relay/upstream.go`.
- [x] T8.8 (AC-003, AC-008) DNS: все DNS-запросы резолвятся локально. `forwardDNSQuery` принимает `shouldCache` — кэш только для direct. Touches: `src/internal/bootstrap/relay/handler.go`, `src/internal/bootstrap/relay/router.go`.

## Фаза 9: Осталось

Цель: стабильность и IPv6.

- [x] T9.1 (AC-004) Server WS keepalive: добавить `SetKeepalive()` на принятые сервером WS-соединения. Touches: `src/internal/bootstrap/server/handler.go`.
- [x] T9.2 (AC-001) IPv6: документировать `ipv6: false` на клиенте + relay pool_ipv6 конфиг. Touches: `src/internal/bootstrap/relay/bootstrap.go`, `docs/`.
- [x] T9.3 (AC-006) Client-side TUN cleanup: client должен чистить TUN при graceful disconnect. Touches: `src/internal/bootstrap/client/`.

## Покрытие критериев приемки

- AC-001 -> T2.1, T2.2, T4.1, T8.6, T9.2
- AC-002 -> T3.1, T4.1
- AC-003 -> T3.1, T4.1, T6.2, T8.8
- AC-004 -> T3.2, T4.1, T8.7, T9.1
- AC-005 -> T2.2, T8.3
- AC-006 -> T2.3, T4.1, T9.3
- AC-007 -> T8.2
- AC-008 -> T8.8
- AC-009 -> T8.1

## Покрытие требований

- RQ-008 -> T6.1, T6.2, T6.3, T8.8
- RQ-010 -> T1.1, T2.1
- RQ-011 -> T6.1, T6.2, T6.4
- RQ-012 -> T8.1
- RQ-013 -> T8.2
- RQ-014 -> T7.2
- RQ-015 -> T8.3
- RQ-016 -> T8.4
