# QUIC Relay Mode

## Scope Snapshot

- In scope: расширение relay-режима для приёма входящих QUIC-соединений **параллельно** с WS (TCP/TLS). Relay работает как сервер — всегда слушает TLS (WS), и опционально QUIC (UDP). Клиент подключается к relay с автоматическим fallback (QUIC → WS).
- Out of scope: chain relay, изменения серверной части, Android-клиент, multi-stream мультиплексирование.

## Цель

Администратор размещает relay-узел, который принимает клиентов как по WS (TCP/TLS), так и по QUIC (UDP) — аналогично тому, как KVN-сервер обслуживает оба транспорта. Клиент автоматически выбирает транспорт (QUIC с fallback на WS) при подключении к relay. Результат: единый relay-узел, прозрачно обслуживающий оба транспорта без дублирования конфигурации.

## Основной сценарий

1. Оператор конфигурирует relay: `mode: relay`, `relay.listen: 0.0.0.0:443`, опционально `relay.quic: {}` для включения QUIC.
2. Relay запускается: открывает TLS listener (TCP) и, если `relay.quic` задан, QUIC listener (UDP) — оба на `relay.listen`.
3. Клиент подключается к relay: dialStream клиента пробует QUIC (UDP), при неудаче падает на WS (TCP/TLS).
4. Relay принимает соединение (WS upgrade или QUIC stream), читает ClientHello, открывает upstream-соединение к `server` через `dialStream()`, форвардит ClientHello, получает ServerHello, возвращает клиенту.
5. После handshake relay работает как прозрачный bridge: двунаправленное копирование сообщений.
6. При обрыве любого соединения relay закрывает оба.

## User Stories

- P1 (MVP): relay с WS (всегда) + QUIC (опционально) listener, transparent bridge для обоих транспортов.
- P2: ObfuscatedQUICConn (XOR-обфускация) для входящих QUIC-соединений.

## MVP Slice

- P1: WS listener (всегда) + QUIC listener (опционально, по конфигу) + accept + dialStream upstream + bridge loop.
- Клиент использует существующий fallback в dialStream.
- Закрывает: AC-001 (оба транспорта), AC-002 (data bridge), AC-003 (конфиг), AC-004 (opaque bridge), AC-005 (reject on upstream failure).

## Контекст

- Relay WS уже реализован: TLS listener + HTTP/ServeMux + WS Accept + bridge.
- Сервер уже использует dual-transport pattern: TLS listener (всегда) + QUIC listener (опционально) — оба в errgroup.
- QUIC-транспорт (`quic.Listen`/`quic.Dial`) уже реализован, StreamConn interface един для WS и QUIC.
- `dialStream()` уже умеет выбирать WS или QUIC и имеет fallback QUIC → WS — relay переиспользует её для upstream.
- Relay upstream не использует обфускацию/паддинг (raw forward) — это не меняется.
- В отличие от сервера, relay не имеет session management, routing, NAT — только bridge.

## Требования

- RQ-001 Relay ВСЕГДА ДОЛЖЕН открывать TLS listener (TCP) на `relay.listen` для WS-клиентов.
- RQ-002 Relay ДОЛЖЕН открывать QUIC listener (UDP) на `relay.listen`, если в конфиге присутствует блок `relay.quic` или `transport: quic`.
- RQ-003 Оба listener-а ДОЛЖНЫ работать конкурентно (errgroup или аналогичный механизм).
- RQ-004 Для WS: relay ДОЛЖЕН выполнять HTTP → WS upgrade, проверять path по `relay.ws_paths`.
- RQ-005 Для QUIC: relay ДОЛЖЕН принимать через `quicListener.Accept(ctx)` + `conn.AcceptStream(ctx)`.
- RQ-006 Для каждого принятого соединения relay ДОЛЖЕН читать ClientHello (30s timeout), открывать upstream через `dialStream()`, форвардить handshake, запускать bridge.
- RQ-007 После handshake relay ДОЛЖЕН работать как прозрачный bridge: `ReadMessage → WriteMessage` в обе стороны.
- RQ-008 При обрыве любого соединения relay ДОЛЖЕН закрыть оба (closeBoth через sync.Once).
- RQ-009 Relay ДОЛЖЕН использовать TLS-конфиг из `relay.tls` для обоих listener-ов (или self-signed при отсутствии).
- RQ-010 Relay ДОЛЖЕН использовать `relay.max_connections` (глобальный semaphore) для лимита bridge-сессий.
- RQ-011 Relay НЕ ДОЛЖЕН запускать TUN, NAT, session pool, admin API, DNS proxy, system proxy, kill switch.

