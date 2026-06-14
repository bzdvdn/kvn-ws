# Client Relay Mode — План

## Phase Contract

Inputs: spec + inspect (pass).
Outputs: plan, data-model.
Stop if: неоднозначностей нет, продолжаем.

## Цель

Добавить `mode: "relay"` в Go-клиент. Relay принимает входящие WS-соединения от KVN-клиентов, открывает upstream-соединение к серверу через `dialStream()` и работает как прозрачный bridge до закрытия любого из двух плеч. Ни TUN, ни NAT, ни session management на relay не запускаются.

## MVP Slice

- WS-only relay (TCP/TLS listener + WS accept + upstream dial + bridge loop).
- Закрывает: AC-001 (forward handshake), AC-002 (data bridge + disconnect), AC-003 (конфиг), AC-004 (reject on upstream failure), AC-005 (opaque bridge).

## First Validation Path

1. Собрать релей: `go build -o bin/relay ./src/cmd/client`
2. Запустить upstream-сервер.
3. Запустить relay с `mode: relay`, `server: wss://upstream/tunnel`.
4. Подключиться клиентом к relay (указав `server: wss://relay-ip/tunnel`).
5. Проверить: `tcpdump` — клиент не соединяется с upstream напрямую; PING/PONG проходят; сессия создаётся на upstream.

## Scope

- Новые поля в `ClientConfig` — блок `RelayCfg` (listen, tls, max_connections)
- Новый бранч в `Client.Run()` — `runRelayMode(ctx)`
- Новый файл `src/internal/bootstrap/client/relay.go` — TLS listener, accept loop, bridge per connection
- Переиспользование `dialStream()` и `tls.Config` (серверная часть из `tlspkg`)
- `configs/client.yaml` — relay config example

## Implementation Surfaces

| Surface | Почему | Статус |
|---|---|---|
| `src/internal/config/client.go` | добавить `RelayCfg` (включая `ws_paths`) + `Relay *RelayCfg` в `ClientConfig` | existing, change |
| `src/internal/bootstrap/client/client.go` | бранч `mode == "relay"` в `Run()` | existing, change |
| `src/internal/bootstrap/client/relay.go` | TLS listener, WS path allowlist, accept loop, bridge logic | new |
| `src/internal/transport/tls/tls.go` | `NewServerTLSConfig` для listener | existing, reuse |
| `src/internal/bootstrap/client/dial.go` | `dialStream()` для upstream | existing, reuse |
| `configs/client.yaml` | relay config block как комментарий | existing, change |

## Bootstrapping Surfaces

- `src/internal/bootstrap/client/relay.go` — новый файл; всё остальное уже существует.

## Влияние на архитектуру

- Локально: третий mode в клиенте, изолированный от TUN и proxy mode.
- Relay не затрагивает server, не меняет протокол, не требует миграций.
- Все обфускации (uTLS, padding, SNI, crypto) работают через делегирование `dialStream()`.

## Acceptance Approach

| AC | Подход | Surfaces | Наблюдение |
|---|---|---|---|
| AC-001 | Forward handshake: accept → dial upstream → forward ClientHello → forward ServerHello | `relay.go` listener + bridge | лог `handshake forwarded`; upstream видит IP relay |
| AC-002 | Data bridge + disconnect: двунаправленное копирование; обрыв любого плеча закрывает оба | `relay.go` bridge loop | tcpdump; `ss` после закрытия |
| AC-003 | Конфиг: `relay.listen`, `relay.ws_paths`, `relay.tls`, `relay.max_connections` | `config/client.go`, `relay.go` | `ss -tlnp`; лог `listening on` |
| AC-004 | Reject on upstream failure: upstream недоступен → клиент получает ошибку | `relay.go` dial + cleanup | клиентский `connection refused`; лог `upstream dial failed` |
| AC-005 | Opaque bridge: любой фрейм (включая неизвестный тип) проходит без модификации | `relay.go` bridge loop (raw message copy) | relay не логирует `unknown frame` |

## Данные и контракты

- Data model: добавляется `RelayCfg` — см. `data-model.md`.
- Контракты не меняются — relay работает на уровне WS message (opaque pipe), не интерпретирует фреймы.
- Relay не вводит новых wire-форматов или API.

