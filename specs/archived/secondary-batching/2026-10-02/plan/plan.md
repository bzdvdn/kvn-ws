# Secondary Channel Batching План

## Phase Contract

Inputs: `specs/active/secondary-batching/spec.md`, конституция, repo-контекст (handshake.yaml, session.go, bootstrap client/server, Android protocol).
Outputs: `plan.md`. Data model: **no change** (без персистентности/состояния).
Stop if: спеку нельзя реализовать без выдумывания требований — можно, все AC покрыты.

## Цель

Двухфазная реализация батчинга вторичного WS-канала (UDP/медиа): фаза 1 (Go — сервер+клиент) фиксирует wire-формат флага и поведение, фаза 2 (Android) повторяет механизм на Kotlin. Подход безопасен: батчинг включается только при обоюдном handshake-флаге, первичный канал не трогается, параметры батча ограничены бюджетом латентности (≤5 мс).

## MVP Slice

Фаза 1 (Go): `BatchTag` в ClientHello/ServerHello → codegen → батчинг uplink в `tunToWS` + цикл-декодер в `secondaryToTun` → интеграционные тесты. Закрывает AC-001..AC-005. Фаза 2 (Android) — отдельный инкремент (AC-006..AC-007), не начинается до верификации фазы 1.

## First Validation Path

Прогон `go test -race ./src/integration/... ./src/internal/protocol/handshake/...` + интеграционный тест: клиент с флагом шлёт ≥2 UDP-пакетов одним WS-сообщением, приёмник записывает все пакеты в TUN; hello без флага → однокадровые сообщения (backward-compat).

## Scope

- `protocol/handshake.yaml` + codegen (Go `types_gen.go`, Kotlin `Handshake.kt`): константа `BatchTag=0x0E`, поля `BatchSupport` в ClientHello и ServerHello.
- `src/internal/tunnel/session.go`: батчинг secondary в `tunToWS`, цикл-декодер в `secondaryToTun`, accessor `SetBatchMode/BatchEnabled`.
- `src/internal/bootstrap/client/tun.go` (`dialSecondaryChannel`): слать флаг, включить батч-режим после подтверждения в ServerHello.
- `src/internal/bootstrap/server/handler.go` (`handleSecondaryStream`): распознать флаг, включить батч-режим, подтвердить в ServerHello.
- Фаза 2 (Android): `HandshakeClient.kt` (флаг), `FrameCodec.kt` (цикл-декодер), `KvnVpnService.kt` (батчинг uplink secondary).
- Явная граница вне правок: первичный канал, TCP-трафик, дефолт `obfuscation.padding.size`.

## Performance Budget

- Hold-время батча ≤ 5 мс (AC-005 / SC-002); макс. размер батча 4096 B (флаш по размеру для высоких PPS).
- Батч-буфер из пула (без `make` на каждое сообщение); в батч-режиме на пакет не добавляется аллокаций сверх текущего фрейминга.
- Wire-overhead для мелких UDP при padding 128 < 2x (SC-001).

## Implementation Surfaces

- `protocol/handshake.yaml` — существующий: константы/поля.
- `src/internal/protocol/handshake/handshake.go`, `types_gen.go` — существующие: encode/decode тегов.
- `src/internal/tunnel/session.go` — существующий: `tunToWS`, `secondaryToTun`, добавить `batchMode atomic.Bool`.
- `src/internal/bootstrap/client/tun.go` — существующий: `dialSecondaryChannel`.
- `src/internal/bootstrap/server/handler.go` — существующий: `handleSecondaryStream`.
- Фаза 2: `HandshakeClient.kt`, `FrameCodec.kt`, `KvnVpnService.kt` — существующие.
- Новых пакетов не требуется.

## Bootstrapping Surfaces

`none` — кодек и secondary-канал уже существуют; добавляются только поле/тег и методы.

## Влияние на архитектуру

