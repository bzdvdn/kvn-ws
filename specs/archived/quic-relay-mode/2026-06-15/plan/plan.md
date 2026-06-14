# QUIC Relay Mode — План

## Phase Contract

Inputs: spec + inspect (pass).
Outputs: plan, data-model.
Stop if: неоднозначностей нет.

## Цель

Добавить в relay mode QUIC listener (UDP) параллельно с существующим WS (TCP/TLS) listener-ом — аналогично dual-transport pattern сервера. Relay принимает клиентов по обоим транспортам, bridge-логика единая через StreamConn.

## MVP Slice

- WS listener (всегда) + QUIC listener (опционально, по `relay.quic`) + accept + bridge loop.
- Закрывает: AC-001 (оба транспорта), AC-002 (data bridge), AC-003 (конфиг), AC-004 (opaque), AC-005 (reject).

## First Validation Path

1. Собрать relay: `go build -o bin/relay ./src/cmd/client`
2. Запустить upstream-сервер + relay с `relay.quic: {}`.
3. Подключиться WS-клиентом (`wss://relay:443/tunnel`) — handshake OK.
4. Подключиться QUIC-клиентом (`quic://relay:443`) — handshake OK.
5. Проверить: `ss -tlnp` и `ss -ulnp` показывают оба listener-а.

## Scope

- QUIC listener в `runRelayMode` через errgroup (WS listener всегда + QUIC опционально).
- Новый блок `RelayQuicCfg` в конфиге с полями `KeepAlive`, `IdleTimeout`.
- Общий semaphore `max_connections` для обоих транспортов.
- Единая bridge-функция для WS и QUIC (через StreamConn).
- `examples/relay/relay.yaml` — QUIC example.

## Implementation Surfaces

| Surface | Почему | Статус |
|---|---|---|
| `src/internal/config/client.go` | добавить `RelayQuicCfg`, поле `Quic` в `RelayCfg` | existing, change |
| `src/internal/bootstrap/client/relay.go` | refactor `runRelayMode` на errgroup + QUIC accept loop | existing, change |
| `src/internal/transport/quic/listen.go` | QUIC listener — re-use | existing, reuse |
| `src/internal/transport/quic/conn.go` | `QUICConn` + `StreamConn` interface — re-use | existing, reuse |
| `src/internal/bootstrap/client/dial.go` | `dialStream()` для upstream — re-use | existing, reuse |
| `examples/relay/relay.yaml` | QUIC config example | existing, change |

## Bootstrapping Surfaces

- `none` — relay.go существует, quic transport существует, config существует.

## Влияние на архитектуру

- `runRelayMode` переходит от single listener (TLS only) к dual listener pattern через errgroup.
- bridge-логика не меняется — QUIC `QUICConn` реализует тот же `StreamConn`, что и WS.
- Semaphore поднимается из HTTP handler на уровень `runRelayMode` (общий для обоих listener-ов).
- WS relay path allowlist (`relay.ws_paths`) применяется только к WS-соединениям — QUIC не имеет URL-путей.

## Acceptance Approach

| AC | Подход | Surfaces | Наблюдение |
|---|---|---|---|
| AC-001 | errgroup: TLS listener + QUIC listener | `relay.go` | `ss -tlnp` + `ss -ulnp`; лог `relay listening` / `quic relay listening` |
| AC-002 | Обе bridge-горутины через StreamConn | `relay.go` bridge loop | tcpdump; `ss` после закрытия |
| AC-003 | Config struct + defaults | `config/client.go`, `relay.go` | listener-ы на ожидаемых портах |
| AC-004 | ReadMessage → WriteMessage opaque | `relay.go` copyDirection | relay не логирует `unknown frame` |
| AC-005 | Upstream failure → close client | `relay.go` bridge | лог `upstream dial failed` |

## Данные и контракты

- Data model: добавляется `RelayQuicCfg` — см. `data-model.md`.
- Контракты не меняются — relay работает на уровне StreamConn (opaque pipe).
- WS path allowlist не применяется к QUIC-соединениям (QUIC не имеет HTTP-путей).

## Стратегия реализации

### DEC-001 Dual listener через errgroup

