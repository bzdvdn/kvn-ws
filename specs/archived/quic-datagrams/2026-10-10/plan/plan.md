# QUIC Datagrams План

## Phase Contract

Inputs: `specs/active/quic-datagrams/spec.md`, конституция, минимальный repo-контекст.
Outputs: `plan.md` (data-model/contracts не меняются).
Stop if: спека расплывчата для безопасного планирования.

## Цель

Передавать real-time UDP (игры/VoIP) как unreliable QUIC datagrams поверх существующего QUIC-транспорта, убрав ретрансмит/HOL; TCP/control остаются на stream; включение только по обоюдной capability с fallback на stream для старых peer и без изменений Android/WS.

## MVP Slice

- Отправка/приём QUIC datagrams в транспорте и их подключение к вторичному (UDP) пути сессии.
- Аддитивная capability-негоциация + fallback.
- Закрывает AC-001, AC-002, AC-003, AC-004.

## First Validation Path

- Integration-тест: клиент и сервер с `transport=quic` поднимают сессию, capability согласована; UDP-пакет уходит/приходит датаграммой (счётчик datagrams>0, stream-счётчик не растёт).
- Тест с искусственной потерей датаграмм: задержка параллельного TCP-потока не растёт (HOL отсутствует), повторов нет.
- Тест «старый peer» (`Transport` без dgram-маркера) → UDP идёт stream, без ошибок.

## Scope

- QUIC-транспорт: datagram API обёртки и конфигурация.
- Capability-негоциация в handshake без изменения формата фреймов.
- Сессия: datagram-путь для UDP + reader-loop в TUN; stream для остального.
- Конфиг-флаг включения.

Граница, остающаяся нетронутой: WS/OkHttp путь и Android, фреймовый формат, схема аутентификации, обфускация WS/padding.

## Performance Budget

- При потере 5% датаграмм задержка stream-трафика p99 ≤ базовый RTT + 50 мс (SC-001).
- Ноль ретрансмитов на датаграммном пути (SC-003).
- Без регресса throughput: soak/speedtest зелёные (SC-002).
- Горячий путь: без лишних аллокаций на датаграмму (переиспользование буферов).

## Implementation Surfaces

- `src/internal/transport/quic/conn.go` — методы отправки/приёма датаграмм на `QUICConn` (existing).
- `src/internal/transport/quic/dial.go`, `src/internal/transport/quic/listen.go`, `src/internal/transport/quic/quicfactory.go` — включение датаграмм в `quic.Config` (existing).
- `src/internal/transport/quic/obfuscated.go` — паритет обфускации для датаграмм (existing).
- `src/internal/transport/transport.go` — опциональный интерфейс datagram-capability (existing).
- `src/internal/tunnel/session.go` — отправка UDP датаграммой + reader-loop датаграмм → TUN (existing).
- `src/internal/bootstrap/client/tun.go` и `src/internal/bootstrap/client/dial.go` — маркер capability в ClientHello и проверка echo (existing).
- `src/internal/bootstrap/server/handler.go` — включение датаграмм по маркеру и echo в ServerHello (existing).
- `src/internal/config/client.go`, `src/internal/config/server.go` — конфиг-флаг (existing).
- Тесты: `src/internal/transport/quic/quic_test.go`, `src/internal/tunnel/session_test.go`, `src/integration/tunnel_integration_test.go` (existing).

Новые поверхности не нужны: всё ложится на существующие швы.

## Bootstrapping Surfaces

- none — QUIC-транспорт, сессия и handshake уже существуют.

## Влияние на архитектуру

- Локально: `QUICConn` получает методы датаграмм; сессия — второй (unreliable) путь для UDP.
- Интеграции: capability передаётся в уже существующем поле `Transport` ClientHello/ServerHello — новые поля/теги не вводятся, Kotlin-модели не меняются.
- Compatibility: отсутствие dgram-маркера у старой стороны → stream-режим без изменений.

## Acceptance Approach

- AC-001 -> DEC-001/DEC-003; surfaces quic + session; наблюдаемо: integration-тест — счётчик датаграмм > 0, stream-счётчик для UDP не растёт.
- AC-002 -> DEC-001/DEC-003; surface session; наблюдаемо: тест с потерями датаграмм — p99 stream ≤ RTT+50 мс, нет повторов.
- AC-003 -> DEC-002; surfaces client/server handshake; наблюдаемо: тест «старый peer» → датаграмм нет, UDP идёт stream.
- AC-004 -> DEC-005; surfaces не меняются; наблюдаемо: WS-тесты зелёные, smoke WS-подключение.

## Данные и контракты

- Data model: no change.
- Контракты: формат handshake-фреймов не меняется; capability кодируется значением поля `Transport` (`quic` ↔ `quic-dgram`) — обратно совместимо (старая сторона значение игнорирует).
- Персистентность/миграции: не требуются.

