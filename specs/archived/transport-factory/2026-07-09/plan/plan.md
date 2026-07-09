# Transport Factory — План

## Phase Contract

Inputs: spec, inspect (pass), минимальный repo-контекст.
Outputs: plan, data-model stub.
Stop if: intent spec не менялся — все решения приняты.

## Цель

Ввести `TransportFactory` interface в `transport/`, реализовать `WSFactory` и `QUICFactory`, переписать 4 bootstrap-файла (client/relay/server) на фабрику. MVP — только client-side Dial (AC-001–004, AC-007). Data model не меняется.

## MVP Slice

Client-side Dial через TransportFactory. AC-001, AC-002, AC-003, AC-004, AC-007.

## First Validation Path

```bash
go test ./src/internal/transport/... ./src/internal/bootstrap/client/... -v -count=1
```
Проверить что `dial.go` больше не содержит прямых `websocket.Dial`/`quictp.Dial`.

## Scope

- `src/internal/transport/transport.go` — новый interface + `GetFactory` + helper types
- `src/internal/transport/websocket/` — `WSFactory` struct
- `src/internal/transport/quic/` — `QUICFactory` struct
- `src/internal/bootstrap/client/dial.go` — замена `dialStream()` на фабрику
- `src/internal/bootstrap/relay/upstream.go` — замена `dialUpstream()` на фабрику
- `src/internal/bootstrap/relay/bridge.go` — замена `dialRelayUpstream()` на фабрику
- `src/internal/bootstrap/server/handler.go` — не меняется в MVP (deferred AC-006)

Границы: `StreamConn`, `WSConn`, `QUICConn`, `WSConfig`, существующие функции `Dial`/`Accept` — не трогаются.

## Performance Budget

- none: фабрика добавляет 1 interface dispatch + 1 heap alloc на Dial; оба в µs-диапазоне, не влияют на hot path (где доминирует crypto/TLS handshake).

## Implementation Surfaces

| Surface | Роль | Статус |
|---|---|---|
| `src/internal/transport/transport.go` | `TransportFactory` interface, `GetFactory()`, `TransportListener` | новая логика |
| `src/internal/transport/websocket/` | `WSFactory` struct + constructor | новая |
| `src/internal/transport/quic/` | `QUICFactory` struct + constructor | новая |
| `src/internal/bootstrap/client/dial.go` | замена `dialStream()` | изменение |
| `src/internal/bootstrap/relay/upstream.go` | замена `dialUpstream()` | изменение |
| `src/internal/bootstrap/relay/bridge.go` | замена `dialRelayUpstream()` | изменение |

## Bootstrapping Surfaces

`src/internal/transport/transport.go` — первый файл для реализации (интерфейс + GetFactory).

## Влияние на архитектуру

- Локальное: добавление layer of indirection в создании транспортов.
- Никаких изменений StreamConn, wire protocol, config format, persistence.
- Обратная совместимость: старые функции `websocket.Dial`/`quictp.Dial` остаются публичными — фабрика их оборачивает, внешние потребители не ломаются.

## Acceptance Approach

- AC-001: `go build ./src/internal/transport/...` — проверяет компиляцию интерфейса.
- AC-002: unit test в `websocket/` — `WSFactory.Dial` с mock-сервером.
- AC-003: unit test в `quic/` — `QUICFactory.Dial` с mock-сервером.
- AC-004: `go build ./src/internal/bootstrap/client/...` + `grep` на отсутствие `websocket`/`quic` импортов в `dial.go`.
- AC-005: `go build ./src/internal/bootstrap/relay/...` + `grep` на отсутствие прямых вызовов. (MVP deferred — AC существует, но не в MVP).
- AC-006: deferred (server-side во втором цикле).
- AC-007: integration test — factory.Dial с QUIC->WS fallback.

## Данные и контракты

См. `data-model.md` (no-change). Config не расширяется. Новых API/event контрактов нет.

## Стратегия реализации

### DEC-001 Единый интерфейс TransportFactory с Dial + Listen