## Стратегия реализации

### DEC-001 Reuse `dialStream()` для upstream

- Why: `dialStream()` уже содержит всю логику выбора транспорта (WS/QUIC), uTLS, padding, SNI, crypto. Relay получает все обфускации бесплатно.
- Tradeoff: relay жёстко привязан к клиентскому стеку транспорта — изменения в `dialStream()` автоматически влияют на relay. Это желаемое поведение.
- Affects: `relay.go` вызывает `dialStream()`.
- Validation: AC-001 — upstream handshake проходит со всеми опциями обфускации.

### DEC-002 Relay listener использует серверный TLS-конфиг из `tlspkg`

- Why: `tlspkg.NewServerTLSConfig` уже создаёт `*tls.Config` для сервера (cert, key, mTLS). Relay listener — та же задача.
- Tradeoff: relay зависит от `tlspkg`, но это внутренний пакет проекта.
- Affects: `relay.go` → `tlspkg.NewServerTLSConfig()`.
- Validation: AC-003 — relay слушает с TLS, клиент подключается через WSS.

### DEC-003 Bridge без горутин на сообщение: две горутины на сессию (client→upstream, upstream→client)

- Why: минимальное число горутин, каждая блокируется на `ReadMessage()`. При обрыве одной — закрываем второй контекст/канал.
- Tradeoff: при 100 одновременных сессий = 200 горутин + 200 TLS-соединений. SC-001 проверяет утечки.
- Affects: `relay.go` bridge loop.
- Validation: SC-001 — 100 сессий без утечки горутин.

### DEC-004 Frame-level bridge, не byte-level

- Why: `ReadMessage()`/`WriteMessage()` — существующий интерфейс `StreamConn`. Побайтовый copy не работает с WS/QUIC framed transport.
- Tradeoff: relay должен читать полное сообщение перед записью — добавляет latency одного сообщения. Для VPN-трафика <MTU это незначительно (<<1ms).
- Affects: `relay.go` — `stream.ReadMessage()` + `upstream.WriteMessage()` в цикле.
- Validation: AC-002 — data проходит в обе стороны.

## Incremental Delivery

### MVP (Первая ценность)

- WS relay: TLS listener + accept + dial upstream + bridge loop.
- AC-001, AC-002, AC-003, AC-004, AC-005.
- Validation: ручной тест из First Validation Path.

### Итеративное расширение

- P2: QUIC relay — требует listener QUIC + accept из quic-go, отдельный bridge для QUIC streams. AC-* не меняются, добавляется транспорт.

## Порядок реализации

1. Конфиг: `RelayCfg` struct + default values в `LoadClientConfig()`.
2. `relay.go`: TLS listener + accept loop.
3. `relay.go`: bridge per connection (handshake forward + data copy).
4. `client.go`: бранч `mode == "relay"` → `runRelayMode()`.
5. Конфиг example в `configs/client.yaml`.

Шаги 2-3 можно параллелить с 1 (зависимость только от struct, не от config loading).

## Риски

- **Double-close при одновременном обрыве**: bridge loop с двух сторон может вызвать двойное закрытие. Mitigation: `sync.Once` для close, контекст с cancel.
- **Self-signed cert не указан**: RQ-08 требует автогенерацию. Mitigation: использовать `crypto/tls` генерацию на лету + WARNING в лог.
- **Upstream dial timeout**: если upstream долго не отвечает, клиент висит. Mitigation: применить `context.WithTimeout` на dial (по умолч. 10s).

## Rollout and compatibility

- Новый mode — обратная совместимость полная: `mode: ""` (TUN) и `mode: "proxy"` не меняются.
- Если `RelayCfg` не указан при `mode: relay` — relay не стартует с ошибкой конфига.
- Специальных rollout-действий не требуется — relay isolated mode.

## Проверка

- `go test ./src/internal/config/...` — тесты нового конфига.
- `go vet ./src/internal/bootstrap/client/...` — статический анализ.
- Ручной сценарий: сборка relay, upstream, клиент → проверка handshake + ping.
- AC-001..AC-005 покрываются manual validation path; автоматизация — интеграционный тест в P2.

## Соответствие конституции

- Нет конфликтов: feature branch, traceability, clean architecture, языковая политика соблюдены.
