# Relay-Terminator: Задачи

## Phase Contract

Inputs: plan, data-model, spec.
Outputs: задачи для implement с покрытием AC.
Stop if: задачи расплывчаты — нет, план и spec дают чёткие surfaces.

## Surface Map

| Surface                                     | Tasks                            |
| ------------------------------------------- | -------------------------------- |
| `src/internal/config/client.go`             | T1.1                             |
| `src/cmd/relay/main.go`                     | T1.2                             |
| `Dockerfile`                                | T1.2                             |
| `src/internal/bootstrap/client/relay.go`    | T1.3 (перенос)                   |
| `src/internal/bootstrap/relay/bridge.go`    | T1.3 (перенос)                   |
| `src/internal/bootstrap/client/client.go`   | T1.3 (удалить relay-ветку)       |
| `src/internal/bootstrap/relay/bootstrap.go` | T2.1, T2.2, T2.3 |
| `src/internal/bootstrap/relay/handler.go`   | T2.1               |
| `src/internal/bootstrap/relay/router.go`    | T3.1               |
| `src/internal/bootstrap/relay/upstream.go`  | T3.2               |
| `src/internal/bootstrap/client/dial.go`     | T3.2 (reuse)       |
| `src/internal/session/`                     | T2.1, T2.2 (reuse) |
| `src/internal/tun/`                         | T2.2 (reuse)       |
| `src/internal/routing/matcher.go`           | T3.1 (reuse)       |
| `src/internal/routing/domain_matcher.go`    | T3.1 (reuse)       |
| `src/internal/routing/router.go`            | T3.1 (reuse)       |
| `examples/relay-terminator/`                | T4.1               |
| `docs/`                                     | T4.2               |

## Implementation Context

- **Цель MVP**: отдельный `cmd/relay`-terminator: relay принимает WS/QUIC клиента, поднимает TUN, маршрутизирует direct CIDR + домены → TUN relay, остальное → upstream tunnel.
- **Инварианты**:
  - Relay использует свой `crypto.key` для расшифровки фреймов клиента (как сервер).
  - Upstream соединение — через `dialStream()` с relay-конфигом (obfuscation от relay).
  - Bridge-режим (`mode: relay` в `cmd/client`) не трогается.
  - Domain routing: DNS interception на relay через перехват DNS-запросов из туннеля (как `TunRouter` на клиенте). DomainMatcher из `internal/routing/`.
- **Контракты/протокол**: frame format, handshake — без изменений.
- **Ошибки**:
  - No upstream: клиент получает disconnect, сессия чистится.
  - TUN fail: relay не стартует.
  - Direct route fail: пакет дропается (не шёл upstream).
  - DNS intercept fail: пакет идёт upstream (fallback).
- **Proof signals**:
  - `go build ./src/cmd/relay` успешен.
  - docker-compose: клиент подключается, direct CIDR пингуется, direct domain резолвится и пингуется, external трафик идёт через upstream.
  - Лог relay содержит `route=direct` / `route=upstream` / `dns intercept`.
- **Вне scope**: multi-upstream, bridge-mode изменения.

## Фаза 1: Основа (Config + entrypoint)

Цель: RelayConfig, загрузчик конфига, cmd/relay/main.go, Dockerfile target.

- [x] T1.1 (AC-001, RQ-010) Добавить `RelayConfig` + `RelayTermCfg` структуры в `internal/config/client.go` и загрузчик `LoadRelayConfig()`. Конфиг включает: `relay.mode` (bridge|terminator), `relay.routing.direct_ranges []string`, `relay.routing.direct_domains []string`, `relay.network`, `crypto`, `server`, `transport`, `obfuscation`, `auth.tokens`. Touches: `src/internal/config/client.go`, `src/internal/config/server.go`.
- [x] T1.2 (AC-001) Создать `cmd/relay/main.go` — entrypoint, загружает RelayConfig, вызывает `bootstrap/relay.Run()`. Добавить relay target в `Dockerfile`. Touches: `src/cmd/relay/main.go`, `Dockerfile`, `Dockerfile.test`.
- [x] T1.3 (AC-001) Перенести bridge-режим из `internal/bootstrap/client/relay.go` в `internal/bootstrap/relay/bridge.go`. Переименовать пакет, починить импорты. Удалить `mode: relay` ветку из `client.Run()`. Bridge работает без изменений через `cmd/relay --config relay.yaml` с `relay.mode: bridge`. Touches: `src/internal/bootstrap/client/relay.go`, `src/internal/bootstrap/client/client.go`, `src/internal/bootstrap/relay/bridge.go`, `src/internal/config/client.go`.

