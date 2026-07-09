# Transport Factory — Задачи

## Phase Contract

Inputs: plan, data-model, spec.
Outputs: упорядоченные исполнимые задачи с покрытием критериев.
Stop if: — все AC привязаны к задачам.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/transport/transport.go` | T1.1, T2.1 |
| `src/internal/transport/websocket/wsfactory.go` | T1.2 |
| `src/internal/transport/quic/quicfactory.go` | T1.3 |
| `src/internal/bootstrap/client/dial.go` | T2.2 |
| `src/internal/bootstrap/relay/upstream.go` | T2.3 |
| `src/internal/bootstrap/relay/bridge.go` | T2.4 |

## Implementation Context

- Цель MVP: ввести TransportFactory interface, WSFactory + QUICFactory, заменить Dial в bootstrap/client и bootstrap/relay на фабрику. AC-001,002,003,004,007.
- Инварианты/семантика:
  - TransportFactory — один interface с Dial(ctx, endpoint) StreamConn + Listen(ctx, addr) TransportListener
  - GetFactory(transport string) возвращает WSFactory по умолчанию, QUICFactory для "quic"
  - FallbackFactory оборачивает две фабрики: при ошибке Primary.Dial вызывает Secondary.Dial
  - Keepalive настраивается внутри фабрики (передаётся в конструктор), bootstrap не вызывает SetKeepalive
- Ошибки/коды:
  - Dial возвращает (nil, error) при недоступности всех транспортов
  - Fallback логирует ошибку primary-фабрики, возвращает ошибку secondary, если и та недоступна
- Контракты/протокол:
  - Существующие функции websocket.Dial/Accept, quictp.Dial/Listen остаются — фабрика их оборачивает
- Границы scope:
  - НЕ меняем StreamConn, WSConn, QUICConn, WSConfig
  - НЕ трогаем bootstrap/server/handler.go (deferred AC-006)
  - НЕ трогаем relay server-side Accept (deferred AC-005, шаг 2)
- Proof signals:
  - `go build ./src/internal/transport/...` — компиляция
  - `go build ./src/internal/bootstrap/client/...` — без websocket/quic импортов в dial.go
  - `go build ./src/internal/bootstrap/relay/...` — без websocket/quic импортов в upstream.go
  - unit-тесты WSFactory.Dial, QUICFactory.Dial, FallbackFactory.Dial
- References: DEC-001 (единый интерфейс), DEC-002 (GetFactory), DEC-003 (FallbackFactory), DEC-004 (keepalive)

## Фаза 1: Основа — интерфейсы и фабрики

Цель: определить TransportFactory/TransportListener interface, GetFactory, реализации WSFactory и QUICFactory.

- [x] T1.1 Определить `TransportFactory` и `TransportListener` interface + `Register()`/`NewFactory()` в `transport.go`. Factory имеет методы `Dial(ctx, endpoint)` и `Listen(ctx, addr)`. NewFactory принимает строку транспорта, возвращает WSFactory для `""`/`"ws"`, QUICFactory для `"quic"`. Touches: `src/internal/transport/transport.go`

- [x] T1.2 Реализовать `WSFactory` struct в `websocket/wsfactory.go`. Конструктор принимает FactoryConfig, keepalive interval/timeout. Dial вызывает websocket.Dial + SetKeepalive. Listen/Accept — заглушка для MVP. Touches: `src/internal/transport/websocket/wsfactory.go`

- [x] T1.3 Реализовать `QUICFactory` struct в `quic/quicfactory.go`. Конструктор принимает FactoryConfig. Dial вызывает quictp.Dial + опционально NewObfuscatedQUICConn. Listen/Accept — заглушка для MVP. Touches: `src/internal/transport/quic/quicfactory.go`

- [x] T2.1 Реализовать `FallbackFactory` struct в `transport.go`. Оборачивает primary и secondary TransportFactory. При ошибке primary.Dial логирует и вызывает secondary.Dial. Touches: `src/internal/transport/transport.go`

- [x] T2.2 Заменить `dialStream()` в `bootstrap/client/dial.go`: использовать `NewFactory(cfg.Transport, factoryCfg).Dial(...)`. Убрать прямые импорты `websocket`/`quic` из dial.go. Touches: `src/internal/bootstrap/client/dial.go`

- [x] T2.3 Заменить `dialUpstream()` и связанные функции в `bootstrap/relay/upstream.go` на фабрику. Убрать прямые импорты `websocket`/`quic` для Dial. Удалить `dialQUICUpstream`. Touches: `src/internal/bootstrap/relay/upstream.go`

- [x] T2.4 Заменить `dialRelayUpstream()` в `bootstrap/relay/bridge.go` на фабрику. Убрать прямые вызовы `websocket.Dial`. Touches: `src/internal/bootstrap/relay/bridge.go`

- [x] T3.1 Написать unit-тесты для `WSFactory.Dial` (с httptest WS server), `QUICFactory.Dial` (с QUIC echo server). Проверить Receive/Send. Touches: `src/internal/transport/websocket/wsfactory_test.go`, `src/internal/transport/quic/quicfactory_test.go`

- [x] T3.2 Написать unit-тест для `FallbackFactory.Dial` — primary недоступен, secondary успешен. Touches: `src/internal/transport/transport_test.go`

- [x] T3.3 Проверить: `go build ./...`, `go vet ./...`, `golangci-lint run ./...` — без ошибок. Touches: `(CI)`

- [x] T0.1 Задокументировать AC-006 как deferred — server-side Accept не входит в MVP. Touches: `spec.md`

## Покрытие критериев приемки

- AC-001 -> T1.1, T3.3
- AC-002 -> T1.2, T3.1
- AC-003 -> T1.3, T3.1
- AC-004 -> T2.2, T3.3
- AC-005 -> T2.3, T2.4
- AC-006 -> T0.1
- AC-007 -> T2.1, T3.2

## Заметки

- T1.2 и T1.3 можно параллелить
- T2.2–T2.4 зависят от T1.1–T1.3 (нужен interface + фабрики)
- После T3.3: `go test -race ./src/internal/transport/... ./src/internal/bootstrap/client/... ./src/internal/bootstrap/relay/...`
- Trace-маркеры `@sk-task` размещать над объявлениями типов/функций (не package/import/file-header)
