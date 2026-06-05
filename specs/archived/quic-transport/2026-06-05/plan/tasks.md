# QUIC Transport Задачи

## Phase Contract

Inputs: plan `specs/active/quic-transport/plan.md`.
Outputs: упорядоченные задачи с покрытием AC-001–AC-004, DEC-001–DEC-003.
Stop if: не выбран подход к StreamConn interface (один тип vs generic).

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/transport/quic/` | T2.1, T2.2 |
| `src/internal/tunnel/session.go` | T1.1 |
| `src/internal/transport/websocket/` | T1.1 |
| `src/internal/config/` | T1.2 |
| `src/internal/protocol/handshake/` | T1.2 |
| `src/internal/bootstrap/client/` | T3.1, T3.2 |
| `src/cmd/server/` | T2.2 |
| `src/internal/tests/` | T4.1 |

## Implementation Context

- Цель MVP: QUIC stream заменяет WebSocket, throughput >5 Mbps на 60ms/1% loss
- Границы приемки: AC-001, AC-002, AC-003, AC-004
- Ключевые правила: `quic-go` фиксированной версии; trace-маркеры `@sk-task` на новых объявлениях; Dockerfile → EXPOSE 443/udp
- Инварианты: все существующие тесты проходят; TCP-транспорт не меняется
- Контракты: StreamConn interface {ReadMessage, WriteMessage, Close}; Config.Transport string
- Proof signals: iperf >5 Mbps на плохом канале; fallback TCP при блокировке UDP
- Вне scope: proxy frames поверх QUIC; multi-stream; 0-RTT reconnect

## Фаза 1: Подготовка

Цель: подготовить интерфейс для абстракции транспорта и добавить поле конфига.

- [x] T1.1 Выделить интерфейс `StreamConn` в `src/internal/tunnel/` с методами `ReadMessage() ([]byte, error)`, `WriteMessage([]byte) error`, `Close() error`. Переименовать существующий `*websocket.WSConn` usage в интерфейс. Touches: `src/internal/tunnel/session.go`, `src/internal/transport/websocket/conn.go`
- [x] T1.2 Добавить поле `Transport string` (значения: `tcp`, `quic`) в `config.ClientConfig` и `config.ServerConfig` (если есть). ClientHello/ServerHello — добавить поле Transport для согласования. Touches: `src/internal/config/client.go`, `src/internal/config/server.go`, `src/internal/protocol/handshake/`

## Фаза 2: QUIC transport

Цель: ядро QUIC — dial, listen, conn wrapper.

- [x] T2.1 Создать `src/internal/transport/quic/conn.go` — обёртка `QUICConn` над `quic-go` stream, реализующая `StreamConn`. Touches: `src/internal/transport/quic/conn.go`
- [x] T2.2 Создать `src/internal/transport/quic/listen.go` — QUIC listener: `Listen(addr, tlsConfig) (*QUICListener, error)`, accept возвращает `*QUICConn`. Интегрировать в `cmd/server/` — параллельный quic listener. Touches: `src/internal/transport/quic/listen.go`, `src/cmd/server/main.go`

## Фаза 3: Интеграция клиента

Цель: клиент выбирает транспорт, fallback при недоступности UDP.

- [x] T3.1 В `bootstrap/client/client.go` — выбор транспорта при dial: если `transport: quic` — `quic.Dial` вместо `websocket.Dial`. Touches: `src/internal/bootstrap/client/client.go`, `src/internal/bootstrap/client/tun.go`
- [x] T3.2 Реализовать fallback: при timeout/ошибке QUIC dial → пробуем TCP/WebSocket с логом. Touches: `src/internal/bootstrap/client/client.go`

## Фаза 4: Проверка

Цель: доказать производительность и совместимость.

- [x] T4.1 Написать integration test: поднять сервер c QUIC, клиент c QUIC, измерить throughput с `tc netem delay 60ms loss 1%`. Touches: `src/internal/transport/quic/quic_test.go`
- [x] T4.2 Проверить обратную совместимость: старый клиент (без transport) → TCP/WS, новый клиент → QUIC. Touches: `src/internal/tests/quic_transport_test.go`

## Покрытие критериев приемки

- AC-001 -> T1.1, T2.1, T2.2, T3.1, T4.1
- AC-002 -> T3.2, T4.1
- AC-003 -> T4.1
- AC-004 -> T4.2

## Покрытие решений

- DEC-001 (один stream) -> T2.1, T3.1
- DEC-002 (StreamConn interface) -> T1.1
- DEC-003 (fallback) -> T3.2

## Заметки

- T1.1 и T1.2 независимы, можно параллелить
- T2.1 и T2.2 зависят от T1.1
- T3.1 и T3.2 зависят от T2.1 и T2.2
- T4.1 и T4.2 — финальная валидация
- Перед T4.1: `go get github.com/quic-go/quic-go@v0.50.0`