## Фаза 2: MVP — серверная часть relay

Цель: relay принимает клиента, handshake, TUN, session, cleanup. AC-001, AC-004, AC-005.

- [x] T2.1 (AC-001, RQ-010) Реализовать `internal/bootstrap/relay/bootstrap.go`: WS (HTTP) + QUIC accept loop (аналогично серверу), handshake (ClientHello → ServerHello), валидация токена клиента через `auth.FindToken`, session creation с IP из pool. Touches: `src/internal/bootstrap/relay/bootstrap.go`, `src/internal/bootstrap/relay/handler.go`, `src/internal/session/`, `src/internal/tunnel/`.
- [x] T2.2 (AC-001, AC-005) TUN setup: relay открывает TUN, настраивает IP из network.pool_ipv4, назначает клиенту IP. Переиспользовать `internal/tun.TunDevice`. Touches: `src/internal/bootstrap/relay/bootstrap.go`, `src/internal/tun/`.
- [x] T2.3 (AC-006) Cleanup at disconnect: освобождение IP, закрытие сессии, удаление маршрутов TUN. Touches: `src/internal/bootstrap/relay/bootstrap.go`, `src/internal/session/`.

## Фаза 3: Routing + upstream

Цель: routing engine + upstream tunnel. AC-002, AC-003.

- [x] T3.1 (AC-002, AC-003) Routing engine: `relay/router.go` — для каждого пакета от клиента: если destination IP в `routing.direct` (CIDR) или domain rule → write в TUN relay (direct), иначе → upstream. DNS interception: перехватывать DNS-запросы из туннеля, извлекать domain, запоминать resolved IP для domain-матчинга. Переиспользовать `CIDRMatcher`, `DomainMatcher`, DNS-логику из `internal/routing/`. Touches: `src/internal/bootstrap/relay/router.go`, `src/internal/routing/matcher.go`, `src/internal/routing/domain_matcher.go`, `src/internal/routing/router.go`.
- [x] T3.2 (AC-004) Upstream tunnel: relay открывает upstream соединение через `dialStream()` к `server`, шифрует non-direct пакеты и отправляет через upstream tunnel. Переиспользовать `Session` из `internal/tunnel`. Touches: `src/internal/bootstrap/relay/upstream.go`, `src/internal/bootstrap/client/dial.go`, `src/internal/tunnel/`.

## Фаза 4: Проверка

Цель: docker-compose пример, документация, тесты.

- [x] T4.1 (AC-001, AC-002, AC-003, AC-004, AC-006) Создать `examples/relay-terminator/` с docker-compose.yml, relay.yaml (c routing.direct), client.yaml. Убедиться: WS и QUIC клиенты подключаются, direct CIDR трафик идёт напрямую, external — через upstream server. Touches: `examples/relay-terminator/`.
- [x] T4.2 (AC-001, AC-002) Добавить unit-тесты для routing engine (CIDR match), config loading. Обновить документацию: `docs/en/relay.md`, `docs/ru/relay.md` — раздел terminator с конфигом. Touches: `src/internal/bootstrap/relay/router_test.go`, `src/internal/config/client_test.go`, `docs/en/relay.md`, `docs/ru/relay.md`.

## Покрытие критериев приемки

- AC-001 -> T2.1, T2.2, T4.1
- AC-002 -> T3.1, T4.1
- AC-003 -> T3.1, T4.1
- AC-004 -> T3.2, T4.1
- AC-005 -> T2.2
- AC-006 -> T2.3, T4.1

## Покрытие требований

- RQ-010 -> T1.1, T2.1

## Заметки

- Все новые функции/методы получают trace-маркеры `@sk-task relay-terminator#Tx.y`.
- Dockerfile.test тоже обновить для CI.
- `internal/bootstrap/client/dial.go` — переиспользовать экспортируемую `DialStream`, без копирования.
