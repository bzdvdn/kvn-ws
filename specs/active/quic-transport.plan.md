# QUIC Transport План

## Phase Contract

Inputs: spec `quic-transport.md`, кодовая база kvn-ws.
Outputs: план миграции с WebSocket (TCP) на QUIC (UDP).
Stop if: непонятен механизм fallback TCP или требования к портам.

## Цель

Заменить WebSocket на QUIC как основной транспорт туннеля с сохранением обратной совместимости. Результат: 3-10x прирост скорости на плохих каналах + улучшенная маскировка трафика.

## MVP Slice

Один QUIC stream на сессию (аналог текущего WS соединения), data frames только для TUN mode. Proxy-frames отложены.

## First Validation Path

1. `make integration-test` с двумя хостами, `tc` эмулирует 60ms + 1% loss
2. iperf через туннель показывает >5 Mbps

## Scope

- `src/internal/transport/quic/` — новый пакет (dial/accept/listen)
- `src/internal/config/` — поле Transport {tcp, quic}
- `src/internal/protocol/handshake/` — транспорт в Hello
- `src/internal/bootstrap/client/` — выбор транспорта
- `src/cmd/server/` — QUIC listener
- `src/internal/tunnel/` — абстракция stream interface
- `src/internal/transport/websocket/` — без изменений (остаётся fallback)

## Implementation Surfaces

| Surface | Роль |
|---------|------|
| `src/internal/transport/quic/` | **Новый пакет**: `dial.go`, `listen.go`, `conn.go` — обёртка над quic-go |
| `src/internal/config/config.go` | Добавить `Transport string` в shared config |
| `src/internal/protocol/handshake/` | ClientHello: поле `Transport`; ServerHello: подтверждение |
| `src/internal/bootstrap/client/` | `client.go`: выбор транспорта при dial |
| `src/cmd/server/main.go` | `server.go`: QUIC listener рядом с TCP |
| `src/internal/tunnel/session.go` | Заменить `*websocket.WSConn` на интерфейс `StreamConn` |

## Bootstrapping Surfaces

`src/internal/transport/quic/` — новая директория.

## Влияние на архитектуру

- **Текущая:** `tunnel.Session` принимает `*websocket.WSConn` (конкретный тип).
- **Цель:** `tunnel.Session` принимает интерфейс:
  ```go
  type StreamConn interface {
      ReadMessage() ([]byte, error)
      WriteMessage([]byte) error
      Close() error
  }
  ```
  `*websocket.WSConn` уже реализует этот интерфейс. Новый `*quic.Conn` — тоже.
- QUIC listener на сервере — параллельный accept на UDP 443.
- Конфиг: новое поле `transport: quic|tcp`.

## Acceptance Approach

- AC-001: запустить сервер с `transport: quic`, клиент с `transport: quic`, проверить handshake и iperf
- AC-002: заблокировать UDP (iptables -A OUTPUT -p udp --dport 443 -j DROP), проверить fallback и iperf через WS
- AC-003: `tc qdisc add dev eth0 root netem delay 60ms loss 1%`, iperf >5 Mbps
- AC-004: старый клиент (без `transport`) → соединение через WS

## Данные и контракты

- **Новые:** `src/internal/transport/quic/` пакет, интерфейс `StreamConn`
- **Изменённые:** `tunnel.Session` — замена конкретного типа на интерфейс
- **Конфиг:** `transport: tcp|quic` (default: tcp — обратная совместимость)

## Стратегия реализации

### DEC-001 Один QUIC stream на сессию

- Why: минимальное изменение — current WS модель (один stream) просто перекладывается на QUIC. Не надо менять framing, handshake, proxy.
- Tradeoff: не используем multi-stream фичи QUIC (MPTCP). Можно добавить позже.
- Affects: quic/conn.go, tunnel/session.go
- Validation: iperf показывает выигрыш

### DEC-002 StreamConn interface

- Why: tunnel.Session не должен знать о транспорте. Выделяем интерфейс для ReadMessage/WriteMessage/Close.
- Tradeoff: Go interface overhead минимален
- Affects: tunnel/session.go, websocket/conn.go

### DEC-003 Fallback: dial timeout → TCP

- Why: UDP может быть заблокирован файрволом. Клиент пробует QUIC, при timeout > 5s переключается на TCP.
- Tradeoff: добавляет latency при недоступности QUIC
- Affects: bootstrap/client/client.go

## Incremental Delivery

### MVP (Первая ценность)

1. StreamConn interface + quic conn wrapper
2. QUIC listener на сервере
3. Клиент с выбором транспорта
4. Fallback TCP

### Итеративное расширение

5. Proxy frames поверх QUIC streams
6. Multiplexing (отдельные QUIC streams для data/control)
7. 0-RTT reconnect

## Порядок реализации

1. `StreamConn` interface + `quic/conn.go` — фундамент
2. `quic/listen.go` + интеграция в `cmd/server/` — сервер
3. `quic/dial.go` + выбор транспорта в `bootstrap/client/` — клиент
4. Fallback TCP при недоступности QUIC
5. Тесты (unit + integration с `tc`)

## Риски

- Риск: quic-go API breaking changes — mitigation: фиксируем версию в go.mod
- Риск: UDP заблокирован на хостинге — mitigation: fallback TCP обязателен
- Риск: производительность QUIC ниже ожидаемой — mitigation: benchmark до merge

## Rollout и compatibility

- `transport: tcp` по умолчанию — старые клиенты не меняют поведение
- Параллельный TCP/UDP listener на сервере
- Без флага `transport` в конфиге — транспорт = tcp

## Проверка

- `go test ./src/internal/transport/quic/...` — unit
- Integration: два хоста с `tc netem`, iperf замер
- `go vet ./...` + `golangci-lint`

## Соответствие конституции

- нет конфликтов
- Trace-маркеры обязательны в новых объявлениях
- Docker: QUIC требует UDP, обновить Dockerfile (EXPOSE 443/udp)
