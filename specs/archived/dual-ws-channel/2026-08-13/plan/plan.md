# Dual WebSocket Channel — План

## Phase Contract

Inputs: `specs/active/dual-ws-channel/spec.md`, `inspect.md` (pass), минимальный repo-контекст.
Outputs: `plan.md`, `data-model.md`.
Stop if: spec расплывчата для безопасного планирования.

## Цель

Реализовать второй WS-канал одного клиента/сессии на Go-стеке (kvn-web/CLI, TUN-режим): UDP-пакеты уходят отдельным стримом, избавляя VoIP-медиа от head-of-line blocking общего потока. Ключевая точка оптимизации — единый `tunnel.Session` обслуживает и клиент (через `SetTunRouter`), и сервер (через `SetDemux`), поэтому мульти-стрим вводится один раз и работает на обеих ролях.

## MVP Slice

Клиент с двумя каналами → сервер: primary (TCP/ICMP/остальное) + secondary (UDP), привязка вторичного по `session_id`, фоллбэк на одиночный канал. Затрагивает `AC-001, AC-002, AC-003, AC-004, AC-005`.

## First Validation Path

Интеграционный тест в `src/integration/tunnel_integration_test.go`: два WS-подключения к тестовому серверу, вторичный привязывается по `session_id`, UDP-кадр доходит до TUN сервера по secondary, TCP-кадр — по primary; при ошибке вторичного handshake первичный канал продолжает работать. Быстрая проверка: `go test -race ./src/integration/... ./src/internal/tunnel/...`.

## Scope

- `protocol/handshake.yaml` + codegen — новые tag'и ClientHello.
- `src/internal/protocol/handshake/` — encode/decode новых полей, обратная совместимость.
- `src/internal/tunnel/session.go` — мульти-стрим: `SetSecondary`, классификация по IP-протоколу, вторичный read-loop, буфер привязки.
- `src/internal/tunnel/demux.go` — helper классификации протокола для возвратного пути.
- `src/internal/bootstrap/client/tun.go`, `dial.go` — поднятие вторичного канала клиентом + фоллбэк.
- `src/internal/bootstrap/server/handler.go`, `server.go` — сregistry активных tunnel.Session, приём и привязка вторичного канала.
- Интеграционные тесты: `src/integration/`.
- Граница, которая остаётся нетронутой: proxy-режим, Android (OkHttp), QUIC, фреймовый формат `FrameTypeData`, schema auth.

## Performance Budget

- Per-packet: классификация IP-протокола — разбор заголовка (байт 9 IPv4 / next-header IPv6), ~нс, без аллокаций.
- `alloc/op` вторичного пути: переиспользуется существующий `tunReadBufPool`/`proxyBufPool`; новых аллокаций на пакет не добавлять, кроме обязательной crypto-копии (как сейчас).
- p99 latency data-path: не хуже текущего (запись в TUN < 5 мс p99 на тестовом стенде); secondary-path допускает тот же бюджет.
- Буфер привязки: ограничен по времени (300 мс) и по количеству пакетов (см. DEC-005).

## Implementation Surfaces