- Локально: `session.go` — write-путь secondary становится накопительным; read-путь `secondaryToTun` — цикл-декодер. Логика изолирована за флагом, первичный путь не меняется.
- Интеграции: wire-совместимость сохраняется (аддитивный тег; старые версии игнорируют неизвестный тег → однокадровый режим).
- Compatibility/rollout: сервер и клиент обновляются вместе; смешанные версии автоматически фолбэкаются на 1 пакет = 1 сообщение.

## Acceptance Approach

- AC-001 (обоюдный флаг): юнит `handshake_test.go` round-trip `BatchSupport` + интеграционный тест проверяет флаг в ServerHello. Surfaces: handshake.yaml, handler.go, tun.go.
- AC-002 (UDP одним сообщением): интеграционный тест на secondary-стрим считает кадры/сообщения (≥2 кадра в 1 сообщении). Surfaces: session.go `tunToWS`.
- AC-003 (все кадры в TUN): интеграционный тест сверяет отправленные/полученные пакеты. Surfaces: session.go `secondaryToTun`.
- AC-004 (обратная совместимость): интеграционный тест hello без флага → однокадровые сообщения. Surfaces: handler.go, handshake.
- AC-005 (латентный бюджет): тест измеряет max hold одного UDP-пакета ≤ 5 мс. Surfaces: session.go константы батча.
- AC-006 (Android батч/разбор): HandshakeCodecTest (флаг) + KvnVpnServiceTest (батч-декодер/маршрутизация). Surfaces: Android protocol/vpn.
- AC-007 (Android backward-compat): тест кодека без флага + ручная проверка со старым APK. Surfaces: Android HandshakeClient/FrameCodec.

## Данные и контракты

- `Data model: no change` (нет персистентности).
- Контракт: handshake.yaml расширяется аддитивно (`BatchTag=0x0E` + `BatchSupport` в ClientHello/ServerHello); старые декодеры пропускают неизвестный тег — совместимость сохранена.
- Wire-семантика батча: одно WS-сообщение = конкатенация стандартных кадров (4-байтовый заголовок + payload); приёмник циклически разбирает по `Length`.

## Стратегия реализации

- DEC-001 Capability через тег `BatchTag=0x0E` в ClientHello и ServerHello
  Why: зеркалит существующий паттерн ChannelTag/SessionTag, симметрично для клиента и сервера (в ServerHello нет flags-байта); аддитивно и backward-compatible (неизвестный тег пропускается).
  Tradeoff: один лишний optional-тег; старые версии просто не переходят в батч-режим.
  Affects: handshake.yaml, types_gen.go, handshake.go, Handshake.kt.
  Validation: codec round-trip `BatchSupport`, интеграционный флаг в ServerHello.
- DEC-002 Параметры батча — константы `secondaryBatchMaxBytes=4096`, `secondaryBatchHold=5ms`
  Why: hold ограничивает латентность (AC-005/SC-002), size — размер фрейма при высоких PPS; фиксируются в коде, не в конфиге (спеку не расширяем конфиг-поверхностью).
  Tradeoff: не конфигурируемо без релиза; приемлемо для фиксированного бюджета.
  Affects: session.go.
  Validation: тест max-hold ≤5 мс, флаш по размеру на высокой PPS.
- DEC-003 Батчинг только в write-пути secondary, за флагом `batchMode`
  Why: RQ-005 (первичный канал нетронут); обоюдность через DEC-001 исключает поломку старых приёмников.
  Tradeoff: дополнительный таймер в цикле `tunToWS` — активен только когда есть secondary-батч.
  Affects: session.go `tunToWS`, `SetBatchMode/BatchEnabled`.
  Validation: AC-002/AC-004 интеграционные тесты.
- DEC-004 Цикл-декодер в `secondaryToTun` (клиент и сервер)
  Why: RQ-003 — один батч содержит N кадров; цикл по `Length` безопасен и для однокадровых сообщений (backward-compat).
  Tradeoff: переиспользование пула payload'ов требует аккуратной обработки хвоста сообщения.
  Affects: session.go `secondaryToTun`.
  Validation: AC-003 (все кадры в TUN).
