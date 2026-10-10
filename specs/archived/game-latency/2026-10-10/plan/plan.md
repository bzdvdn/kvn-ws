# Game Latency План

## Phase Contract

Inputs: `specs/active/game-latency/spec.md`, конституция, минимальный repo-контекст.
Outputs: `plan.md` (data-model/contracts не меняются).
Stop if: спека расплывчата для безопасного планирования.

## Цель

Убрать оверхед и джиттер на real-time UDP-пути и выровнять MTU, не меняя intent спеки и не трогая Android:
1) UDP (вторичный канал) больше не паддится;
2) агрегатор UDP не вносит искусственную задержку;
3) сервер применяет сконфигурированный MTU к TUN, анонсирует фактический, клиент берёт минимум.

## MVP Slice

Три независимых инкремента, каждый закрывает свои AC:
- MTU-консистентность → AC-003.
- Zero-hold real-time UDP → AC-002.
- Unpadded secondary (negotiated) → AC-001.
AC-004 (совместимость) проверяется на каждом шаге.

## First Validation Path

- MTU: интеграционный тест шлёт пакет граничного размера; сверка ServerHello.MTU и обоих TUN.
- Zero-hold: unit-тест — одиночный UDP-кадр уходит < 1 мс (нет удержания), бёрст коалесцируется.
- Unpadded: integration-тест фиксирует размер вторичных кадров ≈ payload, первичных — с padding; тест «старый peer» сохраняет padding.

## Scope

- Транспортный WS-слой (padding toggle + upgrade-negotiation), клиентский вторичный dial, серверный secondary-handle.
- Вторичный writer в tunnel/session.
- Серверный bootstrap/TUN и клиентский `configureTun` (MTU).
- Конфиг-флаги режимов (без UI-работы).

Граница, остающаяся нетронутой: QUIC-путь, фреймовый формат, схема аутентификации, Android-клиент, handshake-фреймы.

## Performance Budget

- Добавленная клиентская задержка real-time UDP: p99 < 1 мс (SC-001).
- Амплификация UDP ≤ 1.2× (SC-002).
- Нет регресса throughput: soak/speedtest зелёные (SC-003).
- Горячий путь: не более +1 atomic-load на кадр (padding-флаг).

## Implementation Surfaces

- `src/internal/transport/websocket/websocket.go` — `SetPadding`, upgrade-negotiation в `DialContext`/`Accept`, `WSConfig` поля (existing, меняется).
- `src/internal/bootstrap/client/tun.go` — `dialSecondaryChannel`/`dialStream` просят unpadded secondary; `configureTun` клампит MTU (existing).
- `src/internal/bootstrap/client/dial.go` — проброс запроса unpadded при dial (existing).
- `src/internal/transport/transport.go` — добавить поле RequestNoPad в конфиг фабрики (existing).
- Тесты: `src/internal/transport/websocket/websocket_test.go`, `src/internal/tunnel/session_test.go`, `src/internal/bootstrap/client/client_test.go`, `src/integration/tunnel_integration_test.go` (existing).
- Документация: `docs/ru/config.md`, `docs/en/config.md` — описание флага `realtime_unpadded` (existing).
- `src/internal/tunnel/session.go` — `startSecondaryWriter` zero-hold (existing).
- `src/internal/transport/websocket/wsfactory.go` — проброс запроса unpadded из фабрики (existing).
- `src/internal/config/client.go`, `src/internal/config/server.go` — флаги режима и дефолт MTU (existing).
- `src/internal/bootstrap/server/server.go` — применить сконфигурированный MTU к TUN и прокинуть AllowNoPad (existing).

Новые поверхности не нужны: всё ложится на существующие швы.

## Bootstrapping Surfaces

- none — все модули и интерфейсы существуют.

## Влияние на архитектуру

- Локально: WS-транспорт получает рантайм-переключатель padding и заголовочную негоциацию на upgrade; tunnel-сессия — режим вторичного writer.
- Интеграции: negotiation целиком внутри TLS-соединения, наружу (в протокол/БД/клиентов) не выходит.
- Compatibility: отсутствие заголовка у старого peer сохраняет padding (по умолчанию).

## Acceptance Approach

- AC-001 -> DEC-001/DEC-002; surfaces websocket + client/secondary handle; наблюдаемо: захваченные размеры кадров (secondary ≈ payload, primary padded) в integration-тесте.
- AC-002 -> DEC-003; surface session.go; наблюдаемо: unit-тест тайминга отправки одиночного кадра < 1 мс.
- AC-003 -> DEC-004; surfaces config/server/client; наблюдаемо: integration-тест граничного пакета + проверка ServerHello.MTU и TUN MTU.
- AC-004 -> DEC-001 (нет заголовка → прежнее поведение) + DEC-004 (нет клампа при отсутствии/0); наблюдаемо: существующие тесты + запуск без новых флагов.

## Данные и контракты

- Data model: no change.
- Контракты: handshake-фреймы не меняются; добавляется только необязательный HTTP-заголовок на WS upgrade (`X-KVN-NoPad`) — внутренний для транспорта, обратно совместим (игнорируется старой стороной).
- Персистентность/миграции: не требуются.

## Стратегия реализации