- Why: spec resolved Q2 — одна фабрика вместо двух. Client использует Dial, server использует Listen+Accept. Меньше сущностей.
- Tradeoff: серверная фабрика для WS должна внутри делать HTTP upgrade — Listen/Accept для WS семантически отличается от QUIC. Фабрика скрывает это.
- Affects: `transport.go`, `websocket/`, `quic/`.
- Validation: WSFactory реализует оба метода; QUICFactory реализует оба метода.

### DEC-002 GetFactory по строке конфига

- Why: bootstrap-код принимает `cfg.Transport` (`""`, `"ws"`, `"quic"`). GetFactory("ws") → WSFactory, GetFactory("quic") → QUICFactory, GetFactory("") → WSFactory (default).
- Tradeoff: жёсткое связывание строк с типами — при добавлении нового транспорта править switch. Приемлемо для 2–3 фабрик; plugin-архитектура — вне scope.
- Affects: `transport.go`.
- Validation: GetFactory("quic") возвращает QUICFactory, GetFactory("") возвращает WSFactory.

### DEC-003 Fallback QUIC→WS через FallbackFactory wrapper

- Why: AC-007 (fallback) — сквозное поведение, не специфичное для QUIC. FallbackFactory оборачивает любую фабрику и при ошибке Dial вызывает резервную.
- Tradeoff: дополнительный wrapper + alloc. Проще, чем встраивать fallback в QUICFactory.
- Affects: `transport.go`.
- Validation: тест AC-007.

### DEC-004 Keepalive внутри фабрики

- Why: spec допущение — bootstrap не вызывает SetKeepalive. Фабрика настраивает keepalive при Dial (для WS) или в конструкторе для QUIC.
- Tradeoff: конфигурация keepalive передаётся в конструктор фабрики — если keepalive понадобится настраивать per-connection, потребуется рефакторинг.
- Affects: конструкторы WSFactory, QUICFactory.
- Validation: после Dial у WSConn вызван SetKeepalive (проверка через mock).

## Incremental Delivery

### MVP (Первая ценность)

- Определить `TransportFactory` interface + `GetFactory` в `transport.go`
- `WSFactory` в `websocket/` + `QUICFactory` в `quic/`
- `FallbackFactory` в `transport.go`
- Заменить `dialStream()` в `bootstrap/client/dial.go`
- Заменить `dialUpstream()` в `bootstrap/relay/upstream.go`
- Заменить `dialRelayUpstream()` в `bootstrap/relay/bridge.go`
- AC: 001, 002, 003, 004, 007

### Итеративное расширение

- Шаг 2: server-side — заменить `handleTunnel()` в `bootstrap/server/handler.go` на `TransportListener`. AC-005, AC-006.

## Порядок реализации

1. `transport.go` — interface + GetFactory (bootstrapping surface)
2. `websocket/wsfactory.go` — WSFactory
3. `quic/quicfactory.go` — QUICFactory
4. `transport.go` — FallbackFactory
5. `bootstrap/client/dial.go` — замена
6. `bootstrap/relay/upstream.go` — замена
7. `bootstrap/relay/bridge.go` — замена
8. Unit-тесты для каждой фабрики + fallback тест

Шаги 2–3 можно параллелить.

## Риски

1. **Server-side Accept через фабрику меняет HTTP-обработчик** — WS Accept требует `http.ResponseWriter`, который не вписывается в `TransportListener.Accept(ctx)`. Mitigation: deferred до второго цикла; в MVP трогаем только Dial.
2. **Fallback затемняет ошибки** — при недоступности обоих транспортов возвращается последняя ошибка (WS). Mitigation: фабрика логирует обе ошибки; добавлено в краевые случаи spec.
3. **GetFactory жёстко зашивает транспорты** — switch вместо registry. Mitigation: осознанное ограничение (out of scope — plugin-архитектура).

## Rollout и compatibility

- Специальных rollout-действий не требуется. Изменения строго additive: старые функции не удаляются.
- После merge: `go build ./...`, `go test -race ./...`.

## Проверка

- Unit-тесты для WSFactory (mock WS server), QUICFactory (mock QUIC server).
- Fallback-тест: закрытый QUIC порт + открытый WS.
- `go vet ./...`, `golangci-lint run ./...`.
- Ручная проверка: сборка клиента в TUN-режиме и Proxy-режиме, подключение к серверу.

## Соответствие конституции

- нет конфликтов.
