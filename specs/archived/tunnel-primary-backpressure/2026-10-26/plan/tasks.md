# Primary TCP backpressure и переиспользование TUN-буферов — Задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/tunnel/session.go | T1.1, T1.2 |
| src/internal/tunnel/writer.go | T1.1 |
| src/internal/tunnel/session_test.go | T1.1, T2.1, T2.2, T2.4 |
| src/internal/tun/tun.go | T1.3 |
| src/internal/tun/tun_test.go, src/internal/tun/tun_alloc_test.go | T1.3, T2.3 |
| CHANGELOG.md | T3.1 |
| docs/en/architecture.md, docs/ru/architecture.md | T3.1 |
| src/internal/tunnel/, src/internal/tun/, src/internal/transport/websocket/ | T3.2 |

## Implementation Context

- Цель MVP: primary TCP снова на backpressure без drop-очереди; TUN read/write не аллоцируют на пакет.
- Инварианты: primary пишется синхронно из `dispatchPacket` с deadline `tunnelTimeout`; кадры не дропаются и не переставляются; secondary-очередь/batching/no-pad без изменений.
- Ошибки: ошибка primary-записи возвращается из `tunToWS` и завершает сессию; буферы возвращаются в пул.
- Контракты: `transport.StreamConn` не меняется; `queueWriter` остаётся только для secondary.
- Границы: не трогаем маршрутизацию, padding, QUIC, Android и relay.
- Proof signals: тест «все N кадров доставлены по порядку при медленном writer'е»; alloc-тест TUN; существующие secondary-тесты зелёные.
- Trace: `@sk-task` на новых/изменённых декларациях, `@sk-test` на тестах.

## Фаза 1: Реализация

Цель: убрать primary drop-очередь и per-packet аллокации.

- [x] T1.1 Вернуть синхронную primary-запись — outcome: `dispatchPacket` пишет TCP в primary с deadline, очередь/дропы primary удалены. Touches: src/internal/tunnel/session.go, src/internal/tunnel/writer.go
      Proof: code src/internal/tunnel/session.go writePrimary
- [x] T1.2 Обновить тесты и удалить primary-queue остатки — outcome: secondary-тесты сохранены, primary drop-тесты заменены на backpressure-семантику. Touches: src/internal/tunnel/session_test.go, src/internal/tunnel/writer.go
      Proof: test src/internal/tunnel/session_test.go TestPrimaryWriteAppliesBackpressure
- [x] T1.3 Убрать per-packet аллокации TUN — outcome: `Read` использует стековый sizes, `Write` берёт буфер с headroom из пула. Touches: src/internal/tun/tun.go, src/internal/tun/tun_test.go
      Proof: code src/internal/tun/tun.go writeScratchPool

## Фаза 2: Валидация

Цель: доказать backpressure, обработку ошибок и отсутствие регресса secondary.

- [x] T2.1 Покрыть backpressure — outcome: медленный fake writer получает все N кадров в исходном порядке. Touches: src/internal/tunnel/session_test.go
      Proof: test src/internal/tunnel/session_test.go TestPrimaryWriteAppliesBackpressure
- [x] T2.2 Покрыть ошибку primary-записи — outcome: сессия завершается, паники/зависания нет. Touches: src/internal/tunnel/session_test.go
      Proof: test src/internal/tunnel/session_test.go TestPrimaryWriteErrorEndsSession
- [x] T2.3 Покрыть аллокации TUN — outcome: `AllocsPerRun` после прогрева ≤1 на read/write серию. Touches: src/internal/tun/tun_test.go, src/internal/tun/tun_alloc_test.go
      Proof: test src/internal/tun/tun_alloc_test.go TestTunReadWriteAllocs
- [x] T2.4 Подтвердить secondary-регресс — outcome: bounded drop/stale/batching/no-pad тесты зелёные. Touches: src/internal/tunnel/session_test.go
      Proof: test src/internal/tunnel/session_test.go TestUDPDispatchUsesSecondaryWhenPrimaryIdle

## Фаза 3: Документация и gate

Цель: зафиксировать изменение поведения и прогнать проверки.

- [x] T3.1 Обновить CHANGELOG и архитектурную заметку — outcome: описано, что primary TCP снова backpressure, а drop-очередь остаётся только для UDP. Touches: CHANGELOG.md, docs/en/architecture.md, docs/ru/architecture.md
      Proof: docs CHANGELOG.md
- [x] T3.2 Прогнать gate — outcome: `go test -race` по tunnel/tun/websocket и `go vet ./...` зелёные. Touches: src/internal/tunnel/, src/internal/tun/, src/internal/transport/websocket/
      Proof: test src/internal/tunnel/session_test.go TestPrimaryWriteAppliesBackpressure

## Покрытие критериев приемки

- AC-001 -> T1.1, T2.1
- AC-002 -> T1.2, T2.4
- AC-003 -> T1.3, T2.3
- AC-004 -> T1.1, T2.2
- AC-005 -> T2.4, T3.2

## Заметки

- Data model: no change; contracts-файлы не создаются.
- Синхронный primary может задержать UDP при длительном заторе primary — принято в spec.
