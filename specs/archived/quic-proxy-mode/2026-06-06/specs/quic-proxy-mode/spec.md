# QUIC Proxy Mode

## Scope Snapshot

- In scope: использование QUIC (вместо WebSocket/TCP) как транспорта в proxy-mode клиента, с сохранением обратной совместимости.
- Out of scope: multi-stream QUIC (отдельный stream на SOCKS-коннект), TUN-mode hybrid proxy через QUIC.

## Цель

Пользователи proxy-mode с плохим каналом (высокий RTT, потери) получают прирост скорости за счёт устранения TCP-over-TCP meltdown. Клиент/сервер договариваются о транспорте в одном конфиге — той же опцией `transport`, что и в TUN-mode.

## Основной сценарий

1. Пользователь указывает `transport: quic` в конфиге клиента (сервер уже поддерживает QUIC listener).
2. Клиент устанавливает QUIC-соединение (один bidir stream, как в TUN-mode) и проходит handshake.
3. Все proxy-фреймы (SOCKS/HTTP CONNECT) мультиплексируются поверх этого QUIC stream через существующий `FrameTypeProxy`.
4. При обрыве соединения — reconnection loop с fallback QUIC→TCP (как в TUN).

## MVP Slice

Замена `*websocket.WSConn` на `tunnel.StreamConn` в `proxy.go`, `proxy.Manager`, `Stream.ForwardToWS()` + добавление выбора транспорта в `runProxyMode()` по `cfg.Transport`.

## First Deployable Outcome

`kvn-client --mode proxy --transport quic` подключается к серверу через QUIC, SOCKS5/HTTP-запросы проксируются, `curl -x socks5://... https://example.com` работает.

## Scope

- `src/internal/bootstrap/client/proxy.go` — выбор транспорта, замена `*websocket.WSConn` на `tunnel.StreamConn`
- `src/internal/proxy/stream.go` — `ForwardToWS()` и `Manager` переходят на `tunnel.StreamConn`
- `src/internal/transport/quic/dial.go` — уже готов, без изменений
- Server: `handleStream()` уже принимает `tunnel.StreamConn` — изменений не требуется

## Контекст

- TUN-mode уже поддерживает QUIC: `reconnectLoop()` выбирает транспорт, `tunnel.Session` получает `tunnel.StreamConn`
- Proxy-mode использует собственную `runProxySession()` (не `tunnel.Session`), с прямым вызовом `wsConn.ReadMessage/WriteMessage`
- `proxy.Manager` и `Stream.ForwardToWS()` завязаны на `*websocket.WSConn`
- `tunnel.StreamConn` содержит только `ReadMessage/WriteMessage/Close/SetReadDeadline/SetWriteDeadline`
- `quic.QUICConn` уже реализует `tunnel.StreamConn`
- `websocket.WSConn` не реализует `tunnel.StreamConn` сейчас — нужно добавить метод `SetWriteDeadline` (заглушка или реальный)

## Требования

- RQ-001 Клиент proxy-mode ДОЛЖЕН поддерживать `transport: quic` и `transport: tcp` (по умолчанию `tcp`, обратная совместимость).
- RQ-002 `proxy.Manager` и `Stream.ForwardToWS()` ДОЛЖНЫ работать с `tunnel.StreamConn`, а не с `*websocket.WSConn`.
- RQ-003 При недоступности QUIC proxy-mode ДОЛЖЕН падать на TCP (fallback с warn-логом, как в TUN).
- RQ-004 Серверная сторона НЕ ДОЛЖНА требовать изменений — `handleStream()` уже принимает `tunnel.StreamConn`.

## Вне scope

- Multi-stream QUIC (один QUIC stream на SOCKS-коннект) — вторая итерация.
- Proxy-over-TUN (FrameTypeProxy в TUN-mode через QUIC) — уже работает, не требует изменений.
- Оптимизация производительности QUIC для proxy (0-RTT handshake, потоковая запись) — отдельная итерация.

## Критерии приемки

### AC-001 QUIC proxy-mode handshake и data flow

- Почему: базовый сценарий — proxy трафик через QUIC
- **Given** сервер с QUIC listener + клиент с `mode: proxy, transport: quic`
- **When** клиент подключается и выполняет SOCKS5-запрос
- **Then** handshake проходит, proxy-фреймы передаются, ответ приходит
- Evidence: логи "QUIC transport selected", `curl -x socks5h://...` успешно получает ответ

### AC-002 Fallback на TCP

- Почему: не терять соединение при блокировке UDP
- **Given** клиент с `transport: quic`, UDP заблокирован
- **When** QUIC dial не удаётся
- **Then** клиент переключается на TCP/WebSocket
- Evidence: лог "QUIC dial failed, falling back to TCP"

### AC-003 Обратная совместимость

- Почему: старые конфиги не должны ломаться
- **Given** клиент с `mode: proxy` без `transport`
- **When** клиент подключается
- **Then** транспорт TCP/WebSocket как и раньше
- Evidence: соединение через WS, proxy работает

## Допущения

- quic-go v0.50.0 стабилен на Go 1.25 (как в TUN-mode).
- Сервер уже развёрнут с QUIC listener из `quic-transport`.
- `tunnel.StreamConn` достаточно для WriteMessage/ReadMessage — не требуется доступ к WS-specific API (Keepalive, NextWriter).

## Критерии успеха

- SC-001 SOCKS5 через proxy-mode QUIC работает без ошибок на хорошем канале (latency <10ms, loss 0%).
- SC-002 Время установки proxy-соединения не превышает время TCP/WS-соединения более чем на 1s.

## Краевые случаи

- UDP заблокирован — fallback на TCP (как в TUN).
- Сервер без QUIC listener — клиент получает connection refused, fallback на TCP.
- WSConn не реализует `SetWriteDeadline` — добавить no-op заглушку.

## Открытые вопросы

- `WSConn.SetWriteDeadline` — нужна реальная реализация или no-op достаточно для proxy-mode?
- Нужна ли опция `transport: auto` (try QUIC first, fallback TCP) в proxy-mode?
