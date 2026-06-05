# QUIC Proxy Mode — Задачи

## Phase Contract

Inputs: `specs/active/quic-proxy-mode/plan.md`, `plan/data-model.md`.
Outputs: задачи с покрытием AC-001, AC-002, AC-003.
Stop if: нет.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/proxy/stream.go` | T2.1, T4.1 |
| `src/internal/bootstrap/client/proxy.go` | T2.2 |
| `src/internal/bootstrap/client/tun.go` | (reference only) |
| `src/internal/transport/websocket/websocket.go` | (already implements StreamConn) |

## Implementation Context

- Цель MVP: замена `*websocket.WSConn` на `tunnel.StreamConn` в proxy-mode + выбор транспорта.
- Границы приемки: AC-001, AC-002, AC-003.
- Ключевые правила:
  - trace-маркеры `@sk-task` обязательны на изменённых объявлениях
  - Никаких изменений сервера
  - `transport: ""` → TCP/WS (обратная совместимость)
- Инварианты данных: `tunnel.StreamConn` interface не меняется; `ProxyFrame` формат не меняется.
- Контракты: `config.ClientConfig.Transport`, `handshake.ClientHello.Transport` уже существуют.
- Proof signals: `go build ./...`, ручной proxy smoke test через QUIC и WS.
- Вне scope: общий dial-слой для TUN и proxy, multi-stream QUIC.

## Фаза 1: Основа

Осознанно пропущена — вся структура уже есть (`tunnel.StreamConn`, `quic.QUICConn`, `websocket.WSConn`).

## Фаза 2: MVP Slice

Цель: заменить тип `*websocket.WSConn` на `tunnel.StreamConn` во всех proxy-mode поверхностях и добавить выбор транспорта.

- [x] T2.1 Перевести `proxy.Manager` и `Stream.ForwardToWS()` на `StreamConn`.
  Touches: `src/internal/proxy/stream.go`
  - `ForwardToWS(*websocket.WSConn)` → `ForwardToStream(StreamConn)` (локальный интерфейс)
  - `Manager.wsConn *websocket.WSConn` → `Manager.stream StreamConn`
  - `NewManager(*websocket.WSConn)` → `NewManager(StreamConn)`
  - Импорт `tunnel` заменён на `time` + локальный `StreamConn` (избегание import cycle)
  - AC-001, AC-003

- [x] T2.2 Добавить выбор транспорта в `runProxyMode()` и обновить `runProxySession()`.
  Touches: `src/internal/bootstrap/client/proxy.go`
  - `runProxySession(ctx, *websocket.WSConn)` → `runProxySession(ctx, proxy.StreamConn)`
  - Блок выбора транспорта (по `cfg.Transport`), паттерн из tun.go:58-95
  - При `transport == "quic"` → `quictp.Dial()` с fallback на TCP
  - QUIC address parse через `url.Parse(c.cfg.Server).Host`
  - Импорты: `proxy` (для StreamConn), `quictp`, `net/url`
  - AC-001, AC-002, AC-003

## Фаза 3: Основная реализация

Осознанно пропущена — MVP покрывает все AC.

## Фаза 4: Проверка

- [x] T4.1 Проверить сборку и выполнить ручной smoke test.
  Touches: `src/internal/proxy/stream.go`, `src/internal/bootstrap/client/proxy.go`
  - `go build ./...` — успешная компиляция
  - `go test ./...` — все тесты проходят
  - `go vet ./...` — без ошибок
  - Сервер c `transport: quic` → клиент `--mode proxy --transport quic` → `curl -x socks5h://...` OK
  - Клиент без `transport` → WS соединение → `curl -x socks5h://...` OK
  - AC-001, AC-002, AC-003

## Покрытие критериев приемки

- AC-001 -> T2.1, T2.2, T4.1
- AC-002 -> T2.2, T4.1
- AC-003 -> T2.1, T2.2, T4.1

## Заметки

- T4.1 одновременно служит verify — после прохождения можно архивировать spec.
- trace-маркеры `@sk-task quic-proxy-mode#T2.1` и `@sk-task quic-proxy-mode#T2.2` на изменённых объявлениях.