- DEC-005 Флаг-пропагация: клиент шлёт BatchSupport → сервер ставит batchMode и подтверждает в ServerHello → клиент включает батчинг только после подтверждения
  Why: обоюдность (RQ-001/AC-001/AC-004) — ни одна сторона не батчит без согласия второй.
  Tradeoff: uplink батчинг клиента включается чуть позже handshake (после ServerHello) — незначительная задержка перехода.
  Affects: client/tun.go `dialSecondaryChannel`, server/handler.go `handleSecondaryStream`, Session.
  Validation: AC-001, AC-004.
- DEC-006 Android повторяет механизм Go (флаг + цикл-декодер + простой батчер uplink)
  Why: единый wire-формат (спека: фаза 2 зависит от формата фазы 1); минимум расхождений с Go.
  Tradeoff: дублирование логики батчера в Kotlin; приемлемо для изоляции платформ.
  Affects: HandshakeClient.kt, FrameCodec.kt, KvnVpnService.kt.
  Validation: AC-006/AC-007 (HandshakeCodecTest, KvnVpnServiceTest).

## Incremental Delivery

### MVP (Первая ценность)

Фаза 1 (Go): handshake-флаг → батчинг uplink → цикл-декодер → интеграционные тесты. Готовность: AC-001..AC-005 зелёные; проверка — `go test -race ./src/integration/... ./src/internal/protocol/handshake/...`.

### Итеративное расширение

Фаза 2 (Android): флаг в HandshakeClient, батч-декодер в FrameCodec, батчинг uplink в KvnVpnService. Готовность: AC-006..AC-007 зелёные; проверка — `./gradlew testDebugUnitTest`.

## Порядок реализации

1. `protocol/handshake.yaml` + codegen (Go+Kotlin) — сначала, т.к. от формата флага зависят обе фазы.
2. Go session.go: `batchMode` + батчинг `tunToWS` + цикл-декодер `secondaryToTun` — ядро фазы 1.
3. Go client/server пропагация флага (dialSecondaryChannel, handleSecondaryStream).
4. Интеграционные тесты (AC-001..005) и backward-compat.
5. Android фаза (HandshakeClient/FrameCodec/KvnVpnService + тесты).
Параллелится: кодек round-trip тесты с разработкой session.go; фаза 2 не пересекается с фазой 1 по файлам.

## Риски

- Риск 1: батч-сообщение уходит старому приёмнику → потеря «хвоста».
  Mitigation: батчинг строго за обоюдным флагом (DEC-001/005); старые версии — однокадровый режим.
- Риск 2: удержание батча добавляет латентность → джиттер в медиа.
  Mitigation: hold ≤5 мс (AC-005), флаш по размеру для высоких PPS.
- Риск 3: таймер в горячем цикле `tunToWS` вносит сложность/задержку.
  Mitigation: таймер активен только при наличии secondary-батча; флаш по размеру не требует таймера.
- Риск 4: цикл-декодер теряет кадры из-за хвоста сообщения.
  Mitigation: строгий разбор по `Length` с проверкой остатка ≥ заголовка; покрыто AC-003 тестом.

## Rollout и compatibility

- Аддитивный тег — обратная совместимость без флаг-гейтов в конфиге; специальный rollout не требуется.
- Сервер и клиент обновляются вместе; смешанные версии фолбэкаются автоматически (AC-004/AC-007).
- Мониторинг: те же метрики сессий; поведение меняется только для пар «новый клиент ↔ новый сервер».

## Проверка

- Automated: `go test -race ./src/...` + `go vet` + `golangci-lint`; интеграционные тесты AC-001..005; Android `./gradlew testDebugUnitTest` AC-006..007.
- Manual: звонок/видео при `padding.size: 128` на Go-клиенте и Android — чисто, без робовойса; старый клиент на новом сервере — туннель работает.
- Каждый шаг подтверждает конкретные AC-*/DEC-* (см. Acceptance Approach).

## Соответствие конституции

- нет конфликтов: Go/Kotlin, trace-маркеры, DDD, тесты — соблюдены; фича аддитивна и не меняет поставку (Go-сервер + APK).