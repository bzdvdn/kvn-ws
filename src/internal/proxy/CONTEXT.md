# Context — src/internal/proxy

## Purpose

Локальный прокси-слой клиента: listener, принимающий SOCKS5, HTTP CONNECT или прозрачные TCP-соединения, и менеджер proxy-стримов, которые мультиплексируются поверх одного `transport.StreamConn` кадрами `FrameTypeProxy`.

## Responsibilities

- Владеет: `Listener` (детект протокола, SOCKS5/HTTP-CONNECT/transparent, семафор конкурентности), `Stream`/`Manager` (per-stream упорядоченная запись, маппинг stream ID → `net.Conn`), `SessionStreams` (контейнер стримов сессии), `NewStreamID`.
- Не владеет: транспортом (`transport`), форматом кадра (`framing`), решением «direct vs tunnel» (`routing`/`bootstrap/client`), обработкой кадров на серверной стороне (`tunnel`).

## How it works

- `NewListener(addr, auth, onConn, concurrency...)`: дефолты `127.0.0.1:2310` и 1000 одновременных клиентов; `onConn(conn, dst)` вызывается после успешного handshake и передаёт владение соединением.
- `AcceptLoop` берёт слот семафора и запускает `handleClient`; первый байт определяет протокол: `0x05` → SOCKS5, `'C'` → HTTP CONNECT, иначе (при `transparent`) → `SO_ORIGINAL_DST`.
- SOCKS5: версия 5, метод no-auth или RFC 1929 user/pass; адреса IPv4/IPv6/domain; IPv4-mapped IPv6 приводится к IPv4; «чистый» IPv6-CONNECT отклоняется с `host unreachable`; ответ содержит BND.ADDR локального сокета listener'а.
- HTTP CONNECT: запрос читается через `bufio` (первый байт подставляется обратно), клиенту сразу пишется `200 Connection Established`, затем `onConn` получает соединение с уже прочитанными байтами.
- Transparent (Linux): `getsockopt(IPPROTO_IP, SO_ORIGINAL_DST)` через `SyscallConn`; поддерживается только `AF_INET`; на прочих ОС возвращается ошибка. Первый прочитанный байт возвращается через `prependConn`.
- `Stream.ForwardToStream`: читает локальный сокет (4КБ), упаковывает `stream_id(4)+dst_len(2)+dst+data` в `FrameTypeProxy` и пишет в stream с дедлайном 120с.
- `Manager.Add` запускает per-stream writer; `HandleIncomingFrame` разбирает тот же формат, `len(data)==0` трактуется как закрытие стрима; данные кладутся в `writeCh` (буфер 64) блокирующей отправкой — backpressure вместо потери.
- `SessionStreams` — потокобезопасная карта `stream_id → net.Conn` на сессию; `CloseAll` закрывает все и пересоздаёт карту.
- `NewStreamID` — глобальный `atomic.AddUint32`, ID уникальны в пределах процесса.

## Key files

- `src/internal/proxy/listener.go` — `Listener`, SOCKS5/HTTP/transparent, `prependConn`.
- `src/internal/proxy/listener_linux.go` — `getOriginalDst` через `SO_ORIGINAL_DST`.
- `src/internal/proxy/listener_other.go` — заглушка transparent для не-Linux.
- `src/internal/proxy/stream.go` — `SessionStreams`, `Stream`, `Manager`, `NewStreamID`.

## Public contracts

- `NewListener(addr string, auth *ProxyAuth, onConn func(net.Conn, string), concurrency ...int)`, `Start`, `AcceptLoop`, `Close`, `Addr`, `SetTransparent`, `SetLogFn`.
- `ProxyAuth{Username, Password}`; ошибки `ErrInvalidSOCKSVersion`, `ErrUnsupportedCmd`, `ErrUnsupportedAddrType`.
- `NewManager(stream StreamConn, logfn ...)`, `Add`, `Get`, `Remove`, `HandleIncomingFrame`; `Stream{ID, Dst, Local}`, `ForwardToStream`.
- `NewSessionStreams()`, `Load/Store/Delete/CloseAll`; `NewStreamID() uint32`.
- Формат payload `FrameTypeProxy` (`id`, `dst_len`, `dst`, `data`) — общий контракт с `tunnel.Session`.

## Dependencies

- `src/internal/transport` (алиас `StreamConn`), `src/internal/transport/framing`; стандартная библиотека.

## Consumed by

- `src/internal/bootstrap/client/proxy.go` — локальный SOCKS/HTTP/transparent listener и `proxy.Manager` на слот.
- `src/internal/tunnel/session.go` — `proxy.SessionStreams` для учёта стримов сессии.
- `src/internal/bootstrap/server/handler.go` — создаёт `proxy.NewSessionStreams()`.

## Gotchas

- Если `auth` задан, но клиент предлагает только no-auth, listener **принимает no-auth** (fallback ради совместимости) — пароль фактически не обязателен.
- SOCKS5 отвечает успехом до фактического подключения к цели: `onConn` вызывается уже после `REP=success`, поэтому ошибка dial видна клиенту только как закрытие или молчание.
- HTTP CONNECT пишет `200` ещё до проверки `onConn`; при отсутствии обработчика соединение просто закрывается после «успешного» ответа.
- Raw IPv6-CONNECT намеренно отклоняется (`host unreachable`) — бинарник/vps без IPv6-маршрута; IPv4-mapped адреса при этом разрешены.
- Transparent работает только на Linux и только для IPv4 (`AF_INET`); на macOS/Windows вернёт ошибку, и соединение закроется.
- `HandleIncomingFrame` при переполнении `writeCh` блокирует читающий цикл — это осознанный backpressure, но медленный локальный сокет может затормозить обработку кадров.
- `Stream.stopWriter` закрывает `writeCh`, а writer-горутина выходит по `range`; повторный `Remove` безопасен благодаря `sync.Once`, но запись в закрытый канал после этого паникует.
- `NewStreamID` глобальный и не сбрасывается между сессиями — при долгой работе возможно переполнение `uint32` (теоретически).
- `getOriginalDst` требует именно `*net.TCPConn`; обёрнутый `prependConn`/`cancelOnContextConn` приведёт к ошибке (используется `Unwrap` только в WS-диалоге).
