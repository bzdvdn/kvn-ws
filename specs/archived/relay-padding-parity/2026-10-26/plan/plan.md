# Relay: паритет padding и unpadded-переговоры — План

## Phase Contract

Inputs: `spec.md`, relay/client/server padding surfaces.
Outputs: `plan.md`; data model и contracts не меняются.
Stop if: spec потребует новых AC или изменения wire-формата.

## Цель

Свести relay padding к одному источнику правил (как у client/server), провести `X-KVN-NoPad` через оба WS-плеча и не допустить смешанного состояния в bridge. Работа локальна `src/internal/bootstrap/relay` + один accessor в `transport/websocket`; протокол и конфиг-схема не меняются.

## MVP Slice

- Единый helper padding-конфига и его применение в terminator (accept + upstream) и bridge.
- Закрывает `AC-001`, `AC-002`, `AC-003`, `AC-004`, `AC-006`, `AC-008`.
- `AC-005` и `AC-007` закрываются bridge-частью того же изменения и отдельными integration-сценариями.

## First Validation Path

1. `go test ./src/internal/bootstrap/relay/... ./src/internal/transport/websocket/...`.
2. Запуск нового bridge-теста: upstream с `AllowNoPad` → клиент `PaddingActive()==false`; upstream без `AllowNoPad` → клиент `PaddingActive()==true`.
3. Terminator-тест accept-сseam: ответ содержит `X-KVN-NoPad: 1` только при `realtime_unpadded: true`.

## Scope

- `src/internal/bootstrap/relay/padding.go` — единые правила enabled/size/realtime.
- `handler.go` / `bootstrap.go` — terminator accept и upstream WS dial.
- `bridge.go` — accept и per-connection upstream dial с согласованием обоих плеч.
- `src/internal/transport/websocket` — экспорт заголовка и accessor фактического состояния.
- `docs/ru|en/{relay,config}.md` — поведение padding relay.
- Не трогаем: wire-формат, QUIC-padding, client/server логику, другие relay-настройки (multiplex/MTU/batching/uTLS).

## Performance Budget

- Hot path не меняется: `PaddingActive()` — atomic load; согласование выполняется только на установке соединения.
- Bridge не добавляет копий сверх текущих `ReadMessage`/`WriteMessage`.
- Целевой alloc/op для padding hot path — без роста относительно текущего.

## Implementation Surfaces

- `src/internal/bootstrap/relay/padding.go` (new): `relayPaddingEnabled`, `relayPaddingSize`, `relayRealtimeUnpadded`; правила 1:1 с `bootstrap/client` и `bootstrap/server`.
- `src/internal/bootstrap/relay/handler.go` (existing): `terminatorWSConfig`, `AllowNoPad`; вынести accept в `acceptTerminatorWS` для тестируемости.
- `src/internal/bootstrap/relay/upstream.go` (existing): `dialAndHandshakeWS` получает `RequestNoPad` из helper, лог исхода через `PaddingActive`.
- `src/internal/bootstrap/relay/bridge.go` (existing): конфиг padding в `relayWSHandler`, предварительный dial upstream при запросе no-pad, передача готового conn в `bridgeRelayConn`.
- `src/internal/transport/websocket/websocket.go` (existing): экспорт `NoPadHeader` и метод `(*WSConn).PaddingActive() bool`.
- Тесты: `relay/padding_test.go` (new), `relay/bridge_test.go` (new), расширение `relay/handler_test.go`, `websocket/websocket_test.go`.

## Bootstrapping Surfaces

- none: пакеты, TLS/WS-серверы и test harness уже существуют.

## Влияние на архитектуру

- Relay перестаёт быть «особым» потребителем padding: конфиг трактуется как у client/server.
- Bridge из прозрачной пересылки конвертов становится hop-aware: принимает/отдаёт сообщения с учётом padding каждого плеча. Это локально усиливает bridge и убирает зависимость от случайного совпадения конвертов.
- Terminator upstream остаётся одним общим соединением; его unpadded-режим — следствие relay-конфига (зафиксировано в spec).
- Wire-совместимость: заголовок уже существует; старые peer'ы просто не подтверждают no-pad.

## Acceptance Approach

- AC-001 → `relayPadding*` + применение во всех трёх точках; unit-матрица helper и WS-тест «`obfuscation.enabled` без padding + клиент без padding».
- AC-002 → `acceptTerminatorWS` в httptest; клиент `RequestNoPad` проверяет `PaddingActive()==false` и подтверждающий заголовок.
- AC-003 → `dialAndHandshakeWS` против fake upstream с `AllowNoPad`; проверка `PaddingActive()==false` у relay conn и handshake.
- AC-004 → тот же тест с `AllowNoPad:false`; `PaddingActive()==true`, данные проходят.
- AC-005 → `relayWSHandler` + fake upstream в двух сценариях; проверка состояния на клиенте, bridge и upstream, отсутствие смешанного состояния.
- AC-006 → запрос без заголовка; `PaddingActive()==true` на клиентском плече.
- AC-007 → запись сообщений известной длины при `size: 1024`/default 512; проверка кратности и выравнивания на обоих плечах.
- AC-008 → `zaptest/observer` для подтверждённого и отклонённого исхода; проверка отсутствия токенов в полях.

## Данные и контракты

- Data model: no change.
- Wire contract: без изменений; `X-KVN-NoPad` и семантика подтверждения переиспользуются.
- Внутренний контракт: `websocket.NoPadHeader` (exported const), `(*WSConn).PaddingActive() bool`; `transport.StreamConn` не меняется, relay использует type assertion.
- `contracts/*` и `data-model.md` не создаются.

