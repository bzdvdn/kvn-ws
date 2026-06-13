# Client Relay Mode

## Scope Snapshot

- In scope: добавление третьего режима работы (`mode: "relay"`) в Go-клиент kvn-ws, который принимает входящие KVN-соединения от других клиентов и форвардит их на upstream KVN-сервер, работая как прозрачный KVN-бридж.
- Out of scope: полноценный multi-hop relay / chain proxy, собственный session management на relay, изменения в серверной части, Android-клиент.

## Цель

Администратор, управляющий инфраструктурой обхода блокировок, получает возможность разместить relay-узел в доверенной сети (с белым IP, whitelist-доступом к VPN-серверу). Клиенты из внешних сетей подключаются к relay (у которого может быть менее строгий whitelist или другой транспорт), а relay уже пересылает трафик на основной сервер. Результат: клиент не подключается к серверу напрямую — вместо этого relay транзитом пробрасывает соединение, прозрачно для handshake, шифрования и маршрутизации.

## Основной сценарий

1. Оператор конфигурирует relay: `mode: relay`, `relay.listen: 0.0.0.0:443` (как сервер), `server: wss://upstream-server.example.com/tunnel` (как клиент).
2. Relay запускается: открывает TLS listener на `relay.listen`, ждёт входящих WebSocket/QUIC-подключений от клиентов.
3. Клиент подключается к relay по WS (P1; QUIC — P2, будущее) — выполняет ClientHello, handshake, auth.
4. Relay принимает соединение, но **не обрабатывает handshake самостоятельно**. Вместо этого relay открывает второе соединение к upstream-серверу, пересылает ClientHello, дожидается ServerHello, и возвращает его клиенту. После handshake relay входит в режим прозрачного bridge: всё, что приходит от клиента → upstream; всё, что приходит от upstream → клиенту.
5. Результат: клиент не знает о relay — его сессия создана на upstream-сервере, relay — прозрачный pipe.
6. При обрыве одного из соединений relay закрывает оба. При переподключении клиента relay повторяет процедуру.

## User Stories

- P1 (MVP): relay для WS (TCP/TLS) транспорта с полным handshake forwarding. Клиент подключается к relay — relay создаёт пару `client↔relay`, `relay↔upstream` и bridge.
- P2: relay для QUIC транспорта.

## MVP Slice

- P1: WS-only relay с TLS listener + upstream dial, прозрачный bridge после handshake.
- Закрывает: AC-001 (forward handshake), AC-002 (data relay + disconnect propagation).

## First Deployable Outcome

После первого implementation pass: оператор запускает relay с `mode: relay`, клиент подключается к relay (вместо прямого сервера) по WS — handshake проходит, пинги ходят, трафик идёт. Проверить через `tcpdump`: клиент НЕ соединяется с upstream-сервером напрямую, relay — единственная точка входа.

## Scope

- Новый `mode: "relay"` в `ClientConfig.Mode`
- Новый блок `relay:` в `ClientConfig`: `listen`, `ws_paths`, `tls` (сертификат/ключ relay)
- Новый пакет или функция в `src/internal/bootstrap/client/` — `runRelayMode`
- Прозрачный bridge для входящих WS-соединений: read from client → write to upstream, read from upstream → write to client
- Полный forward handshake (ClientHello → upstream → ServerHello → client) без модификации фреймов
- Проброс keepalive/PING в обе стороны
- Disconnect propagation: обрыв с любой стороны закрывает обе
- Поддержка существующих конфигурационных опций: `obfuscation`, `crypto`, `tls.sni`, `auth.token` — relay использует их для upstream-соединения

## Контекст

