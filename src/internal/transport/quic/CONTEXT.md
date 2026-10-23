# Context — src/internal/transport/quic

## Purpose

Реализация `transport.StreamConn` поверх quic-go: один QUIC-стрим как поток сообщений с 4-байтовым length-prefix, плюс ненадёжные датаграммы и опциональная XOR-обфускация на TLS-экспортёре.

## Responsibilities

- Владеет: `QUICConn` (read/write/deadlines/datagrams), `Dial`/`Listen`/`Accept`, `AcceptWithBackoff`, `ObfuscatedQUICConn`, маркером `DatagramTransport`, `DatagramTooLarge`.
- Не владеет: TLS-политикой (`transport/tls`), форматированием KVN-кадров (`transport/framing`), пулами сообщений верхнего уровня.

## How it works

- `QUICConn` хранит `quic.Connection` + один `quic.Stream`; сообщение = `uint32 BE length` + payload. `WriteMessage` сериализуется `writeMu`; read/write deadlines — раздельные мьютексы, чтобы не блокировать друг друга.
- `maxMessageSize` — `atomic.Int32`, по умолчанию 10 МиБ; `SetMaxMessageSize` игнорирует `<=0` и `> MaxInt32`.
- `ReadMessage` читает префикс, проверяет лимит (`ErrMessageTooLarge`) и читает payload в буфер из внутреннего пула.
- `Close` закрывает стрим и всё QUIC-соединение с `CloseWithError(0, "close")`.
- Датаграммы: `SupportsDatagrams` смотрит на `ConnectionState().SupportsDatagrams`; `Dial`/`Listen` **всегда** включают `EnableDatagrams` в конфиге; `DatagramTransport = "quic-dgram"` — маркер, которым handshake сообщает о запросе датаграмм; `DatagramTooLarge(err)` отдаёт `MaxDatagramPayloadSize` пира.
- `Dial` парсит URL в host:port, ставит таймаут 10с и открывает первый стрим через `OpenStreamSync`.
- `Listener.Accept` возвращает коннект с первым принятым стримом; `AcceptWithBackoff` повторяет при временных ошибках (10мс → 5с), останавливается только на `context.Canceled`.
- `ObfuscatedQUICConn`: 8-байтовый nonce из `TLS.ExportKeyingMaterial("kvn-obfuscation", nil, 8)` (ленивая инициализация, `CompareAndSwap`), XOR применяется к length-prefix и payload'у, а также к датаграммам; nonce один на оба направления.
- Фабрика `QUICFactory` регистрирует `"quic"`, ставит `MaxMessageSize` и при `Obfuscation` оборачивает соединение в `ObfuscatedQUICConn`.

## Key files

- `src/internal/transport/quic/conn.go` — `QUICConn`, лимит сообщения, датаграммы.
- `src/internal/transport/quic/obfuscated.go` — XOR-обфускация стрима и датаграмм.
- `src/internal/transport/quic/dial.go` — `Dial`, таймаут, нормализация адреса.
- `src/internal/transport/quic/listen.go` — `Listen`, `Accept`, `AcceptWithBackoff`.
- `src/internal/transport/quic/quicfactory.go` — фабрика и регистрация `"quic"`.

## Public contracts

- `Dial(ctx, addr, *tls.Config, *quic.Config) (*QUICConn, error)`, `Listen(addr, ...) (*Listener, error)`, `(*Listener).Accept(ctx)`.
- `NewQUICConn(conn, stream)`, `(*QUICConn).SetMaxMessageSize(int)`, `StreamID()`, плюс методы `transport.StreamConn`/`DatagramConn`.
- `AcceptWithBackoff(ctx, acceptFn, logger)`, `DatagramTooLarge(err) (int64, bool)`, `DatagramCapable(transport string) bool`.
- `NewObfuscatedQUICConn(core)`, `(*ObfuscatedQUICConn).SetNonce([8]byte)` (для тестов).
- Константы `ErrMessageTooLarge`, `DatagramTransport = "quic-dgram"`, `DefaultDialTimeout = 10s`.

## Dependencies

- `github.com/quic-go/quic-go`, `go.uber.org/zap`, `src/internal/transport` (интерфейсы и регистрация).

## Consumed by

- `src/internal/transport` (регистрация), `src/internal/bootstrap/{client,server,relay}`, `src/internal/protocol/handshake`, `src/internal/tunnel` — через `StreamConn`/`DatagramConn`.

## Gotchas

- Датаграммы включаются в QUIC-конфиге всегда, независимо от `udp_datagrams`; конфиг лишь решает, использовать ли их (согласование — через `DatagramTransport` в handshake).
- XOR-обфускация — не криптография: одинаковый nonce в обе стороны, XOR обратим; защита только от сигнатурного анализа.
- `initNonce` в тестовом режиме (`conn == nil`) молча ставит нулевой nonce — обфускация фактически отключается.
- Length-prefix тоже XOR-ится (и при чтении, и при записи), поэтому сторонний/необфусцированный peer не прочитает обфусцированный стрим.
- `AcceptWithBackoff` ретраит **все** ошибки кроме `context.Canceled` (включая постоянные) — при неустранимой ошибке цикл не завершится сам, нужен ctx с дедлайном.
- `ReadMessage` возвращает буфер из внутреннего `sync.Pool`; потребители (`tunnel`) передают его в `framing.ReturnBuffer`, то есть пулы двух пакетов не разделены строго.
- `Dial` всегда открывает ровно один стрим (`OpenStreamSync`); мультиплексирование реализуется на уровне `tunnel`, а не QUIC-стримов.
