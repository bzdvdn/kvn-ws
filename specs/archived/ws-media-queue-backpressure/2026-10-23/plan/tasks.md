# WS Media Queue Backpressure and NoPad Integrity Задачи

## Phase Contract

Inputs: `plan.md`, `spec.md`, минимальный repo-контекст.
Outputs: упорядоченные задачи с покрытием AC.
Stop if: задача требует изменения wire-протокола или Android.

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/tunnel/writer.go | T1.1, T1.2, T2.1, T3.1, T3.4 |
| src/internal/tunnel/session.go | T1.2, T2.1, T3.1, T3.2, T3.4 |
| src/internal/transport/websocket/websocket.go | T3.3 |
| src/internal/transport/transport.go | T3.3 |
| src/internal/config/{client,server}.go | T2.2 |
| src/internal/bootstrap/client/tun.go | T2.2 |
| src/internal/bootstrap/server/handler.go | T2.2 |
| src/internal/tunnel/session_test.go | T2.3, T3.4 |
| src/internal/transport/websocket/websocket_test.go | T3.3 |
| src/integration/tunnel_integration_test.go | T4.1 |
| docs/ru/config.md, docs/en/config.md | T4.2 |
| src/internal/tunnel/CONTEXT.md, CHANGELOG.md | T4.2 |
| src/internal/webui/frontend/src/{types.ts,TabbedForm.tsx} | T4.4 |

## Implementation Context

- Цель MVP: bounded secondary-очередь с age/generation/clear и восстановленный batching.
- Границы приемки: AC-001, AC-002, AC-003, AC-006, AC-007.
- Ключевые правила: wire-протокол не меняется; trace-маркеры `@sk-task`/`@sk-test`; комментарии английские.
- Инварианты: один кадр принадлежит ровно одной очереди; дроп/очистка возвращают буфер в `framing` pool; secondary-кадры старого поколения не отправляются.
- Контракты/протокол: framing/handshake без изменений; опциональные конфиг-поля `secondary_batch_hold_ms`.
- Proof signals: unit/integration тесты, счётчики `SessionStats`, логи rate-limited.
- Вне scope: Android, audio/video priority, QUIC, relay no-pad, UI.

## Фаза 1: Основа

Цель: ввести переиспользуемый writer и статистику, не меняя поведение.

- [x] T1.1 Добавить `src/internal/tunnel/writer.go`: `queuedFrame`, bounded queue writer с batching, max age, generation, clear/drain, drop-счётчиками и безопасным возвратом буферов. Touches: src/internal/tunnel/writer.go
      Proof: code src/internal/tunnel/writer.go queueWriter
- [x] T1.2 Добавить `SessionStats` и `Session.Stats()` для снапшота счётчиков; подключить атомики к writer'у. Touches: src/internal/tunnel/writer.go, src/internal/tunnel/session.go
      Proof: code src/internal/tunnel/writer.go SessionStatsSnapshot

## Фаза 2: MVP Slice

Цель: secondary работает через новый writer, batching управляем, очередь чистится.

- [x] T2.1 Перевести secondary writer на `writer.go`: hold, age-drop, generation-drop, clear на loss/rebind/ошибке, возврат буферов. Touches: src/internal/tunnel/session.go, src/internal/tunnel/writer.go
      Proof: code src/internal/tunnel/session.go dispatchPacket
- [x] T2.2 Добавить `secondary_batch_hold_ms` (client flat, server `session.*`, default 2, clamp 0..10) и пробросить в `Session` из bootstrap. Touches: src/internal/config/client.go, src/internal/config/server.go, src/internal/bootstrap/client/tun.go, src/internal/bootstrap/server/handler.go
      Proof: code src/internal/config/client.go SecondaryBatchHold
- [x] T2.3 Unit-тесты: таймерный batching, max age, generation clear, overflow drops, hold=0. Touches: src/internal/tunnel/session_test.go
      Proof: test src/internal/tunnel/session_test.go TestSecondaryWriterCoalescesBurst

