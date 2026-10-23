# Context — src/internal/protocol/handshake

## Purpose

Кодирование/декодирование стартового обмена KVN: `ClientHello`/`ServerHello` (версия, токен, MTU, транспорт, вторичный канал, batching, IPv4/IPv6-адреса, crypto-salt, gateway) и `AuthError`. Один и тот же wire-формат используется Go-клиентом, сервером, relay и Android.

## Responsibilities

- Владеет: бинарным layout'ом hello-кадров (`framing.Frame` с `FrameTypeHello`/`FrameTypeAuth`), TLV-тегами расширений и обратной совместимостью при их отсутствии.
- Не владеет: самим кадром (`framing`), TLS/транспортом, проверкой токена (`protocol/auth`), выводом ключей (`crypto`).

## How it works

- **ClientHello**: `protoVersion(1)`, `flags(1)` (`FlagIPv6=0x01`, `FlagMTU=0x02`), `tokenLen(2 BE)` + token, опционально `mtu(2 BE)` при флаге, затем TLV: `TransportTag=0x0B`, `ChannelTag=0x0C`, `SessionTag=0x0D` (`tag, len(1), value`) и `BatchTag=0x0E` (`tag,1,1`). Значения строк ограничены 255 байтами.
- `DecodeClientHello`: `Mtu` по умолчанию `DefaultMTU=1500`; неизвестные теги пропускаются; при некорректной длине тега разбор прекращается без ошибки.
- **ServerHello**: `sessionID` — 16 байт в hex, затем `count(1)` и `count` записей `[family(1), len(1), addr]` (IPv4 обязателен, IPv6 опционален), затем `mtu(2 BE)` **без флага присутствия**, затем TLV: `CryptoSalt=0x09`, `GatewayTag=0x0A` (4 байта), `TransportTag=0x0B`, `BatchTag=0x0E`.
- `EncodeServerHello` требует валидный 16-байтовый hex `SessionId` и IPv4 `AssignedIp`; salt — произвольной длины (на практике 32 байта).
- `DecodeServerHello` возвращает ошибку, если IPv4-адрес отсутствует; MTU читается, если после адресов остаётся ≥2 байт; TLV разбираются до конца payload.
- `AuthError` — кадр `FrameTypeAuth` с payload = сырая строка причины (без длины/тегов).
- `types_gen.go` генерируется из `protocol/handshake.yaml`; Kotlin-копия в `src/android/.../protocol/Handshake.kt` должна совпадать.

## Key files

- `src/internal/protocol/handshake/handshake.go` — все encode/decode функции.
- `src/internal/protocol/handshake/types_gen.go` — константы и структуры (не править руками).
- `protocol/handshake.yaml` — источник истины для констант/полей.

## Public contracts

- `EncodeClientHello(*ClientHello) (*framing.Frame, error)`, `DecodeClientHello(*framing.Frame) (*ClientHello, error)`.
- `EncodeServerHello(*ServerHello) (*framing.Frame, error)`, `DecodeServerHello(*framing.Frame) (*ServerHello, error)`.
- `EncodeAuthError(*AuthError)`, `DecodeAuthError(*framing.Frame)`.
- Структуры `ClientHello{ProtoVersion, Ipv6, Token, Mtu, Transport, Channel, SessionId, BatchSupport}` и `ServerHello{SessionId, AssignedIp, AssignedIpv6, Mtu, CryptoSalt, GatewayIp, Transport, BatchSupport}`.
- Константы `ProtoVersion=0x02`, `SessionIDLen=16`, `DefaultMTU=1500`, теги `0x09..0x0E`.

## Dependencies

- `src/internal/transport/framing`; стандартная библиотека (`encoding/binary`, `encoding/hex`, `net`).

## Consumed by

- `src/internal/bootstrap/{client,server,relay}`, `src/internal/tunnel` (косвенно через кадры), интеграционные и gate-тесты; Android-клиент повторяет формат.

## Gotchas

- `DecodeServerHello` читает MTU **безусловно**, если после адресов есть 2 байта: старый/сторонний сервер, который сразу кладёт TLV без MTU, будет прочитан как MTU из байтов тега.
- `EncodeServerHello` не сдвигает `pos` после `GatewayTag`, поэтому при одновременных `GatewayIp` и `Transport` транспортный тег пишет поверх gateway (а `total` остаётся с лишними 5 байтами) — gateway в таком hello теряется.
- `EncodeClientHello` не проверяет длину токена: `uint16(len(token))` переполнится на токене > 65535 байт.
- `DecodeClientHello` не проверяет `ProtoVersion` — несовместимая версия пройдёт молча, решение о совместимости принимает вызывающий.
- `AuthError` не имеет структуры/экранирования — reason передаётся как есть, включая возможные `\n`.
- Присутствие `BatchTag` без байта значения (`length==0`) даёт `BatchSupport=false`, а не ошибку.
- IPv4 в `ServerHello` обязателен: IPv6-only ответ закодировать/декодировать нельзя.
- Формат фиксирован для обеих сторон: изменения возможны только через `protocol/handshake.yaml` + кодогенерацию (Go и Kotlin).
