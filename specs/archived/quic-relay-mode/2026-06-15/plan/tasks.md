# QUIC Relay Mode — Задачи

## Phase Contract

Inputs: plan, data-model, spec.
Outputs: исполнимые задачи с покрытием AC.
Stop if: все AC привязаны к задачам; стоп-условий нет.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/config/client.go` | T1.1 |
| `src/internal/bootstrap/client/relay.go` | T2.1, T3.1 |
| `src/internal/transport/quic/listen.go` | T3.1 (reuse) |
| `examples/relay/relay.yaml` | T4.2 |

## Implementation Context

- Цель MVP: WS listener (всегда) + QUIC listener (опционально, по `relay.quic`) — через errgroup, bridge loop общий через StreamConn.
- Инварианты:
  - WS listener всегда активен; QUIC listener только при `RelayCfg.Quic != nil`.
  - `bridgeRelayConn` принимает `transport.StreamConn` — WS и QUIC `QUICConn` реализуют этот интерфейс.
  - Semaphore (`max_connections`) общий для обоих транспортов — живёт на уровне `runRelayMode`.
  - ClientHello timeout 30s на первом `ReadMessage()`.
  - Upstream dial timeout 10s (через `dialRelayUpstream`).
- Ошибки:
  - QUIC listener fail (UDP port занят) → логировать и выйти из errgroup (весь relay падает).
  - Upstream dial fail → закрыть клиента, лог `upstream dial failed, rejecting client`.
  - ClientHello timeout → закрыть клиента.
- Контракты:
  - WS: HTTP→WS upgrade, path allowlist (`relay.ws_paths`).
  - QUIC: `quic.Listen` + `AcceptStream`, без path filter.
  - QUIC config: `relay.quic.keep_alive` (default 7s), `relay.quic.idle_timeout` (default 60s).
- Proof signals:
  - `ss -tlnp | grep <port>` — WS listener.
  - `ss -ulnp | grep <port>` — QUIC listener.
  - Лог `handshake forwarded: session <id>` для обоих транспортов.
- Вне scope: ObfuscatedQUICConn (P2), multi-stream, chain relay, mTLS для QUIC.
- References: DEC-001 (errgroup), DEC-003 (global semaphore), DM-001 (RelayQuicCfg).

## Фаза 1: Конфиг

Цель: добавить `RelayQuicCfg` в модель конфига.

- [x] T1.1 Добавить `RelayQuicCfg` и поле `Quic` в `RelayCfg`
  Touches: `src/internal/config/client.go`
  - Структура `RelayQuicCfg` с полями `KeepAlive time.Duration`, `IdleTimeout time.Duration`
  - Поле `Quic *RelayQuicCfg` в `RelayCfg` (omitempty)
  - Default `KeepAlive = 7s`, `IdleTimeout = 60s` в `LoadClientConfig()` при `Relay.Quic != nil`
  - Валидация: если `Relay.Quic != nil` и `IdleTimeout <= 0` → error
  - Trace: `@sk-task quic-relay-mode#T1.1` над `RelayQuicCfg` struct, `@sk-task quic-relay-mode#T1.1` над `LoadClientConfig`
  - DM-001

## Фаза 2: Dual-listener рефакторинг

Цель: перевести `runRelayMode` на errgroup с общим semaphore.

- [x] T2.1 Перевести `runRelayMode` на errgroup с WS listener в одной горутине и semaphore на уровне relay
  Touches: `src/internal/bootstrap/client/relay.go`
  - Создать `errgroup` через `errgroup.Group` (из `golang.org/x/sync/errgroup`) или ручной канал ошибок
  - Semaphore (`chan struct{}`) создаётся в `runRelayMode`, передаётся в WS handler и QUIC accept
  - WS listener (HTTP.Serve) запускается в первой errgroup goroutine
  - Добавить WS handler как отдельный метод (или замыкание) с доступом к semaphore
  - Shutdown по ctx.Done через HTTP server shutdown
  - Trace: `@sk-task quic-relay-mode#T2.1` над `runRelayMode`
  - DEC-001, DEC-003; RQ-001, RQ-003, RQ-010

## Фаза 3: QUIC accept + bridge

Цель: добавить QUIC listener accept loop и интегрировать с bridge.

- [x] T3.1 Добавить QUIC listener и accept loop
  Touches: `src/internal/bootstrap/client/relay.go`, `src/internal/transport/quic/listen.go`
  - Если `c.cfg.Relay.Quic != nil`: создать `quic.Config{KeepAlivePeriod, MaxIdleTimeout}` из relay конфига
  - `quictp.Listen(c.cfg.Relay.Listen, tlsCfg, quicCfg)` — QUIC listener
  - Вторая errgroup goroutine: `for { quicConn, err := quicListener.Accept(ctx) ... }`
  - На каждый accept: захват semaphore, затем `c.bridgeRelayConn(ctx, quicConn, remoteAddr)` в горутине
  - После завершения bridge — release semaphore
  - WS handler аналогично использует тот же semaphore из T2.1
  - Лог `quic relay listening` при старте QUIC listener
  - Trace: `@sk-task quic-relay-mode#T3.1` над местом QUIC accept (новый метод или блок в `runRelayMode`)
  - DEC-002, DEC-004; RQ-002, RQ-005, RQ-009

## Фаза 4: Проверка

Цель: unit-тесты конфига, обновление примеров, сборка.

- [x] T4.1 Добавить unit-тесты для `RelayQuicCfg` валидации
  Touches: `src/internal/config/client_test.go`
  - Test: `relay.quic` с валидным IdleTimeout → ok
  - Test: `relay.quic` с IdleTimeout=0 → error
  - Test: `relay.quic` defaults (KeepAlive, IdleTimeout)
  - Trace: `@sk-test quic-relay-mode#T4.1` над каждым `Test...`
  - AC-003

- [x] T4.2 Обновить примеры и проверка сборки
  Touches: `examples/relay/relay.yaml`
  - Добавить блок `relay.quic` в `examples/relay/relay.yaml`
  - `go build ./src/cmd/client` — компиляция без ошибок
  - `go vet ./src/internal/bootstrap/client/...`
  - Ручной сценарий: relay → upstream; WS-клиент + QUIC-клиент
  - AC-001, AC-002, AC-003, AC-004, AC-005

## Покрытие критериев приемки

- AC-001 -> T2.1, T3.1, T4.2
- AC-002 -> T3.1, T4.2
- AC-003 -> T1.1, T2.1, T4.1
- AC-004 -> T3.1, T4.2
- AC-005 -> T3.1, T4.2

## Заметки

- T2.1 и T3.1 можно реализовать в одном коммите — рефакторинг `runRelayMode` + QUIC accept.
- T1.1 (config) может выполняться параллельно с T2.1 (зависимость только от структуры, не от config loading).
- После implement: `/speckeep.verify quic-relay-mode`.