## Фаза 3: Основная реализация

Цель: primary не блокирует UDP, no-pad валидируется, дедлайн не теряется.

- [x] T3.1 Добавить primary TUN-data очередь/writer; `sendViaTunnel`/`tunToWS` только non-blocking enqueue; direct proxy/control без изменений; drop-tail + счётчик. Touches: src/internal/tunnel/session.go, src/internal/tunnel/writer.go
      Proof: code src/internal/tunnel/session.go dispatchPacket
- [x] T3.2 Валидация no-pad batch в `secondaryToTun`: trailing bytes, допустимые типы кадров, framing error счётчик и rebind без записи в TUN. Touches: src/internal/tunnel/session.go
      Proof: code src/internal/tunnel/session.go validateSecondaryBatch
- [x] T3.3 Deadline-safe запись: optional interface + `WSConn.WriteMessageWithDeadline`; использовать в writer'ах, чтобы ping не сбрасывал дедлайн. Touches: src/internal/transport/websocket/websocket.go, src/internal/transport/transport.go, src/internal/tunnel/writer.go
      Proof: code src/internal/transport/websocket/websocket.go WriteMessageWithDeadline
- [x] T3.4 Rate-limited логи и периодическая debug-сводка по `SessionStats` (очереди, дропы, write latency, framing errors). Touches: src/internal/tunnel/session.go, src/internal/tunnel/writer.go
      Proof: code src/internal/tunnel/session.go logStats

## Фаза 4: Проверка

Цель: доказать поведение и оставить пакет reviewable.

- [x] T4.1 Integration-тесты: primary blocked + UDP latency; rebind без stale; no-pad+batching на реальном WS; mixed batch/padding compatibility. Touches: src/integration/tunnel_integration_test.go, src/internal/transport/websocket/websocket_test.go
      Proof: test src/internal/tunnel/session_test.go TestPrimaryBackpressureDoesNotBlockUDP
- [x] T4.2 Обновить `src/internal/tunnel/CONTEXT.md`, `docs/ru/config.md`, `docs/en/config.md`, `CHANGELOG.md` (Unreleased). Touches: src/internal/tunnel/CONTEXT.md, docs/ru/config.md, docs/en/config.md, CHANGELOG.md
      Proof: docs docs/ru/config.md
- [x] T4.3 Прогнать gates: `go test -race ./src/...`, `go vet ./...`, `golangci-lint run`; зафиксировать результат. Touches: src/, .golangci.yml
      Proof: test src/internal/tunnel/session_test.go
- [x] T4.4 Добавить поле Secondary Batch Hold в kvn-web: Advanced (per-server) и Global, clamp 0..10, корректное сохранение `0`; пересобрать frontend и `bin/kvn-web`. Touches: src/internal/webui/frontend/src/types.ts, src/internal/webui/frontend/src/TabbedForm.tsx
      Proof: code src/internal/webui/frontend/src/TabbedForm.tsx "Secondary Batch Hold"

## Покрытие критериев приемки

- AC-001 -> T1.1, T2.1, T2.2, T2.3, T4.1
- AC-002 -> T1.1, T2.1, T2.3
- AC-003 -> T1.1, T2.1, T2.3
- AC-004 -> T3.1, T4.1
- AC-005 -> T3.2, T4.1
- AC-006 -> T2.1, T2.3, T4.1
- AC-007 -> T1.2, T3.4, T4.1
- AC-008 -> T3.3, T4.1
- AC-009 -> T4.4

## Заметки

- Не менять `StreamConn`; deadline-safe запись — optional interface с fallback.
- Primary batching не включать: peer `wsToTun` читает один кадр на сообщение.
- При очистке очереди обязательно возвращать каждый `[]byte` в `framing.ReturnBuffer`.
- `Inspect` не запускался; фича идёт по express-пути spec → plan → tasks → implement.