- В клиенте уже есть `mode: "proxy"` и `mode: ""` (TUN) — `client.go:129-170`.
- Клиент уже умеет принимать входящие подключения в proxy mode (SOCKS5/HTTP listener).
- Функция `dialStream()` (`dial.go:20`) уже делает WS/QUIC-подключение с полным набором обфускаций — используется в `reconnectLoop`.
- Relay mode — это `dialStream()` + accept-слушатель + bridge loop.
- TLS-конфиг для входящих соединений relay — это отдельный серверный сертификат (или можно переиспользовать `tls.Config` из серверной части `tlspkg.NewServerTLSConfig`).
- QUIC relay — отложен на P2, т.к. требует демультиплексации сессий на одном QUIC listener.
- Relay НЕ управляет TUN, NAT, IP pool, session store, routing, kill switch, system proxy — это upstream server отвечает за всё.
- Существующий reconnect loop (`reconnectLoop`) не подходит для relay — relay не делает reconnect к upstream при обрыве каждого клиента; он управляет отдельным upstream-соединением на входящий клиент.

## Требования

- RQ-001 Клиент в режиме `mode: "relay"` ДОЛЖЕН открыть TLS listener на адресе `relay.listen`.
- RQ-002 Relay ДОЛЖЕН валидировать входящий WS path по allowlist `relay.ws_paths` (по умолчанию `["/tunnel"]`). Path, не входящий в список, отвечает 404 до WebSocket upgrade.
- RQ-003 При получении нового входящего WS-соединения relay ДОЛЖЕН открыть upstream-соединение к `server` через `dialStream()`, используя все активные опции обфускации (`obfuscation`, `utls`, `padding`, `sni`, `crypto`).
- RQ-004 Relay ДОЛЖЕН форвардить ClientHello от клиента к upstream, а ServerHello от upstream обратно клиенту, без модификации содержимого фреймов.
- RQ-005 После handshake relay ДОЛЖЕН работать как прозрачный bridge: двунаправленное копирование сообщений (`ReadMessage → WriteMessage`), без интерпретации фреймов.
- RQ-006 При обрыве входящего соединения relay ДОЛЖЕН закрыть соответствующее upstream-соединение; при обрыве upstream — закрыть клиентское соединение.
- RQ-007 Relay ДОЛЖЕН поддерживать добавление TLS-конфигурации сервера через блок `relay.tls` (cert, key, или auto-cert).
- RQ-008 Если блок `relay.tls` не указан в конфиге, relay ДОЛЖЕН автогенерировать self-signed сертификат при старте и записать предупреждение в лог.
- RQ-009 Relay НЕ ДОЛЖЕН запускать TUN, NAT, session pool, admin API, DNS proxy, system proxy, kill switch — режим исключает весь нерелевантный bootstrap.
- RQ-010 Relay ДОЛЖЕН логировать каждое входящее подключение (remote addr, session id после handshake, upstream addr).
- RQ-011 Relay ДОЛЖЕН использовать `relay.max_connections` (по умолчанию 100) для ограничения числа одновременных bridge-сессий.

## Вне scope

- QUIC транспорт для relay (P2).
- Многопоточный/мультиплексный relay (один listener, много bridge-сессий через горутины — в scope, но без батчинга).
- Сериализация/десериализация фреймов на relay (relay работает как opaque pipe — не интерпретирует фреймы).
- Health checking upstream — если upstream недоступен, relay отклоняет входящее соединение.
- Relay-цепочки (relay → relay → server).
- Изменения в `src/internal/bootstrap/server/` — relay только клиентская функциональность.

## Критерии приемки

### AC-001 Forward handshake

- Почему это важно: relay должен прозрачно передавать handshake между клиентом и upstream, чтобы upstream создал сессию.
- **Given** relay запущен с `mode: relay`, `server: wss://upstream/tunnel`
- **When** клиент подключается к relay (на `relay.listen`) и отправляет ClientHello
- **Then** relay открывает upstream-соединение, передаёт ClientHello, получает ServerHello, возвращает его клиенту
- Evidence: в логах relay `handshake forwarded: session <id>`; upstream видит сессию от IP relay, а не от IP клиента

### AC-002 Data bridge + disconnect propagation