## Стратегия реализации

DEC-001 Датаграммы только для real-time UDP; TCP/control — stream
  Why: unreliable-путь убирает ретрансмит и HOL для медиа, при этом control должен оставаться надёжным.
  Tradeoff: два параллельных пути чтения; порядок/доставка датаграмм не гарантируются.
  Affects: transport/quic, tunnel/session.go.
  Validation: integration-тест потери датаграмм + счётчики.

DEC-002 Capability через значение поля `Transport`, без изменения формата и codegen
  Why: добавление поля в сгенерированные Go/Kotlin data-классы сломало бы Android-кодек (out of scope); существующее строковое поле уже игнорируется старой стороной.
  Tradeoff: перегрузка семантики `Transport`; фиксируется документированное значение `quic-dgram`.
  Affects: bootstrap/client (send), bootstrap/server (echo/enable), handshake encode/decode (без правок структуры).
  Validation: тесты old+new и new+new.

DEC-003 Датаграмма несёт сырой IP-пакет без фрейминга; отдельный reader-loop пишет в TUN
  Why: датаграммы message-oriented; фрейминг/паддинг избыточны, а reader-loop изолирует их от stream.
  Tradeoff: отдельная горутина и путь обработки; требуется лимит размера.
  Affects: transport/quic/conn.go, tunnel/session.go.
  Validation: unit-тест round-trip одной датаграммы + integration.

DEC-004 Паритет обфускации датаграмм с stream (тот же XOR-конверт) либо гейт на её отключение
  Why: нельзя допустить расхождения fingerprint между stream и datagram-трафиком одной сессии.
  Tradeoff: обфускация датаграммы самодостаточна (nonce в пакете), но добавляет байты/сложность; при невозможности — датаграммы только при выключенной QUIC-обфускации.
  Affects: transport/quic/obfuscated.go.
  Validation: тест «obfuscation on» — датаграмма деобфусцируется на приёме; «off» — как есть.

DEC-005 Android/WS не трогаем
  Why: Android — OkHttp/WS, QUIC не использует; capability-механика живёт в Go-QUIC слое.
  Tradeoff: Android не получает выигрыш датаграмм.
  Affects: none.
  Validation: существующие WS-тесты/handshake-совместимость зелёные.

## Incremental Delivery

### MVP (Первая ценность)

- Шаг 1: datagram API в QUIC-обёртке + EnableDatagrams (AC-001 фундамент).
- Шаг 2: capability-негоциация и fallback (AC-003).
- Шаг 3: подключение UDP к datagram-пути + reader-loop (AC-001, AC-002, AC-004).
- Критерий готовности MVP: AC-001..AC-004 наблюдаемы в tests/integration.

### Итеративное расширение

- Тюнинг размера/фрагментации датаграмм, приоритизация UDP — после MVP (возможно отдельная спека).

## Порядок реализации

- Сначала datagram API транспорта, затем негоциация, затем интеграция в сессию.
- Параллелится: transport-API и handshake-негоциация.
- Датаграммы за флагом (default on), безопасны для старых peer из-за негоциации.

## Риски

- Размер UDP-пакета больше предела datagram frame.
  Mitigation: пакеты > предела шлём stream-фоллбэком; лимит проверяется на отправке.
- Старая сторона не понимает capability и ломается.
  Mitigation: маркер только в существующем поле; при отсутствии echo — stream; никаких новых обязательных полей.
- Потери датаграмм ломают сессию/control.
  Mitigation: контроль и TCP остаются на stream; reader-loop не роняет сессию на потерях.
- Расхождение обфускации stream/datagram.
  Mitigation: DEC-004 — единый конверт или гейт.

## Rollout и compatibility

- Backward compatible: при отсутствии dgram-capability (старый peer/не-QUIC) — полностью прежнее поведение.
- Feature-флаг: `udp_datagrams` (default on) для отключения при проблемах.
- Операционное: после релиза проверить счётчики датаграмм и p99 stream под потерями; миграций нет.

## Проверка

- Unit: round-trip датаграммы в QUIC-обёртке; выбор datagram/stream по capability; лимит размера.
- Integration: UDP через датаграммы + TCP через stream; потеря датаграмм не блокирует stream; старый peer → stream.
- Regression: `go test -race ./src/...`, soak/speedtest.
- Подтверждает: AC-001 (integration), AC-002 (integration), AC-003 (unit+integration), AC-004 (regression), DEC-001..DEC-005.

## Соответствие конституции

- нет конфликтов. Требования `@sk-task`/`@sk-test`, `go test -race`/`vet`/`lint`, разделение docs ru/en соблюдаются; Android и Android-доки не затрагиваются.
