# Context — src/internal/transport/websocket

## Purpose

Реализация `transport.StreamConn` поверх gorilla/websocket: binary-сообщения как границы кадров, опциональный padding, маскировка через uTLS, мультиплексирование подпротоколом, keepalive ping/pong и диал с отменой по контексту.

## Responsibilities

- Владеет: `WSConn`, `Dial`/`DialContext`, `Accept`, `WSConfig`, padding/unpadding, `BatchWriter`, origin-checker'ами, keepalive и буферами TCP.
- Не владеет: TLS-политикой (`transport/tls`), форматом KVN-кадра (`transport/framing`), реестром фабрик (`transport`).

## How it works

- `DialContext`: gorilla `Dialer` с `HandshakeTimeout 10s`; при `Multiplex` предлагает сабпротокол `kvn-ws-mux`; при `UTLS` подменяет `NetDialTLSContext` на `wtls.DialWithUTLS`; TCP получает `NoDelay` и буферы 4 МиБ.
- Диал отменяется вместе с ctx через `context.AfterFunc`, закрывающий соединение (gorilla сам применяет только handshake-timeout).
- `WriteMessage`: под `writeMu` либо raw binary, либо padding-конверт — `uint32 BE len(payload)` + payload + случайный паддинг до кратности `PaddingSize` (по умолчанию 512). `ReadMessage` при включённом padding снимает 4-байтовый префикс.
- Padding можно выключить на лету (`SetPadding`) — например, после handshake для data-фазы.
- Unpadded-переговоры: клиент шлёт `X-KVN-NoPad: 1` (`RequestNoPad`), сервер подтверждает тем же заголовком (`AllowNoPad`); без подтверждения padding остаётся включённым (обратная совместимость).
- Keepalive: `SetKeepalive(interval, timeout)` шлёт ping'и из тикера через `controlCh`, pong продлевает read-deadline на `timeout`; `Accept` по умолчанию ставит pong-deadline `120s`.
- `BatchWriter` копит мелкие записи и сбрасывает их по порогу/таймеру; `Close` идемпотентен через `sync.Once`.
- `Accept` принимает variadic `interface{}` — `func(*http.Request) bool` как CheckOrigin и/или `WSConfig`; после Upgrade ставит TCP_NODELAY и буферы, стартует control-writer и ping-handler.
- `NewOriginChecker(whitelist, allowEmpty)` сравнивает Origin/Referer с glob-шаблонами (`*` — единственный wildcard).

## Key files

- `src/internal/transport/websocket/websocket.go` — `WSConn`, dial/accept, padding, keepalive, batch writer, origin checker.
- `src/internal/transport/websocket/wsfactory.go` — `WSFactory` и регистрация `"ws"` в реестре транспортов.

## Public contracts

- `Dial(serverURL, tlsCfg, logger, cfg ...WSConfig)` и `DialContext(ctx, ...)` — соединение к `ws://`/`wss://`.
- `Accept(w, r, logger, args ...interface{})` — апгрейд на стороне сервера.
- `WSConfig{Multiplex, MTU, UTLS, UTLSFallback, PaddingEnabled, PaddingSize, RequestNoPad, AllowNoPad}`.
- `WSConn` реализует `transport.StreamConn`; дополнительные методы: `SetPadding`, `SetKeepalive`, `SetPingHandler`, `Underlying`, `Subprotocol`.
- Константы `MultiplexSubprotocol = "kvn-ws-mux"`, `DefaultPongTimeout = 120s`, заголовок `X-KVN-NoPad`.

## Dependencies

- `github.com/gorilla/websocket`, `go.uber.org/zap`, `src/internal/transport`, `src/internal/transport/tls`.

## Consumed by

- `src/internal/transport` (регистрация), `src/internal/bootstrap/{client,server,relay}`, `src/internal/protocol/handshake`, `src/internal/tunnel` — через `StreamConn`.

## Gotchas

- `WSConfig.MTU` в этом пакете **не используется** (передаётся фабрикой, но WSConn его не читает) — сегментация MTU делается выше, в framing/tunnel.
- Padding — не шифрование: длина payload'а идёт открытым `uint32`, случайные байты только добивают размер; включать его как защиту нельзя.
- Паддинг и `X-KVN-NoPad` должны совпадать у обеих сторон; рассинхрон даёт «padding frame too short»/мусорные длины.
- Ping/pong идут через `controlCh` (буфер 8); control-writer берёт `writeMu`, поэтому тяжёлый `WriteMessage` может задержать ping, но не блокирует навсегда.
- `Accept` без `WSConfig.AllowNoPad` проигнорирует запрос на unpadded-режим, даже если клиент его прислал.
- `DialContext` при `UTLS` создаёт соединение вручную; при `UTLSFallback` откат идёт на raw TCP + обычный `crypto/tls`, без повторного uTLS.
- Origin-checker по умолчанию (без аргументов) разрешает все origin'ы — небезопасный default для сервера.
