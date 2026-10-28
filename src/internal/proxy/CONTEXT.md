# Context — src/internal/proxy

## Purpose

Локальный SOCKS5/HTTP CONNECT/transparent listener для v2 client. Отдаёт установленное TCP-соединение callback'у `client.runProxy`, который открывает отдельный v2 HTTP/2 stream на target.

## Responsibilities

- Владеет: `Listener` (detect protocol, SOCKS5 no-auth/user-pass, HTTP CONNECT, Linux `SO_ORIGINAL_DST`), семафором конкурентности и `prependConn`.
- Не владеет: v2 транспортом, маршрутизацией direct/server и TUN.

## How it works

- `NewListener(addr, auth, onConn, concurrency...)`: default `127.0.0.1:2310`, 1000 concurrent.
- Первый байт выбирает SOCKS5 (`0x05`), HTTP CONNECT (`C`) или transparent (Linux, если включён).
- SOCKS5 поддерживает IPv4/IPv6/domain и RFC 1929 auth; IPv6 CONNECT отклоняется.
- HTTP CONNECT отвечает `200` до фактического dial; `onConn` получает already-buffered connection.
- Transparent работает только Linux/IPv4; на других ОС возвращает ошибку.

## Public contracts

- `NewListener`, `Start`, `AcceptLoop`, `Close`, `Addr`, `SetTransparent`, `SetLogFn`.
- `ProxyAuth{Username, Password}`; `ErrInvalidSOCKSVersion`, `ErrUnsupportedCmd`, `ErrUnsupportedAddrType`.

## Gotchas

- SOCKS5 auth fallback: no-auth принимается, даже если задан пароль.
- Успешный SOCKS/HTTP ответ отправляется до dial target; ошибка видна как закрытие соединения.
- Transparent требует `*net.TCPConn`; обёрнутый conn не подходит.
