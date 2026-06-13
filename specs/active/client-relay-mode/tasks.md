# Client Relay Mode — Задачи

## Phase Contract

Inputs: plan, data-model, spec.
Outputs: исполнимые задачи с покрытием AC.
Stop if: все AC привязаны к задачам; стоп-условий нет.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/config/client.go` | T1.1 |
| `src/internal/bootstrap/client/relay.go` | T2.1, T2.2, T3.1, T3.2 |
| `src/internal/bootstrap/client/client.go` | T2.1 |
| `src/internal/transport/tls/tls.go` | T2.1 (reuse) |
| `src/internal/bootstrap/client/dial.go` | T2.2 (reuse) |
| `configs/client.yaml` | T3.2 |

## Implementation Context

- Цель MVP: WS-only relay — TLS listener + accept + dialStream upstream + прозрачный bridge.
- Инварианты:
  - Relay не интерпретирует фреймы (opaque pipe на уровне `ReadMessage/WriteMessage`).
  - Две bridge-горутины на сессию; `sync.Once` на close.
  - `dialStream()` переиспользуется для upstream — все обфускации (uTLS, padding, SNI, crypto) работают автоматически.
  - Upstream dial timeout = 10s (context.WithTimeout).
  - Если `relay.tls` не указан — автогенерация self-signed cert + WARNING в лог.
- Ошибки:
  - Upstream недоступен → закрыть client-соединение, лог `upstream dial failed, rejecting client`.
  - Client hello timeout (30s) → закрыть client.
  - AuthError от upstream → форвардить клиенту как есть.
- Контракты:
  - Relay слушает TLS на `relay.listen`, принимает WS upgrade на том же path, что указан в `server` URL.
  - ClientHello → upstream → ServerHello → client (forward, без модификации).
- Proof signals:
  - `ss -tlnp` показывает relay listener.
  - В логах `handshake forwarded: session <id>`.
  - tcpdump: клиент не соединяется с upstream напрямую.
- Вне scope: QUIC (P2), chain relay, интерпретация фреймов, session management, изменения сервера.

## Фаза 1: Конфиг и структура

Цель: добавить `RelayCfg` в модель конфига, подготовить surface для relay-режима.

- [x] T1.1 Добавить `RelayCfg` и поле `Relay` в `ClientConfig`
  Touches: `src/internal/config/client.go`
  - Структура `RelayCfg` с полями `Listen string`, `WSPaths []string`, `MaxConnections int`, `TLS *RelayTLSCfg`
  - Структура `RelayTLSCfg` с полями `Cert string`, `Key string`
  - Поле `Relay *RelayCfg` в `ClientConfig` (omitempty)
  - Default `MaxConnections = 100`, `WSPaths = ["/tunnel"]` в `LoadClientConfig()`
  - Валидация: если `Mode == "relay"` и `Relay == nil` → error
  - Trace: `@sk-task client-relay-mode#T1.1` над `RelayCfg` struct, `@sk-task client-relay-mode#T1.1` над `LoadClientConfig` (добавить маркер к существующим)
  - DM-001

## Фаза 2: MVP relay

Цель: relay слушает, принимает WS-соединения, форвардит handshake, bridgит трафик.

- [x] T2.1 Реализовать `runRelayMode` и relay listener
  Touches: `src/internal/bootstrap/client/relay.go` (new), `src/internal/bootstrap/client/client.go`, `src/internal/transport/tls/tls.go`
  - Новый файл `relay.go` в `src/internal/bootstrap/client/`
  - Функция `(c *Client) runRelayMode(ctx context.Context) error`:
    - Генерация TLS config: если `Relay.TLS` указан → `tlspkg.NewServerTLSConfig`, иначе self-signed через `crypto/tls.GenerateCert`
    - `tls.Listen("tcp", cfg.Relay.Listen, tlsCfg)`
    - Accept loop: `listener.Accept()` → проверка WS path по `relay.ws_paths` (404 если не в списке) → `websocket.Accept()` → для каждого — горутина
  - В `client.go:Run()` добавить бранч `case "relay": return c.runRelayMode(ctx)` до остальных mode
  - Trace: `@sk-task client-relay-mode#T2.1` над `runRelayMode`, `@sk-task client-relay-mode#T2.1` над `Client.Run` (добавить к существующим)
  - AC-003 (listener), RQ-001, RQ-002, RQ-007, RQ-008

