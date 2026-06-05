# QUIC Obfuscation — Задачи

## Phase Contract

Inputs: `specs/active/quic-obfuscation/plan.md`.
Outputs: задачи с покрытием AC-001, AC-002, AC-003, AC-004.
Stop if: нет.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/transport/quic/obfuscated.go` | T1.1 |
| `src/internal/transport/quic/obfuscated_test.go` | T1.2 |
| `src/internal/config/client.go` | T2.1 |
| `src/internal/config/server.go` | T2.1 |
| `src/internal/bootstrap/client/tun.go` | T2.2 |
| `src/internal/bootstrap/client/proxy.go` | T2.2 |
| `src/internal/bootstrap/server/server.go` | T2.3 |
| `docs/ru/config.md`, `docs/en/config.md` | T3.1 |
| `examples/client.yaml`, `examples/server.yaml` | T3.2 |
| `src/internal/webui/frontend/src/App.tsx` | T3.3 |
| `src/internal/webui/handler_config.go` | T3.3 |

## Implementation Context

- Цель MVP: ObfuscatedQUICConn wrapper + конфиг + bootstrap
- Границы приемки: AC-001, AC-002, AC-003, AC-004
- Ключевые правила:
  - trace-маркеры `@sk-task` обязательны на изменённых объявлениях
  - `obfuscation: false` (default) = поведение не меняется
  - ObfuscatedQUICConn embed'ит *QUICConn, реализует StreamConn
- Инварианты: `tunnel.StreamConn` не меняется; QUIC length-prefix framing не меняется
- Контракты: 8-байт nonce в начале стрима, XOR 4-байт length prefix
- Proof signals: `go build ./...`, `go test ./src/internal/transport/quic/... -v`
- Вне scope: WS obfuscation, смена nonce в сессии

## Фаза 1: Основа

Осознанно пропущена — quic-go v0.50 уже есть, QUICConn уже существует.

## Фаза 2: MVP Slice

Цель: ObfuscatedQUICConn wrapper + интеграция в конфиг и bootstrap.

- [x] T1.1 Реализовать `ObfuscatedQUICConn` в `quic/obfuscated.go`.
  Touches: `src/internal/transport/quic/obfuscated.go`
  - `ObfuscatedQUICConn` embed'ит `*QUICConn`
  - При создании с `isClient=true`: генерирует 8-байт nonce (crypto/rand), пишет raw в stream
  - `ReadMessage()`: server-side — первый read читает nonce (8 байт), затем XOR length prefix; client-side — XOR length prefix (nonce уже установлен)
  - `WriteMessage()`: XOR length prefix с nonce, затем payload
  - Немодифицированные методы делегируются `*QUICConn`
  - `NewObfuscatedQUICConn(core *QUICConn, isClient bool) (*ObfuscatedQUICConn, error)`
  - AC-001, AC-003

- [x] T1.2 Написать unit test для XOR roundtrip.
  Touches: `src/internal/transport/quic/obfuscated_test.go`
  - `TestObfuscatedRoundtrip`: клиент → nonce + XOR'd message → сервер → message совпадает
  - `TestObfuscatedNoCorruption`: случайные payload'ы разных размеров
  - AC-003

- [x] T2.1 Добавить `Obfuscation bool` в конфиги клиента и сервера.
  Touches: `src/internal/config/client.go`, `src/internal/config/server.go`
  - `ClientConfig.Obfuscation bool` с `json:"obfuscation" mapstructure:"obfuscation"`
  - `ServerConfig.Obfuscation bool` с `mapstructure:"obfuscation"`
  - AC-001, AC-002

- [x] T2.2 Обновить client bootstrap (tun + proxy) — wrap QUICConn при obfuscation.
  Touches: `src/internal/bootstrap/client/tun.go`, `src/internal/bootstrap/client/proxy.go`
  - После `quictp.Dial()` проверить `c.cfg.Obfuscation`
  - Если true: `quictp.NewObfuscatedQUICConn(quicConn, true)` → присвоить `stream`
  - AC-001, AC-002

- [x] T2.3 Обновить server bootstrap — wrap после Accept при obfuscation.
  Touches: `src/internal/bootstrap/server/server.go`
  - После `quicListener.Accept()` проверить `s.cfg.Obfuscation`
  - Если true: `quictp.NewObfuscatedQUICConn(quicConn, false)` → передать в `handleStream`
  - AC-001, AC-002

## Фаза 3: Основная реализация

Осознанно пропущена — MVP покрывает все AC.

## Фаза 4: Проверка

- [x] T4.1 Проверить сборку и unit test.

- [x] T3.1 Обновить документацию config.md (ru + en).
  Touches: `docs/ru/config.md`, `docs/en/config.md`
  - Добавить строку `obfuscation` в таблицу серверного конфига (после `mtu` или `crypto`)
  - Добавить строку `obfuscation` в таблицу клиентского конфига (после `transport`)
  - Тип `bool`, default `false`, описание "Включить обфускацию QUIC стрима (anti-DPI)"
  - AC-001

- [x] T3.2 Обновить примеры конфигов.
  Touches: `examples/client.yaml`, `examples/server.yaml`
  - Examples/client.yaml: добавить `obfuscation: false`
  - Examples/server.yaml: добавить `obfuscation: false`
  - AC-001

- [x] T3.3 Добавить `obfuscation` checkbox в kvn-web UI.
  Touches: `src/internal/webui/frontend/src/App.tsx`, `src/internal/webui/handler_config.go`
  - `App.tsx`: добавить `obfuscation?: boolean` в `ClientConfig` interface
  - `App.tsx`: добавить `<Checkbox>` для `obfuscation` в секцию "Advanced" (после multiplex)
  - `handler_config.go` (опционально): default `false` уже задан zero value
  - AC-001

## Покрытие критериев приемки

- AC-001 -> T1.1, T2.1, T2.2, T2.3, T3.1, T3.2, T3.3, T4.1
- AC-002 -> T2.1, T2.2, T2.3, T4.1
- AC-003 -> T1.1, T1.2, T4.1
- AC-004 -> T4.1 (build), ручной iperf (post-MVP)

## Заметки

- trace-маркеры `@sk-task quic-obfuscation#T1.1`, `@sk-task quic-obfuscation#T1.2` и т.д. на изменённых объявлениях.