DEC-001 Негоциировать unpadded secondary через WS upgrade header (`X-KVN-NoPad` req/resp), а не через handshake-фреймы
  Why: аддитивно и обратно совместимо без codegen/Android-правок; клиент просит заголовком, сервер подтверждает ответным заголовком, оба переключают padding после upgrade (ClientHello/ServerHello уже идут с согласованным режимом).
  Tradeoff: кастомный заголовок виден только TLS-терминирующему мидлбоксу; правка WS dial/accept.
  Affects: transport/websocket, bootstrap/client (secondary dial), bootstrap/server (secondary handle).
  Validation: integration-тест unpadded secondary + primary padded; тест «старый peer» → padding сохранён.

DEC-002 Переключать padding рантайм-флагом на `WSConn` (`SetPadding`)
  Why: решение принимается после установки соединения; пересоздавать conn не нужно.
  Tradeoff: +1 atomic-load на кадр; сложность минимальна.
  Affects: transport/websocket/websocket.go.
  Validation: unit-тест Read/WriteMessage после SetPadding (unpadded формат).

DEC-003 Zero-hold для real-time UDP: non-blocking drain вместо таймера удержания
  Why: убирает до 5 мс джиттера агрегатора, при этом бёрст коалесцируется без ожидания.
  Tradeoff: чуть больше отправок при разреженном трафике; сохраняем лимит размера батча.
  Affects: tunnel/session.go (`startSecondaryWriter`), опционально флаг в конфиге клиента.
  Validation: unit-тест задержки отправки одиночного кадра; отсутствие регресса throughput.

DEC-004 Канонический дефолт MTU 1400; сервер применяет `cfg.MTU` к TUN и анонсирует фактический; клиент берёт `min(cfg, advertised)`
  Why: единое значение, совпадающее с клиентским дефолтом и текущим TUN; устраняет рассогласование и ретрансмиты.
  Tradeoff: сервер начнёт анонсировать 1400 вместо 1500 (сегодня от этого значения никто не зависит).
  Affects: config/server.go (дефолт 1400), bootstrap/server/server.go (SetMTU), bootstrap/client/tun.go (`configureTun` clamp).
  Validation: unit-тест клампа; integration-тест граничного пакета.

DEC-005 Android не трогаем
  Why: заголовочная негоциация и MTU-кламп живут в Go-слое; Kotlin-кодек уже игнорирует неизвестные теги и флаги.
  Tradeoff: Android не получает выигрыш unpadded/MTU.
  Affects: none (границей служит Go WS-слой).
  Validation: существующие Android-тесты/handshake-совместимость зелёные.

## Incremental Delivery

### MVP (Первая ценность)

- Шаг 1: MTU-консистентность (AC-003) — изолированный, низкий риск.
- Шаг 2: zero-hold real-time UDP (AC-002) — изолированный.
- Шаг 3: unpadded secondary (AC-001) — зависит от surface WS-транспорта.
- Критерий готовности MVP: AC-001..AC-004 наблюдаемы в unit+integration тестах.

### Итеративное расширение

- Конфиг-флаги (hold/padding toggles) — после MVP, если понадобится управление в проде.
- Приоритизация/приоритетные очереди UDP — вне scope этой фичи (возможно отдельная спека).

## Порядок реализации

- Сначала DEC-004 (нет изменений протокола/транспорта), затем DEC-003 (локально в session.go), затем DEC-001/002 (WS-транспорт).
- Параллелится: DEC-004 и DEC-003.
- За флагом/дефолтом: unpadded secondary включается по умолчанию, но безопасен для старых peer'ов из-за негоциации.

## Риски

- Рассинхрон padding между сторонами (битые кадры).
  Mitigation: нельзя включить без подтверждающего заголовка; старый peer → padding остаётся.
- Расширение handshake/заголовков ломает Android/старые бинари.
  Mitigation: handshake-фреймы не меняются; заголовок игнорируется старой стороной; Android не читает padding-режим.
- Zero-hold снижает коалесцинг на высоком рейте и роняет throughput.
  Mitigation: non-blocking drain сохраняет коалесцинг бёрстов; проверка soak/speedtest.
- MTU-кламп неверно занижает MTU при `advertised=0`.
  Mitigation: трактовать 0 как «не ограничивать».
- Изменение анонса MTU 1500→1400.
  Mitigation: никто из клиентов не использует анонс сегодня; проверка интеграционным тестом.

## Rollout и compatibility

- Backward compatible: при отсутствии заголовка — прежний padding; при `cfg.MTU=0`/`advertised=0` — прежнее поведение.
- Feature-флаг: unpadded secondary включается конфигом (default on), при необходимости отключается.
- Операционное: после релиза проверить метрику добавленной задержки UDP и размеры кадров; особых миграций нет.

## Проверка

- Unit: `WSConn.SetPadding` (формат кадра), MTU-кламп в `configureTun`, zero-hold вторичного writer.
- Integration: граничный MTU-пакет; unpadded secondary vs padded primary; «старый peer» без заголовка сохраняет padding.
- Regression: полный `go test -race ./src/...`, soak/speedtest гейты.
- Подтверждает: AC-001 (unit+integration), AC-002 (unit), AC-003 (unit+integration), AC-004 (regression), DEC-001..DEC-005.

## Соответствие конституции

- нет конфликтов. Требования `@sk-task`/`@sk-test`, Go test -race/vet/lint, разделение docs ru/en соблюдаются; Android и Android-доки не затрагиваются.
