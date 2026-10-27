# Stealth surface hardening

## Scope Snapshot

- In scope: случайный padding для малых кадров, decoy HTTP-ответы, jitter keepalive/reconnect, реалистичные WS-заголовки — всё opt-in и без поломки старых peer'ов.
- Out of scope: смена протокола на HTTP/2/3, telemetry-envelope, split-connection, Android/relay, изменение дефолтов padding/ALPN.

## Цель

Снизить пассивный и активный fingerprint текущего WS/TLS-туннеля, не меняя дефолтное поведение и не ломая старых клиентов. Пользователь получает более правдоподобный трафик и ответы API, а скорость не падает.

## Основной сценарий

1. Оператор оставляет `padding.mode: fixed` — всё работает как раньше, включая старые клиенты.
2. Оператор включает `padding.mode: random`; новые клиенты падят малые кадры случайно, крупные — без выравнивания до фиксированного размера. Старый peer с `fixed` продолжает работать: формат `[4B len][payload][padding]` не меняется.
3. Сервер на не-WS запросы к `/`, `/api/v1/events`, `/api/v1/health` отвечает правдоподобным JSON/401, а не 404 туннеля.
4. Keepalive/reconnect клиента добавляют jitter; сервер по умолчанию ведёт себя как раньше.

## User Stories

- P1: как пользователь, я хочу, чтобы мой туннель не выглядел как кастомный бинарный WS с фиксированным padding.
- P2: как оператор, я хочу включить hardening без обновления всех клиентов сразу.
- P3: как клиент со старым бинарником, я не должен потерять соединение после обновления сервера.

## MVP Slice

- `AC-001` + `AC-002` + `AC-003`: random padding и полная mixed-version совместимость.
- Decoy/jitter/headers подключаются тем же релизом, но проверяются отдельно.

## First Deployable Outcome

- Новый сервер и новый клиент с `padding.mode: random` работают; старый клиент с `fixed` и новый клиент со старым сервером — тоже. Decoy-ответы не влияют на WS-подключения.

## Scope

- Конфиг `obfuscation.padding.mode`: `fixed` (default) | `random`.
- Политика random padding: малые кадры — случайный размер в заданном диапазоне; крупные — без выравнивания до `padding.size`.
- Decoy HTTP-ответы для не-WS запросов на сервере.
- Реалистичный профиль WS-заголовков на клиенте (opt-in, default legacy).
- Настраиваемые alias-имена для no-pad заголовков и multiplex subprotocol; дефолты — старые имена.
- Jitter для keepalive/reconnect на клиенте; серверный keepalive по умолчанию без изменений.
- Alias-негоциация no-pad: клиент отправляет список имён, сервер принимает любое из своего списка и отвечает совпавшим именем.
- Тесты mixed-version и бенчмарк throughput/allocs.
- Настройки в kvn-web: per-server и global поля для mode/aliases/headers/jitter.

## Контекст

- Текущий формат padding: `[uint32 len][payload][random tail]`; reader использует только `len`, поэтому `fixed` и `random` **взаимно совместимы**, если padding включён у обеих сторон.
- `padding.enabled: false` уже является «off»-режимом и требует согласия сторон; новый `off` не вводим.
- `multiplex: false` по умолчанию, поэтому `kvn-ws-mux` не отправляется; сохраняем поведение.
- `X-KVN-NoPad` и `kvn-ws-mux` находятся внутри TLS (HTTP upgrade), поэтому пассивный DPI без TLS-интерцепта их не видит. Alias-имена помогают против HTTP-слоя/CDN и позволяют уйти от конкретных строк, не ломая старых peer'ов: списки имён на клиенте и сервере, дефолт — старые имена.
- Decoy-ответы видны активному пробнику и CDN; WS-клиенты их не запрашивают.

## Зависимости

- `src/internal/transport/websocket` (padding, handshake headers, subprotocol).
- `src/internal/config` (`PaddingCfg`, client/server defaults).
- `src/internal/bootstrap/server` (HTTP mux, decoy handlers).
- `src/internal/bootstrap/client` (headers, keepalive/reconnect jitter).

## Требования

- RQ-001 `padding.mode` ДОЛЖЕН по умолчанию быть `fixed`; существующие конфиги ведут себя байт-в-байт как раньше.
- RQ-002 `random` ДОЛЖЕН сохранять формат `[len][payload][tail]` и быть совместимым с `fixed` на другой стороне.
- RQ-003 Старый клиент без новых заголовков ДОЛЖЕН подключаться к новому серверу без изменений в handshake.
- RQ-004 Новый клиент ДОЛЖЕН откатываться на legacy-поведение, если сервер не подтверждает новый режим.
- RQ-005 Decoy-ответы НЕ ДОЛЖНЫ перехватывать/ломать валидный WS upgrade.
- RQ-006 Jitter ДОЛЖЕН быть ограничен настраиваемым диапазоном и не влиять на протокол.
- RQ-007 Реалистичные заголовки НЕ ДОЛЖНЫ ломать origin-check и старые серверы.
- RQ-008 Throughput случайного padding НЕ ДОЛЖЕН быть хуже fixed при том же профиле трафика.
- RQ-009 Списки no-pad заголовков ДОЛЖНЫ сохранять дефолт `X-KVN-NoPad` и поддерживать mixed-version в обе стороны.
- RQ-010 Списки multiplex subprotocol ДОЛЖНЫ сохранять дефолт `kvn-ws-mux` и не ломать старых клиентов.

## Вне scope

- HTTP/2 WebSocket, HTTP/3, telemetry-envelope, protobuf-обёртки.
- Split-connection / tun2socks.
- Android-клиент, relay bridge/terminator.
- Изменение дефолтного ALPN и семантики no-pad; меняются только имена заголовков через alias-списки.
- Отключение padding по умолчанию.
- Cover traffic и тяжёлый traffic shaping.