## Стратегия реализации

- DEC-001 Единый helper padding-правил в relay
  Why: текущие три разные трактовки (`enabled` fallback, только `padding`, отсутствие конфига в bridge) — источник рассинхрона.
  Tradeoff: один новый файл; bridge начинает явно зависеть от relay-конфига.
  Affects: `relay/padding.go`, `handler.go`, `upstream.go`, `bridge.go`.
  Validation: unit-матрица + AC-001.

- DEC-002 Экспортировать `NoPadHeader` и добавить `PaddingActive()`
  Why: bridge должен знать запрос до upgrade и фактический исход после dial; relay не может вывести это из конфига.
  Tradeoff: небольшое расширение внутреннего API пакета websocket.
  Affects: `transport/websocket/websocket.go`, `relay/bridge.go`, `relay/upstream.go`.
  Validation: websocket-тест на padded/confirmed/denied.

- DEC-003 Bridge предварительно диалит upstream при no-pad-запросе
  Why: подтверждение клиенту отправляется в upgrade-ответе до dial upstream; без предварительного dial нельзя гарантировать «оба плеча unpadded».
  Tradeoff: upstream-соединение открывается до чтения ClientHello; при плохом клиенте тратится один dial.
  Affects: `relay/bridge.go` (`relayWSHandler`, `bridgeRelayConn`).
  Validation: AC-005 в двух сценариях, AC-006 без заголовка.

- DEC-004 Bridge применяет padding на обоих плечах, а не пересылает конверт как есть
  Why: только так relay может независимо подтвердить/не подтвердить no-pad и сохранить корректность при разных конфигах.
  Tradeoff: сообщение декодируется и кодируется заново; стоимость как у текущих Read/Write.
  Affects: `relay/bridge.go`, `relayWSHandler`, `dialRelayUpstream`.
  Validation: AC-007 (размер выравнивания) и сквозной обмен.

- DEC-005 Terminator upstream согласует no-pad по конфигу, а не по запросу клиента
  Why: upstream один и общий; привязка к первому клиенту сделала бы поведение недетерминированным.
  Tradeoff: unpadded upstream действует на весь агрегированный трафик terminator.
  Affects: `relay/upstream.go`, `relay/bootstrap.go`.
  Validation: AC-003/AC-004 и документирование в `docs/*/relay.md`.

## Incremental Delivery

### MVP (Первая ценность)

- Helper + terminator client-facing `AllowNoPad` + upstream `RequestNoPad` + accessor/header.
- Готовность: `AC-001`, `AC-002`, `AC-003`, `AC-004`, `AC-006`, `AC-008` покрыты unit/handler-тестами.

### Итеративное расширение

- Bridge pre-dial и hop-aware padding.
- Готовность: `AC-005`, `AC-007`, сквозной bridge-тест; документация ru/en.

## Порядок реализации

1. `websocket`: `NoPadHeader`, `PaddingActive`, тесты.
2. `relay/padding.go` + unit-матрица.
3. Terminator: `handler.go` accept, `upstream.go` dial, логи.
4. Bridge: конфиг accept, pre-dial upstream, передача conn, логи.
5. Тесты bridge/cross-hop, docs, `go test -race`/`go vet`.

Параллелить можно шаги 1 и 2; шаг 4 зависит от 1–3.

## Риски

- Предварительный dial в bridge меняет порядок handshake и поведение при недоступном upstream.
  Mitigation: явный `502`/лог, сохранение семафора; мониторинг ошибок bridge.
- `obfuscation.enabled` больше не включает padding: конфиги, полагавшиеся на неявный padding, меняют режим.
  Mitigation: изменение возвращает документированную семантику; отметить в CHANGELOG/docs.
- Unpadded на общем upstream terminator снижает маскировку агрегированного трафика.
  Mitigation: поведение включается только `realtime_unpadded`; оператор может выставить `false`.
- Рассинхрон подтверждения между плечами bridge.
  Mitigation: подтверждение клиенту только после `PaddingActive()==false` upstream; матрица тестов.
- Type assertion `PaddingActive` для не-WS транспорта.
  Mitigation: fallback в «padding неизвестен/padded», QUIC вне scope.

## Rollout и compatibility

- Backward compatible: без заголовка и без поддержки на upstream всё остаётся padded.
- Конфиг-схема не меняется; специальные migration/feature flag не нужны.
- После релиза смотреть логи исходов no-pad и ошибки `padding frame too short` в relay.
- Документация: `docs/ru|en/relay.md`, `docs/ru|en/config.md`.

## Проверка

- Unit: `relay/padding_test.go`, `websocket` accessor, `relay/handler_test.go`.
- Integration: `relay/bridge_test.go` (оба плеча, accept/deny/без заголовка), fake upstream для `dialAndHandshakeWS`.
- Regression: существующие `TestNoPad*` и `TestPaddingNegotiationPrimaryPaddedSecondaryUnpadded` остаются зелёными.
- Commands: `go test -race ./src/internal/bootstrap/relay/... ./src/internal/transport/websocket/... ./src/integration/...`, `go vet ./...`.
- Покрывает `AC-001`–`AC-008`, `DEC-001`–`DEC-005`.

## Соответствие конституции

- нет конфликтов: изменения локальны, DDD-границы сохранены, wire-контракт не меняется, trace-маркеры `@sk-task/@sk-test` добавляются на новые декларации.