- [x] T2.2 Реализовать bridge-сессию: handshake forward + data bridge
  Touches: `src/internal/bootstrap/client/relay.go`, `src/internal/bootstrap/client/dial.go`
  - Вторая функция в `relay.go`: `func (c *Client) handleRelayConn(ctx context.Context, clientConn transport.StreamConn)`
  - Читает первый фрейм от client (ClientHello), сохраняет raw bytes
  - `dialStream(ctx, cfg, logger)` → upstream `transport.StreamConn`
  - Forward: `upstreamConn.WriteMessage(clientHelloBytes)`
  - Read: `upstreamConn.ReadMessage()` → ответ (ServerHello или AuthError)
  - Forward: `clientConn.WriteMessage(upstreamResponseBytes)`
  - Bridge loop: две горутины:
    - `copyDirection(client, upstream)` — `ReadMessage` → `WriteMessage`
    - `copyDirection(upstream, client)` — `ReadMessage` → `WriteMessage`
  - `sync.Once` для закрытия обоих соединений при обрыве любого
  - Лог `handshake forwarded: session <id>` после получения ServerHello
  - Trace: `@sk-task client-relay-mode#T2.2` над `handleRelayConn`
  - AC-001, AC-002, AC-005; RQ-002, RQ-003, RQ-004, RQ-005, RQ-006, RQ-010

## Фаза 3: Edge cases и error handling

Цель: обработка отказов upstream, лимитов, таймаутов, double-close.

- [x] T3.1 Добавить обработку ошибок upstream, max_connections, timeout
  Touches: `src/internal/bootstrap/client/relay.go`
  - Upstream dial failure: закрыть client с ошибкой, лог `upstream dial failed, rejecting client`
  - Upstream dial timeout: `context.WithTimeout` 10s
  - ClientHello timeout: `SetReadDeadline` 30s на первый read
  - `max_connections` semaphore: `make(chan struct{}, cfg.Relay.MaxConnections)` — acquire on accept, release on close
  - AuthError от upstream: forward клиенту без интерпретации
  - Trace: `@sk-task client-relay-mode#T3.1` над местом обработки (добавить к `handleRelayConn` или отдельный helper)
  - AC-004; RQ-011

- [x] T3.2 Добавить конфиг example для relay
  Touches: `configs/client.yaml`
  - Закомментированный блок `# relay: ...` с example всех полей
  - Если в `client.yaml` уже есть секция `mode`, добавить комментарий с вариантом `mode: relay`

## Фаза 4: Проверка

Цель: доказать, что relay работает, без утечек и регрессий.

- [x] T4.1 Добавить unit-тесты для `RelayCfg` валидации
  Touches: `src/internal/config/client_test.go`
  - Test: `mode: relay` без RelayCfg → error
  - Test: `mode: relay` + RelayCfg → ok
  - Test: default MaxConnections
  - Trace: `@sk-test client-relay-mode#T4.1` над каждым `Test...`
  - AC-003 (конфиг)

- [x] T4.2 manual validation + lint
  Touches: сборка + ручной сценарий
  - `go build ./src/cmd/client` — компиляция без ошибок
  - `go vet ./src/internal/bootstrap/client/...`
  - `golangci-lint run` (если настроен)
  - Ручной сценарий: relay → upstream → client, проверка handshake + ping + disconnect propagation
  - AC-001, AC-002, AC-004, AC-005

## Покрытие критериев приемки

- AC-001 -> T2.2, T4.2
- AC-002 -> T2.2, T4.2
- AC-003 -> T1.1, T2.1, T4.1
- AC-004 -> T3.1, T4.2
- AC-005 -> T2.2, T4.2

## Заметки

- T2.1 и T2.2 можно реализовать в одном коммите — они составляют единый MVP.
- T3.2 (config example) не блокирует остальные задачи — может выполняться параллельно.
- QUIC relay — P2, не входит в задачи.
- После implement: `/speckeep.verify client-relay-mode`.