## Критерии приемки

### AC-001 Random padding для малых кадров

- Почему важно: фиксированный 512 — сильный размерный fingerprint.
- **Given** клиент и сервер с `padding.mode: random`
- **When** отправляется кадр меньше порога
- **Then** размер WS-сообщения случаен в заданном диапазоне, payload доставляется без искажений
- Evidence: unit-тест padding roundtrip + проверка нескольких разных размеров.

### AC-002 Random ↔ fixed совместимы

- Почему важно: нельзя требовать одновременного обновления.
- **Given** новый клиент с `random`, сервер со старым `fixed`
- **When** идёт обмен кадрами
- **Then** обе стороны читают payload, ошибок padding нет
- **Given** старый клиент с `fixed`, сервер с `random`
- **Then** тоже работает
- Evidence: mixed-version integration-тест в обе стороны.

### AC-003 Legacy default не меняется

- Почему важно: старые клиенты и конфиги не должны заметить обновление.
- **Given** `padding.mode` не задан
- **When** выполняется существующий padding/no-pad набор тестов
- **Then** все тесты зелёные, wire-размеры как раньше
- Evidence: `go test ./src/internal/transport/websocket/` + существующие integration-тесты.

### AC-004 Decoy API не ломает WS

- Почему важно: сервер не должен выглядеть как «дыра», но и не мешать клиентам.
- **Given** обычный GET `/api/v1/events` без Upgrade и валидный WS upgrade
- **When** приходят оба запроса
- **Then** GET получает правдоподобный JSON/401, WS upgrade проходит как раньше
- Evidence: httptest-тест на оба запроса.

### AC-005 Jitter ограничен и не ломает keepalive

- Почему важно: ровный интервал — fingerprint, но слишком большой jitter рвёт таймауты.
- **Given** base interval и jitter%
- **When** генерируется серия интервалов
- **Then** все значения в `[base-jitter, base+jitter]`, keepalive-тест проходит
- Evidence: unit-тест распределения/границ.

### AC-006 Реалистичные заголовки opt-in

- Почему важно: старые серверы не должны отклонять клиента из-за новых заголовков.
- **Given** `headers.profile: browser` и старый сервер
- **When** выполняется WS handshake
- **Then** upgrade успешен, сервер получает заголовки, origin-check не ломается
- Evidence: httptest с захватом заголовков + mixed-version тест.

### AC-007 Нет custom subprotocol по умолчанию

- Почему важно: `kvn-ws-mux` — явный маркер.
- **Given** `multiplex: false`
- **When** клиент подключается
- **Then** `Sec-WebSocket-Protocol` отсутствует; при `multiplex: true` старое поведение сохраняется
- Evidence: unit-тест handshake.

### AC-008 Нет просадки throughput

- Почему важно: hardening не должен ухудшать скорость.
- **Given** одинаковый поток кадров в fixed и random
- **When** выполняется benchmark
- **Then** throughput random не хуже fixed более чем на 5%, allocs не растут
- Evidence: benchmark-результат в отчёте.

### AC-009 Alias no-pad заголовков совместим со старыми

- Почему важно: уйти от конкретного имени, не сломав secondary-клиенты.
- **Given** сервер с `no_pad_headers: ["X-KVN-NoPad", "X-Client-Flags"]`, старый клиент шлёт `X-KVN-NoPad`
- **When** открывается secondary-соединение
- **Then** сервер подтверждает `X-KVN-NoPad`, padding снимается
- **Given** новый клиент шлёт оба имени, старый сервер принимает только `X-KVN-NoPad`
- **Then** подтверждение приходит старым именем, клиент снимает padding
- **Given** ни одно имя не подтверждено
- **Then** клиент остаётся padded, соединение работает
- Evidence: mixed-version unit/integration-тест.

### AC-010 Alias multiplex subprotocol

- Почему важно: `kvn-ws-mux` — конкретная строка, но от неё зависит старый multiplex.
- **Given** новые списки `multiplex_subprotocols` с дефолтом `["kvn-ws-mux"]`
- **When** старый клиент предлагает `kvn-ws-mux`
- **Then** сервер его принимает
- **When** новый клиент предлагает новый alias первым
- **Then** сервер выбирает поддерживаемый alias, handshake проходит
- Evidence: unit-тест subprotocol negotiation.

## Допущения

- Пассивный DPI без TLS-интерцепта видит TLS ClientHello, SNI/ALPN, размеры/тайминг пакетов; HTTP-заголовки и бинарные фреймы под TLS ему не видны.
- Оператор согласует `padding.enabled`; `random` не требует согласования режима, только включённого padding.
- Старые peer'ы не валидируют общий размер padding-сообщения, только length prefix.
- Новые заголовки и jitter включаются конфигом и по умолчанию выключены/legacy.

## Критерии успеха

- SC-001 Mixed-version матрица (old/new client × old/new server) проходит без ошибок.
- SC-002 Random padding снижает средний размер малых сообщений по сравнению с fixed 512.
- SC-003 Throughput/alloc regression ≤5%.

## Краевые случаи

- `padding.enabled: false` — поведение как раньше, `mode` игнорируется.
- `padding.mode: random` при старом peer'е — работает как fixed для чтения, пишет random; ошибок нет.
- Клиент с `headers.profile: browser` к серверу со строгим origin-check — Origin должен быть согласован.
- Jitter не должен уменьшать интервал ниже минимума keepalive.
- WS upgrade с неверным `Sec-WebSocket-Key` — decoy-обработчик не должен маскировать ошибку под успех.

## Открытые вопросы

- none
