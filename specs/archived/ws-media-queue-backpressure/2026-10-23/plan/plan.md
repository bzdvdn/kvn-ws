# WS Media Queue Backpressure and NoPad Integrity План

## Phase Contract

Inputs: `specs/active/ws-media-queue-backpressure/spec.md`, конституция, минимальный repo-контекст (`src/internal/tunnel`, `transport/websocket`, `bootstrap/{client,server}`).
Outputs: `plan.md`; data-model/contracts не меняются.
Stop if: реализация потребует менять wire-протокол или Android-клиент.

## Цель

Перестроить отправку TUN-трафика так, чтобы UDP-медиа не зависело от скорости primary: ввести общий bounded-writer с ограничением возраста кадров, вернуть управляемый batch hold, очищать очередь при смене поколения secondary, валидировать no-pad-формат и дать наблюдаемые счётчики дропов/задержек.

## MVP Slice

- Bounded secondary-очередь с max age и очисткой при rebind/loss + тесты.
- Закрывает AC-002, AC-003, частично AC-007; AC-001 возвращается настройкой hold.

## First Validation Path

- `go test -race ./src/internal/tunnel/... ./src/internal/transport/websocket/... ./src/integration/...`
- Затем ручной прогон: тестовый kvn-web в TUN с включённым `multi_channel`, звонок с видео; в логах нет роста stale/overflow, голос не роботизируется.

## Scope

- `src/internal/tunnel`: модель очередей/writer'ов, классификация, очистка, валидация, статистика.
- `src/internal/transport/websocket`: deadline-safe запись.
- `src/internal/config` + `bootstrap/{client,server}`: проброс `secondary_batch_hold_ms`.
- Тесты, `src/internal/tunnel/CONTEXT.md`, `docs/ru|en/config.md`, `CHANGELOG.md`.
- Граница, остающаяся нетронутой: wire-протокол, handshake, Android, QUIC datagrams, relay bridge.

## Performance Budget

- Dispatch-to-enqueue UDP при заблокированном primary: p99 ≤ 5 мс.
- Secondary enqueue не блокирует TUN-диспетчер: cap 512 кадров.
- Max age real-time secondary: 100 мс.
- Добавленная задержка batching: ≤ hold (default 2 мс) + 5 мс.
- Дополнительные аллокации на кадр: не больше одного `queuedFrame` (данные уже из пула).
- Память очередей: secondary ≤ ~0.7 МБ, primary ≤ ~0.8 МБ на сессию.

## Implementation Surfaces

- `src/internal/tunnel/session.go` — поля сессии, классификация, постановка в очереди, очистка, валидация no-pad, вызовы writer'ов.
- `src/internal/tunnel/writer.go` (новый) — generic bounded queue/writer с batching, age, generation, stats, clear/drain.
- `src/internal/transport/websocket/websocket.go` — `WriteMessageWithDeadline`, чтобы ping не сбрасывал дедлайн data-записи.
- `src/internal/transport/transport.go` — optional interface для deadline-записи (без изменения `StreamConn`).
- `src/internal/config/client.go`, `src/internal/config/server.go` — `secondary_batch_hold_ms` (client flat, server `session.*`).
- `src/internal/bootstrap/client/tun.go`, `src/internal/bootstrap/server/handler.go` — установка hold в `Session`.
- `src/internal/webui/frontend/src/{types.ts,TabbedForm.tsx}` — UI-поле Advanced/Global.
- Тесты: `src/internal/tunnel/session_test.go`, `src/internal/transport/websocket/websocket_test.go`, `src/integration/tunnel_integration_test.go`.
- Документация: `src/internal/tunnel/CONTEXT.md`, `docs/ru/config.md`, `docs/en/config.md`, `CHANGELOG.md`.
- Gates: `src/`, `.golangci.yml`.

## Bootstrapping Surfaces

- none — `Session`, WSConn, config и тестовые фейки уже существуют.

## Влияние на архитектуру

- Локально: `Session` получает два независимых writer'а (primary TUN data и secondary UDP) и очередь с метками поколения/времени.
- Интеграции: protocol/handshake и framing не меняются; старые peer'ы работают через fallback без batching/padding.
- Совместимость: новые поля конфига опциональны; default 2 мс; `0` возвращает прежнее немедленное поведение.
- Риск: primary drop-tail при переполнении может временно снизить throughput TCP; смягчается большим cap и тем, что TCP восстанавливается ретрансмиссией.

## Acceptance Approach

- AC-001 -> writer с hold=2 мс; тест «бурст ≥4 кадров → ≤2 сообщения»; счётчик frames-per-message.
- AC-002 -> timestamp в `queuedFrame`, age-drop в writer; unit-тест с блокирующим writer'ом.
- AC-003 -> generation counter в `queuedFrame`, `SetSecondary`/loss инкрементируют generation и drain'ят очередь; тест двух поколений.
- AC-004 -> `primaryCh` + dedicated writer; `tunToWS`/`sendViaTunnel` только enqueue; тест с заблокированным primary.
- AC-005 -> валидация в `secondaryToTun`: trailing bytes, допустимые FrameType; тест real WS no-pad+batch и padded-mismatch.
- AC-006 -> существующие тесты + новый mixed-mode тест.
- AC-007 -> `SessionStats` и rate-limited логи; тест snapshot/логов.
- AC-008 -> `WriteMessageWithDeadline`; тест с конкурирующим ping.
- AC-009 -> поле Advanced/Global `secondary_batch_hold_ms`, сохранение через существующие update-колбэки; ручная проверка `0`/`5` и сборка frontend.

## Данные и контракты