- Why: сервер уже использует этот паттерн — доказанная надёжность. WS listener (HTTP.Serve) в одной горутине, QUIC accept loop в другой.
- Tradeoff: relay теперь всегда имеет WS listener (даже если оператор планирует только QUIC). Это минимальный оверхед (один TLS listener).
- Affects: `relay.go` — `runRelayMode` переходит на errgroup.
- Validation: AC-001 — оба listener-а активны.

### DEC-002 Единая bridge-функция для WS и QUIC

- Why: `StreamConn` interface един для обоих транспортов. `bridgeRelayConn` уже работает с `transport.StreamConn` — WS и QUIC `QUICConn` реализуют этот интерфейс.
- Tradeoff: нет — zero-cost абстракция.
- Affects: `bridgeRelayConn` остаётся без изменений; QUIC accept вызывает её же.
- Validation: AC-002 — data bridge для QUIC.

### DEC-003 Глобальный semaphore вместо per-handler

- Why: лимит `max_connections` должен применяться к обоим транспортам. Сейчас semaphore живёт в HTTP handler — для QUIC он должен быть на уровне `runRelayMode`.
- Tradeoff: рефакторинг WS части — перенос semaphore из handler наружу.
- Affects: `relay.go` — `runRelayMode` создаёт semaphore, передаёт в handler и QUIC accept.
- Validation: AC-003 — `max_connections` работает для обоих транспортов.

### DEC-004 Без path allowlist для QUIC

- Why: QUIC не имеет URL-путей — нет HTTP, нет ServeMux, нет path filter.
- Tradeoff: QUIC-клиенты проходят без проверки пути. В MVP это приемлемо — auth выполняется на upstream через ClientHello token.
- Affects: `relay.go` — QUIC accept не вызывает `allowedRelayPath`.
- Validation: AC-001 — QUIC-клиент подключается без указания пути.

## Incremental Delivery

### MVP (Первая ценность)

- WS listener (всегда) + QUIC listener (опционально) + accept + bridge.
- AC-001, AC-002, AC-003, AC-004, AC-005.
- Validation: manual test — WS и QUIC клиенты через relay.

### Итеративное расширение

- P2: ObfuscatedQUICConn (XOR) для incoming QUIC — обёртка после accept.
- P3: source IP allowlist для QUIC (по аналогии с `ws_paths` для WS).

## Порядок реализации

1. Config: `RelayQuicCfg` struct + defaults + validation.
2. `relay.go`: рефакторинг `runRelayMode` на errgroup (WS listener в errgroup goroutine).
3. `relay.go`: добавление QUIC accept loop во вторую errgroup goroutine.
4. `relay.go`: перенос semaphore на уровень `runRelayMode` — общий для WS и QUIC.
5. `examples/relay/relay.yaml`: QUIC config example.

Шаги 2-3-4 можно выполнять в одном коммите (единый рефакторинг relay.go).

## Риски

- **Double-close при одновременном обрыве**: как и в WS relay — `sync.Once` + отложенный вызов `closeBoth`.
- **QUIC idle timeout**: если клиент не шлёт данные, QUIC-соединение может закрыться по idle timeout раньше, чем ClientHello timeout. Mitigation: настроить `relay.quic.idle_timeout` с запасом (>30s).
- **UDP port conflict**: relay слушает UDP на том же адресе, что и TCP. Если ОС блокирует сосуществование (редко для разных протоколов) — relay не стартует. Mitigation: логировать ошибку при `quic.Listen`.
- **Semaphore race**: два listener-а конкурентно принимают соединения и пытаются захватить semaphore. Mitigation: channel-based semaphore thread-safe по природе.

## Rollout and compatibility

- `mode: relay` без `relay.quic` — поведение не меняется (только WS listener).
- `mode: relay` с `relay.quic` — добавляется QUIC listener.
- Обратная совместимость полная: старые конфиги без `relay.quic` работают как раньше.

## Проверка

- `go test ./src/internal/config/...` — тесты нового конфига.
- `go vet ./src/internal/bootstrap/client/...` — статический анализ.
- `go build ./src/cmd/client` — компиляция.
- Ручной сценарий: relay → upstream; WS-клиент + QUIC-клиент.
- AC-001..AC-005 покрываются manual validation path.

## Соответствие конституции

- Нет конфликтов: feature branch, traceability, clean architecture, языковая политика соблюдены.