- Почему это важно: после handshake relay должен быть прозрачным pipe, а обрыв одной стороны не должен висеть вечно.
- **Given** handshake завершён, bridge active
- **When** клиент отправляет data-фрейм
- **Then** relay передаёт его upstream без изменений; ответ upstream возвращается клиенту
- **And** если клиент закрывает соединение — upstream-соединение закрывается в течение timeout
- **And** если upstream закрывает — клиентское соединение закрывается
- Evidence: tcpdump на relay показывает идентичные payloads на обоих плечах; `ss` показывает 0 открытых соединений после закрытия клиента

### AC-003 Конфигурация relay

- Почему это важно: оператор должен настраивать relay без путаницы с существующими опциями.
- **Given** конфиг:
  ```yaml
  mode: relay
  relay:
    listen: 0.0.0.0:443
    ws_paths:
      - /tunnel
      - /api/v1/events
    max_connections: 200
    tls:
      cert: /etc/relay/cert.pem
      key: /etc/relay/key.pem
  server: wss://upstream.example.com/tunnel
  ```
- **When** клиент запускается
- **Then** relay слушает `0.0.0.0:443` с TLS, `max_connections: 200`, при подключении клиента коннектится к `wss://upstream.example.com/tunnel`
- Evidence: `ss -tlnp | grep 443` показывает relay; relay логи `listening on 0.0.0.0:443`

### AC-004 Relay reject on upstream failure

- Почему это важно: если upstream недоступен, relay не должен принимать соединение — клиент получит немедленную ошибку вместо таймаута.
- **Given** upstream-сервер недоступен
- **When** клиент подключается к relay
- **Then** relay пытается открыть upstream-соединение, не может — и закрывает клиентское соединение с ошибкой
- Evidence: клиент получает `connection refused` / timeout; в логах relay `upstream dial failed, rejecting client`

### AC-005 Bridge без интерпретации фреймов

- Почему это важно: relay не должен зависеть от версии протокола — новые типы фреймов проходят прозрачно.
- **Given** bridge active между клиентом и upstream
- **When** клиент отправляет фрейм неизвестного типа (например, `FrameTypeControl=0x03`)
- **Then** relay передаёт его upstream как есть, без ошибки
- Evidence: relay не логирует `unknown frame type`; upstream получает оригинальный бинарный фрейм

## Допущения

- Relay работает только с WS (TCP/TLS) транспортом на P1; QUIC — P2.
- Relay не требует изменений на стороне сервера — upstream обрабатывает соединение как обычное клиентское.
- TLS listener relay использует отдельный сертификат (не путать с upstream cert).
- Клиенты, подключающиеся к relay, не обязаны знать о его существовании — relay прозрачен.
- `relay.max_connections` — глобальный лимит; при достижении новые входящие соединения сразу закрываются.
- `relay.ws_paths` по умолчанию `["/tunnel"]` — тот же дефолт, что у сервера.
- Relay не требует авторизации для входящих соединений — auth выполняется на upstream через ClientHello token.

## Критерии успеха

- SC-001 Relay запускается и обслуживает 100 одновременных bridge-сессий без утечки горутин.
- SC-002 Пропускная способность bridge <5% потерь относительно прямого соединения (измерять через iperf-style тест).

## Краевые случаи

- **TLS cert не указан**: relay автогенерирует self-signed + логирует WARNING.
- **Оба соединения (client и upstream) закрыты одновременно**: race condition, relay не должен вызывать double-close.
- **Клиент подключается, но не шлёт ClientHello в течение timeout**: relay закрывает по таймауту (30s).
- **Upstream отвечает AuthError вместо ServerHello**: relay форвардит AuthError клиенту без изменений.
- **Max connections исчерпан**: relay закрывает новое входящее соединение сразу после accept.

## Открытые вопросы

1. Нужен ли relay `tls.verify_mode: "insecure"` по умолчанию для self-signed? — Решено: если relay использует self-signed, логировать WARNING.
2. Поддержка mTLS для relay listener? — Отложено: если понадобится, добавим `relay.tls.client_ca`.
3. Может ли relay использовать тот же TLS-конфиг, что и сервер из `tlspkg`? — Да, `tlspkg.NewServerTLSConfig` переиспользуется.

Готово к: /speckeep.inspect client-relay-mode