## Вне scope

- Multi-stream QUIC мультиплексирование (1 QUIC-соединение = 1 stream).
- Chain relay (relay → relay → server).
- ObfuscatedQUICConn на incoming QUIC (P2).
- QUIC-серверная часть.
- Android-клиент.

## Критерии приемки

### AC-001 Оба транспорта (WS + QUIC)

- **Given** relay запущен с `mode: relay`, `relay.listen: 0.0.0.0:443`, блок `relay.quic: {}`
- **When** WS-клиент подключается к `wss://relay:443/tunnel` и QUIC-клиент подключается к `quic://relay:443`
- **Then** оба клиента проходят handshake и получают bridge
- **Evidence**: в логах relay `handshake forwarded: session <id>` для обоих; `ss -tlnp` и `ss -ulnp` показывают listener-ы

### AC-002 Data bridge + disconnect propagation

- **Given** handshake завершён, bridge active (любой транспорт)
- **When** клиент отправляет data-сообщение
- **Then** relay передаёт его upstream без изменений; ответ upstream возвращается клиенту
- **And** обрыв любого плеча закрывает оба
- **Evidence**: идентичные payloads на обоих плечах; 0 открытых соединений после закрытия

### AC-003 Конфигурация

- **Given** конфиг:
  ```yaml
  mode: relay
  relay:
    listen: 0.0.0.0:443
    ws_paths:
      - /tunnel
    max_connections: 200
    quic:
      keep_alive: 7s
      idle_timeout: 60s
    tls:
      cert: /etc/relay/cert.pem
      key: /etc/relay/key.pem
  server: wss://upstream.example.com/tunnel
  ```
- **When** relay запускается
- **Then** relay слушает TCP(TLS) 443 и UDP 443
- **Evidence**: `ss -tlnp | grep 443` + `ss -ulnp | grep 443`

### AC-004 Opaque bridge

- **Given** bridge active (любой транспорт)
- **When** клиент отправляет фрейм неизвестного типа
- **Then** relay передаёт его upstream как есть, без ошибки
- **Evidence**: relay не логирует `unknown frame type`

### AC-005 Relay reject on upstream failure

- **Given** upstream-сервер недоступен
- **When** клиент подключается к relay (любой транспорт) и шлёт ClientHello
- **Then** relay не может открыть upstream, закрывает клиента с ошибкой
- **Evidence**: в логах relay `upstream dial failed, rejecting client`

## Допущения

- TLS listener (WS) работает всегда; QUIC listener — опционально.
- Relay upstream через `dialStream()` — поддерживает WS и QUIC, fallback QUIC→WS.
- QUIC listener использует тот же TLS-конфиг, что и TLS listener (из `relay.tls`).
- `relay.max_connections` — глобальный semaphore на оба listener-а.
- ClientHello timeout — 30s, upstream dial timeout — через dialStream.
- ObfuscatedQUICConn (XOR) не входит в MVP.

## Открытые вопросы

1. Нужен ли отдельный блок `relay.quic` в конфиге или достаточно `transport: quic`? — Решено: блок `relay.quic` включает QUIC listener и задаёт его параметры (keep_alive, idle_timeout).
2. Поддержка mTLS для QUIC listener? — Отложено: если понадобится, добавить `relay.quic.client_ca`.
3. Раздельный лимит `max_connections` для WS и QUIC? — Нет, глобальный semaphore (как сейчас).

Готово к: /speckeep.inspect quic-relay-mode
