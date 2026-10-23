# Context — src/internal/transport

## Purpose

Общая абстракция транспорта: единый интерфейс сообщений (`StreamConn`), опциональные датаграммы, реестр фабрик по имени транспорта и обёртка с автоматическим fallback'ом. Позволяет клиенту/серверу/relay не знать, WS под ними или QUIC.

## Responsibilities

- Владеет: интерфейсами `StreamConn`, `DatagramConn`, `TransportFactory`, `TransportListener`; глобальным реестром `Register`; `NewFactory`; `FallbackFactory`; `CountingStreamConn` для метрик.
- Не владеет: реализациями транспортов (подпакеты `websocket`, `quic`), TLS-конфигами (`transport/tls`), форматом кадров (`transport/framing`).

## How it works

- Подпакеты `websocket` и `quic` в своих `init()` вызывают `transport.Register("ws"/"quic", ...)`; регистрирующий код должен быть импортирован (клиент делает blank-import).
- `NewFactory(typeStr, cfg)` нормализует `""`/`"tcp"` → `"ws"`; неизвестное имя **молча** даёт `"ws"`, отсутствие `"ws"` — `nil`.
- `FactoryConfig` — общий набор настроек: TLS, logger, `MaxMessageSize`, keepalive, `Multiplex`, `MTU`, uTLS, padding, `RequestNoPad`, `Obfuscation`.
- `FallbackFactory.Dial` пробует primary, при ошибке логирует warn и вызывает secondary; `Listen` всегда идёт только в primary.
- `CountingStreamConn` встраивает `StreamConn`, считает байты payload'а (длина сообщения, не провод) и пробрасывает `DatagramConn` через type assertion.
- `ReadMessage`/`WriteMessage` — message-oriented API: валидная реализация обязана сохранять границы сообщений (WS — binary frame, QUIC — 4-байтовый length prefix).

## Key files

- `src/internal/transport/transport.go` — все интерфейсы, реестр, `NewFactory`, `FallbackFactory`, `CountingStreamConn`.

## Public contracts

- `StreamConn`: `ReadMessage() ([]byte, error)`, `WriteMessage([]byte) error`, `SetReadDeadline(time.Time) error`, `SetWriteDeadline(time.Time) error`, `Close() error`.
- `DatagramConn`: `SupportsDatagrams() bool`, `SendDatagram([]byte) error`, `ReceiveDatagram(ctx) ([]byte, error)`.
- `TransportFactory`: `Dial(ctx, endpoint)`, `Listen(ctx, addr)`; `TransportListener`: `Accept(ctx)`, `Close()`.
- `Register(name, ctor)`, `NewFactory(type, *FactoryConfig)`, `NewFallbackFactory(primary, secondary, logger)`.
- `FactoryConfig` — стабильный контракт настроек: добавление поля требует поддержки в обеих фабриках.

## Dependencies

- Только стандартная библиотека + `go.uber.org/zap`; конкретные транспорты регистрируются из подпакетов (обратная зависимость).

## Consumed by

- `src/internal/bootstrap/{client,server,relay}`, `src/internal/protocol/handshake`, `src/internal/dnsproxy`, `src/internal/proxy/stream.go`, `src/internal/tunnel/{session,stream}.go`, `src/cmd/gatetest`, `src/integration/`.

## Gotchas

- `NewFactory` никогда не возвращает ошибку: опечатка в `transport` тихо переключает на WS, а не падает.
- `FallbackFactory.Listen` не фолбэчится — fallback только для клиентского dial.
- `CountingStreamConn` считает длину сообщения, а не сетевые байты (шифрование/заголовки не учитываются) — цифры метрик приблизительные.
- Обёртки (`CountingStreamConn`) сохраняют `DatagramConn` только если базовая реализация его поддерживает; иначе возвращают `"datagrams unsupported"`.
- Реестр глобальный и защищён мьютексом; повторная `Register` с тем же именем молча перезаписывает фабрику.