- `protocol/handshake.yaml` (существующий, changed) — константы `ChannelTag`, `SessionTag`.
- `src/internal/protocol/handshake/types_gen.go` (generated, regenerated codegen'ом) — поля `Channel`, `SessionId` в `ClientHello`.
- `src/internal/protocol/handshake/handshake.go` (существующий, changed) — encode/decode tag'ов.
- `src/internal/tunnel/session.go` (существующий, changed) — ядро: два стрима, классификация, вторичный read-loop, буфер.
- `src/internal/tunnel/demux.go` (существующий, changed) — `parseIPProto`.
- `src/internal/config/client.go` (существующий, changed) — `MultiChannel bool` (флаг включения).
- `src/internal/bootstrap/client/tun.go` (существующий, changed) — поднятие secondary после primary-handshake.
- `src/internal/bootstrap/client/dial.go` (существующий, changed) — переиспользование dialStream для второго соединения.
- `src/internal/bootstrap/server/handler.go` (существующий, changed) — ветка вторичного handshake.
- `src/internal/bootstrap/server/server.go` (существующий, changed) — реестр `tunnelSessRefs sync.Map`.
- `src/integration/tunnel_integration_test.go` (существующий, changed) — двухканальный сценарий.

## Bootstrapping Surfaces

`none` — вся требуемая структура в репозитории уже есть (transport, session, demux, handshake, codegen).

## Влияние на архитектуру

- Локально: `Session` переходит от одиночного `stream` к модели «primary + optional secondary», оба с собственными write-paths; единственная точка классификации (`tunToWS`) покрывает и клиента с split-tunnel, и сервер с демux.
- Интеграции: серверная привязка вторичного канала требует глобального реестра активных `*tunnel.Session` по `session_id` (новый shared state на `Server`).
- Compatibility/rollout: wire-совместимо в обе стороны (decoder уже skipping-теги); включение — флаг `multi_channel`, фоллбэк автоматический.
- Android/QUIC/proxy — не затрагиваются.

## Acceptance Approach

- AC-001 (привязка по session_id + токен) → surfaces: `handshake.go`, `server/handler.go`, `server.go`. Наблюдение: интеграционный тест — secondary получает ServerHello; канал с чужим токеном отклонён AuthError.
- AC-002 (UDP→secondary, остальное→primary) → surfaces: `session.go` (классификация + два write-path), `client/tun.go`. Наблюдение: интеграционный тест счётчиками по стримам.
- AC-003 (возврат в правильный стрим) → surfaces: `session.go` (tunToWS), `demux.go` (`parseIPProto`). Наблюдение: возвратный UDP-ответ приходит клиенту по secondary.
- AC-004 (фоллбэк и падение secondary) → surfaces: `client/tun.go`, `session.go`. Наблюдение: unit/integration — ошибка вторичного handshake не прерывает primary; закрытие secondary переводит UDP на primary.
- AC-005 (буфер до привязки primary) → surfaces: `session.go`, `server/handler.go`. Наблюдение: unit-тест на буфер (копится 300 мс, затем flush/drop).
- AC-006 (идентичная обфускация и cipher) → surfaces: `client/tun.go`, `session.go`. Наблюдение: code review общий WSConfig/`sessionCipher`; unit-тест crypto поверх secondary.
- AC-007 (backward compat) → surfaces: `handshake.go`. Наблюдение: существующие handshake-тесты зелёные без правок.
- AC-008 (no regression) → `go test -race`, `go vet`.
- Зависимость от contracts/data model: wire-контракт ClientHello расширяется (секция «Данные и контракты»); persisted data model не меняется.

## Данные и контракты

- Связанные AC: `AC-001, AC-007`; DEC: `DEC-001, DEC-003`.
- Wire-contract ClientHello расширяется tag-based: `ChannelTag` (0x0C) + `SessionTag` (0x0D). Старый сервер при получении новых тегов корректно их пропускает (`handshake.go:83-93`); новый сервер при отсутствии тегов трактует канал как primary. Совместимость сохранена без версионирования.
- Persisted data model: не меняется (см. `data-model.md`, no-change stub).
- Новых API/event contracts не вводится.

## Стратегия реализации

- DEC-001 Текущий tag-based handshake расширяется, а не меняется
  Why: decoder `handshake.go:83-93` уже итерирует TLV-теги и пропускает неизвестные — добавление `ChannelTag`+`SessionTag` даёт обратную совместимость без версионирования протокола; это минимальный diff в коде и протоколе.
  Tradeoff: два новых байтовых тега в hello; старые клиенты/серверы валидны как есть.
  Affects: `handshake.yaml`, `handshake.go`, `types_gen.go`.
  Validation: `TestDecodeClientHello` с новыми и старыми hello (AC-007).

- DEC-002 Реестр активных `*tunnel.Session` на сервере по `session_id`
  Why: вторичный WS приходит отдельным HTTP-запросом (`handler.go:26`), а первичный создаёт и блокирует Session внутри `handleStream`; чтобы привязать второе подключение, нужен shared lookup активных Session.
  Tradeoff: новый mutable state на `Server`; освобождение по завершении первичного. Ключ — `session_id` (16 hex), совпадает с ключом `SessionManager`.
  Affects: `server.go`, `handler.go`, (чтение) `tunnel/session.go`.
  Validation: AC-001 — secondary находит primary-сессию; AC-004 — отсутствие записи закрывает secondary.

- DEC-003 Мульти-стрим живёт внутри `tunnel.Session`, классификация в `tunToWS`
  Why: `tunnel.Session` используется и клиентом (`tun.go:390`, split-tunnel через `SetTunRouter`), и сервером (`handler.go:196`, демux через `SetDemux`). Один `ipProto(pkt)` в `tunToWS` автоматически покрывает обе роли; `stream` остаётся primary, добавлен `secondary StreamConn` — control/proxy/DNS/keepalive остаются на primary.
  Tradeoff: вторичный канал несёт только `FrameTypeData` (UDP); серверный proxy/DNS-функционал не дублируется на secondary — намеренно.
  Affects: `session.go` (`Session`, `tunToWS`, новый вторичный read-loop), `demux.go`.
  Validation: AC-002/AC-003 — интеграционный тест распределения кадров.

- DEC-004 Установка каналов последовательная: primary → session_id → secondary; фоллбэк без конфига
  Why: `session_id` известен только после primary-handshake; вторичный hello обязан его нести. При любом сбое secondary — первичный продолжает работу без изменения конфигурации (RQ-005).
  Tradeoff: поднятие вторичного длится ещё один RTT handshake; фоллбэк означает кратковременный single-channel (допустимо).
  Affects: `client/tun.go`, `client/dial.go`, `session.go`.
  Validation: AC-004 — secondary-ошибка не ломает primary; AC-001 — успешная привязка.

- DEC-005 Буфер вторичного канала: лимит по времени и количеству, затем flush/drop
  Why: защита от гонки, когда secondary приходит раньше готовности primary-path (RQ-006/AC-005). Real-time UDP предпочитает свежие пакеты — буфер не должен копить старые.
  Tradeoff: первые UDP-пакеты в редком race могут быть сброшены; буфер ограничен 300 мс / 1024 пакета.
  Affects: `session.go` (secondary read-loop + `primaryReady` guard), `server/handler.go`.
  Validation: AC-005 — unit-тест: до ready копится, после — flush, при переполнении — drop.

- DEC-006 Включение фичи — флаг `multi_channel` (по умолчанию выключен в этой итерации)
  Why: снижает риск rollout: сервер без фичи не ломает клиентов; после N-недель стабильности флаг можно включить дефолтно в конфиге поставки.
  Tradeoff: сначала требуется осознанное включение; AC всё равно проверяются тестами независимо от флага.
  Affects: `config/client.go`, `client/tun.go`, `server.go`.
  Validation: AC-008 — тесты зелёные и с флагом, и без; флаг не меняет поведение single-channel.

## Incremental Delivery

### MVP (Первая ценность)

- Протокольные теги в ClientHello (DEC-001) + мульти-стрим в `tunnel.Session` (DEC-003) + серверный реестр и приём вторичного (DEC-002) + клиентский подъём канала (DEC-004) + буфер (DEC-005).
- Критерий готовности MVP: AC-001…AC-005 зелёные в `go test -race ./src/integration/... ./src/internal/tunnel/...`.

### Итеративное расширение

- После MVP: флаг `multi_channel` на клиенте/сервере (DEC-006) и кросс-проверка backward compat (AC-007/AC-008). Валидация: фулл `go test -race ./...`, `go vet`.

## Порядок реализации

1. Сначала протокол: `handshake.yaml` → codegen → `handshake.go` (AC-007). Основа для всего; безопасно без остального.
2. Затем `tunnel.Session` мульти-стрим + `parseIPProto` в `demux.go` (DEC-003) — ядро.
3. Параллельно: серверный реестр + ветка вторичного handshake (DEC-002/005) и клиентский подъём secondary (DEC-004).
4. Флаг `multi_channel` и интеграционные тесты.
- Пункты 2 и 3 можно вести параллельно (независимые поверхности); пункт 1 строго первым.

## Риски

- Риск 1: гонка привязки secondary до готовности primary (сервер).
  Mitigation: DEC-005 буфер 300 мс / 1024 пакета; AC-005 unit-тест.
- Риск 2: вторичный канал без keepalive-маршрута — общий PING/PONG только на primary; inaktивный secondary может быть убит NAT'ом.
  Mitigation: secondary использует тот же `dialStream`/keepalive path, что и primary (GO: `SetKeepalive` на обоих); AC-008 regression.
- Риск 3: реестр `tunnelSessRefs` — утечка записей при аварийном завершении первичного.
  Mitigation: запись аннулируется в `defer` после `tunSess.Run` (`handler.go:199-213`); key=session_id, delete idempotent.
- Риск 4: неверная классификация IPv6 (next-header chaining).
  Mitigation: `parseIPProto` обрабатывает только фиксированные смещения (IPv4 [9], IPv6 [6]) и сложные extension headers никапсулируются в TUN untouched; fallback — primary канал.

## Rollout и compatibility

- Wire-совместимо с прошлыми клиентами/серверами (DEC-001). Новые поля необязательны; их отсутствие = primary.
- Включение фичи — флаг `multi_channel` (DEC-006); никакого migration/backfill не требуется.
- Monitoring: существующие metrics (throughput, sessions) + по желанию счётчик secondary-attach в логах-аудит.
- Отдельных rollout-шагов для Android/QUIC/proxy нет — они вне scope.

## Проверка

- Automated: `go test -race ./src/internal/... ./src/integration/...`, существующие handshake-тесты без правок (AC-007), новые unit-тесты (буфер, классификация, crypto cross-channel), интеграционный тест двухканального round-trip.
- Manual: `kvn-web` режим TUN с `multi_channel: true` — звонок Telegram/WhatsApp; лог: вторичный канал привязан, UDP уходит в secondary.
- Подтверждаемые критерии: AC-001…AC-008; DEC-001…DEC-006.

## Соответствие конституции

- нет конфликтов: Traceability (`@sk-task`/`@sk-test` над объявлениями), DDD/Clean (мульти-стрим внутри `tunnel.Session`, реестр на `Server`), Go 1.22+, языки docs=ru, comments=en — соблюдаются.