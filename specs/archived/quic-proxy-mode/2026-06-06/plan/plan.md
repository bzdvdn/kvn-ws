# QUIC Proxy Mode — План

## Phase Contract

Inputs: `specs/active/quic-proxy-mode/spec.md` + текущее состояние proxy-mode кода.
Outputs: plan с задачами, реализация в `feature/quic-proxy-mode`.
Stop if: нет — spec однозначна.

## Цель

Перевести proxy-mode клиента с `*websocket.WSConn` на `tunnel.StreamConn`, чтобы QUIC работал в proxy-mode по тому же паттерну, что и в TUN-mode. Сервер уже transport-agnostic — изменений не требует.

## MVP Slice

Замена типа во всех proxy-mode surfaces + выбор транспорта в `runProxyMode()`. Закрывает AC-001, AC-002, AC-003.

## First Validation Path

1. Собрать `kvn-client` и `kvn-server` с ветки.
2. Запустить сервер с `transport: quic`.
3. Запустить клиент `--mode proxy --transport quic`.
4. `curl -x socks5h://127.0.0.1:2310 https://httpbin.org/get` → успешный ответ.
5. Убрать `--transport quic` → соединение через WS.

## Scope

- `src/internal/proxy/stream.go` — `ForwardToWS` → `ForwardToStream`, `Manager.wsConn` → `tunnel.StreamConn`
- `src/internal/bootstrap/client/proxy.go` — выбор транспорта, `runProxySession` принимает `tunnel.StreamConn`
- `src/internal/transport/quic/` — уже готов, без изменений

Явная граница: сервер (`handler.go`) не меняется.

## Implementation Surfaces

| Surface | Тип | Зачем меняется |
|---------|-----|----------------|
| `proxy/stream.go:ForwardToWS` | existing | замена `*websocket.WSConn` на `tunnel.StreamConn` |
| `proxy/stream.go:Manager.wsConn` | existing | замена `*websocket.WSConn` на `tunnel.StreamConn` |
| `proxy/stream.go:NewManager` | existing | сигнатура: `*websocket.WSConn` → `tunnel.StreamConn` |
| `client/proxy.go:runProxyMode` | existing | добавить транспорт selection (как в tun.go:64-95) |
| `client/proxy.go:runProxySession` | existing | параметр `*websocket.WSConn` → `tunnel.StreamConn` |
| `websocket/websocket.go` | existing | уже реализует `tunnel.StreamConn` — без изменений |

## Bootstrapping Surfaces

`none` — вся необходимая структура уже есть: `tunnel.StreamConn`, `quic.QUICConn`, `websocket.WSConn`.

## Влияние на архитектуру

- `proxy.Manager` теряет зависимость от `*websocket.WSConn` — становится transport-agnostic
- Локальное влияние на 2 файла, меняется только тип аргумента
- Обратная совместимость: `transport: ""` → TCP/WS, как в TUN

## Acceptance Approach

- **AC-001**: `proxy/stream.go` замена типа + `proxy.go` QUIC dial → ручной тест `curl -x socks5h://`
- **AC-002**: `proxy.go` fallback логика → лог "QUIC dial failed, falling back to TCP"
- **AC-003**: `proxy.go` default transport → `wsConn.Dial` без `cfg.Transport` → ручной тест

## Данные и контракты

Нет изменений: `Transport` поле уже есть в `config.ClientConfig`, `handshake.ClientHello`/`ServerHello`. `data-model.md` — no-change stub.

## Стратегия реализации

### DEC-001 Замена типа без рефакторинга

- Why: минимальное изменение — меняем только тип, не трогаем логику. proxy/stream.go меняет 3 сигнатуры, proxy.go ~15 строк.
- Tradeoff: `ForwardToWS` остаётся с "WS" в имени — rename будет, но уже сейчас, вместе с заменой типа.
- Affects: `proxy/stream.go`, `client/proxy.go`
- Validation: `go build ./...`

### DEC-002 Тот же паттерн транспорта, что в TUN

- Why: единообразие — копируем блок выбора транспорта из tun.go:58-95 в proxy.go. Fallback QUIC→TCP с warn логом.
- Tradeoff: Дублирование кода, но выделение общего dial-слоя — в отдельную итерацию.
- Affects: `client/proxy.go`
- Validation: визуально идентичная логика

## Incremental Delivery

### MVP

1. `proxy/stream.go`: `ForwardToWS` → `ForwardToStream`, Manager → `tunnel.StreamConn`
2. `client/proxy.go`: выбор транспорта + runProxySession → `tunnel.StreamConn`
3. `go build ./...` + ручная проверка

### Итеративное расширение

- Выделение общего `dialTransport()` для TUN и proxy (post-MVP)
- Multi-stream QUIC для proxy (отдельная spec)

## Порядок реализации

1. **proxy/stream.go** — меняем типы (никто не зависит от старых имён, кроме proxy.go)
2. **client/proxy.go** — новый транспорт selection + сигнатура runProxySession
3. **Проверка**: `go build ./...`, ручной smoke test

Параллелить нечего — две задачи последовательны.

## Риски

- **WSConn имплементирует `tunnel.StreamConn`?** — да, все методы уже есть (ReadMessage, WriteMessage, SetReadDeadline, SetWriteDeadline, Close). Риска нет.
- **Server сломается?** — нет, `handleStream()` уже принимает `tunnel.StreamConn`. Риска нет.
- **QUIC proxy не переживает переподключение** — переиспользуется reconnection loop из TUN (runProxyMode уже имеет backoff). Риска нет.

## Rollout и compatibility

Специальных rollout действий не требуется. Старый config без `transport` → WS, новый `transport: quic` → QUIC.

## Проверка

- `go build ./...` — проверка сборки (все AC)
- Ручной: socks5 + curl на QUIC транспорте (AC-001)
- Ручной: socks5 + curl на WS транспорте без `transport` (AC-003)
- Ручной: блокировка UDP → fallback (AC-002)

## Соответствие конституции

нет конфликтов.