- Data model: no change (in-memory queues/stat counters only).
- API/event contracts: no change; wire-формат framing/handshake не меняется.
- Конфиг: добавляются опциональные `secondary_batch_hold_ms` (клиент) и `session.secondary_batch_hold_ms` (сервер), default `2`, допустимо `0..10`; `0` — немедленный flush.
- Compatibility: при отсутствии полей и при старом peer'е поведение прежнее.

## Стратегия реализации

- DEC-001 Generic bounded writer с generation и max age
  Why: одна реализация для primary/secondary убирает расхождения и даёт одинаковые гарантии дропа/очистки.
  Tradeoff: чуть больше кода в tunnel; нужен аккуратный возврат буферов.
  Affects: `src/internal/tunnel/writer.go`, `session.go`.
  Validation: unit-тесты на batching, age-drop, clear, overflow.

- DEC-002 Primary TUN data через отдельный writer, direct proxy/control оставить как есть
  Why: именно синхронная запись primary в общем цикле задерживает постановку UDP в secondary.
  Tradeoff: при переполнении primary-очередь дропает TCP-пакеты; TCP восстанавливается ретрансмиссией.
  Affects: `session.go` (`sendViaTunnel`, `tunToWS`), `writer.go`.
  Validation: integration-тест с блокирующим primary и живым secondary.

- DEC-003 Вернуть таймерный batching с настраиваемым hold
  Why: zero-hold сделал batching фактически мёртвым; тесты этого не ловили из-за предзаполнения очереди.
  Tradeoff: +hold к задержке UDP; default 2 мс, `0` отключает.
  Affects: `writer.go`, config, bootstrap.
  Validation: тест paced/burst с реальным таймером; `TestSecondaryWriterNoHold` остаётся для `hold=0`.

- DEC-004 Строгая валидация no-pad batch
  Why: при рассинхроне padded-сообщение декодируется как мусор/рвёт канал; нужно безопасно падать в rebind.
  Tradeoff: любое неизвестное сообщение закрывает secondary; для real-time это приемлемо.
  Affects: `secondaryToTun` в `session.go`.
  Validation: real-WS тест no-pad+batch; тест padded-mismatch.

- DEC-005 Deadline-safe запись
  Why: control-ping выставляет дедлайн в ноль между `SetWriteDeadline` и `WriteMessage`, из-за чего запись может висеть.
  Tradeoff: новый optional interface; fallback для не-WS транспортов сохраняется.
  Affects: `websocket.go`, `transport.go`, writer'ы.
  Validation: unit-тест с конкурирующим ping.

- DEC-006 Наблюдаемость через counters + rate-limited logs
  Why: без этого не видно, какая очередь растёт и почему.
  Tradeoff: небольшой объём логов/атомиков на горячем пути.
  Affects: `session.go`, `writer.go`.
  Validation: `Stats()` snapshot в тестах; ручная проверка в звонке.

## Incremental Delivery

### MVP (Первая ценность)

- Generic writer + bounded secondary + clear/age + hold default 2 мс.
- AC-001, AC-002, AC-003, AC-006, AC-007.

### Итеративное расширение

- Primary writer (AC-004).
- No-pad validation (AC-005).
- Deadline-safe запись (AC-008).
- Тесты/доки/changelog.

## Порядок реализации

1. `writer.go` + `SessionStats` + unit-тесты.
2. Перевод secondary на новый writer, очистка/поколения.
3. Конфиг hold + проброс.
4. Primary writer и разблокировка диспетчера.
5. No-pad validation.
6. Deadline-safe запись.
7. Integration-тесты, CONTEXT/docs/CHANGELOG, полный прогон.

## Риски

- Риск: primary drop-tail ухудшит bulk TCP при перегрузке.
  Mitigation: cap 512 кадров, drop только при реальном переполнении; метрики; при необходимости увеличить cap.
- Риск: batching снова добавит задержку.
  Mitigation: default 2 мс, конфиг `0..10`, тест на задержку.
- Риск: очистка очереди гоняется с producer'ами.
  Mitigation: generation-метка; writer дропает кадры старого поколения, drain только возвращает буферы.
- Риск: строгая no-pad валидация закроет канал на неизвестном, но валидном кадре.
  Mitigation: разрешать только известные типы Data/Close/Proxy/DNS; для secondary ожидаем Data/Close; ошибка ведёт к rebind, не к падению сессии.
- Риск: primary writer меняет порядок относительно direct proxy-записей.
  Mitigation: proxy/control остаются на прежнем пути; кадры независимы, порядок внутри proxy-стрима сохраняется его writer'ом.

## Rollout и compatibility

- Новые поля конфига опциональны; старые конфиги валидны.
- Старые peer'ы без batching: односообщенческий режим; с padding: padded-формат.
- Rollout: собрать клиент и сервер вместе; проверить звонок; при откате вернуть прежний бинарь — wire-протокол не менялся.
- Monitoring: rate-limited логи `secondary queue drop`, `stale drop`, `framing error`, `primary queue drop`, periodic stats.

## Проверка

- Unit: writer batching/age/clear/overflow, deadline race, no-pad validation.
- Integration: primary blocked + UDP secondary latency; rebind без stale; mixed primary/secondary.
- Regression: `go test -race ./src/...`, `go vet ./...`, `golangci-lint run`.
- Manual: тестовый kvn-web + сервер, TUN, `multi_channel`, звонок с видео; смотреть логи/счётчики.
- Подтверждает: AC-001..AC-008, DEC-001..DEC-006.

## Соответствие конституции

- нет конфликтов; trace-маркеры `@sk-task`/`@sk-test` добавляются, комментарии английские, docs ru/en.
