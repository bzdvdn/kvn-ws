# Relay: паритет padding и unpadded-переговоры — Задачи

## Phase Contract

Inputs: `spec.md`, `plan.md`.
Outputs: исполнимые задачи с `Touches:` и покрытием AC.

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/transport/websocket/websocket.go | T1.1, T3.2 |
| src/internal/transport/websocket/websocket_test.go | T1.1, T2.1 |
| src/internal/bootstrap/relay/padding.go | T1.2 |
| src/internal/bootstrap/relay/padding_test.go | T1.2, T4.1 |
| src/internal/bootstrap/relay/handler.go | T1.3, T3.2 |
| src/internal/bootstrap/relay/handler_test.go | T1.3, T2.2 |
| src/internal/bootstrap/relay/upstream.go | T1.4, T3.2 |
| src/internal/bootstrap/relay/upstream_test.go | T1.4, T2.3 |
| src/internal/bootstrap/relay/bridge.go | T3.1, T3.2 |
| src/internal/bootstrap/relay/bridge_test.go | T3.3, T4.1 |
| docs/ru/relay.md, docs/en/relay.md | T4.2 |
| docs/ru/config.md, docs/en/config.md | T4.2 |
| CHANGELOG.md | T4.2 |
| bootstrap/ | T1.2, T1.3, T1.4, T3.1 |
| relay/padding_test.go | T1.2, T4.1 |

## Implementation Context

- Цель MVP: relay применяет padding по правилам client/server и согласует `X-KVN-NoPad` на обоих WS-плечах без смешанного состояния.
- Границы MVP: AC-001, AC-002, AC-003, AC-004, AC-006, AC-008; AC-005 и AC-007 — bridge-тесты.
- Инварианты: padding включается только `obfuscation.padding.enabled`; size default 512; `realtime_unpadded` default true; terminator upstream решает по конфигу, bridge — per-connection с подтверждением upstream до ответа клиенту.
- Контракты: заголовок `X-KVN-NoPad`; новый `websocket.NoPadHeader` и `(*WSConn).PaddingActive() bool`; `transport.StreamConn` не меняется.
- Bridge: при no-pad-запросе предварительный dial upstream; клиент получает подтверждение только если upstream `PaddingActive()==false`; иначе оба плеча padded.
- Proof signals: unit-матрица helper; websocket-тест confirmed/denied; handler/upstream/bridge тесты по AC; `go test -race` + `go vet` зелёные.
- Вне scope: wire-формат, QUIC-padding, client/server-поведение, multiplex/MTU/batching/uTLS.
- Trace: на новых декларациях ставить `@sk-task`, на тестах `@sk-test`; комментарии английские.

## Фаза 1: Основа

Цель: дать relay единые правила padding и наблюдаемое состояние переговоров.

- [x] T1.1 Добавить `NoPadHeader` и `PaddingActive()` — outcome: relay видит no-pad-запрос до upgrade и фактический режим после Dial/Accept. Touches: src/internal/transport/websocket/websocket.go, src/internal/transport/websocket/websocket_test.go
      Proof: code src/internal/transport/websocket/websocket.go PaddingActive
- [x] T1.2 Добавить relay-padding helper — outcome: enabled/size/realtime вычисляются 1:1 с client/server, неявного включения из `obfuscation.enabled` нет. Touches: src/internal/bootstrap/relay/padding.go, src/internal/bootstrap/relay/padding_test.go
      Proof: test src/internal/bootstrap/relay/padding_test.go TestRelayPaddingRules
- [x] T1.3 Перевести terminator accept на helper — outcome: `AllowNoPad` включается только при padding+realtime, accept вынесен в тестируемый seam. Touches: src/internal/bootstrap/relay/handler.go, src/internal/bootstrap/relay/handler_test.go
      Proof: code src/internal/bootstrap/relay/handler.go acceptTerminatorWS
- [x] T1.4 Добавить no-pad в terminator upstream dial — outcome: `dialAndHandshakeWS` выставляет `RequestNoPad` по helper и логирует исход. Touches: src/internal/bootstrap/relay/upstream.go, src/internal/bootstrap/relay/upstream_test.go
      Proof: code src/internal/bootstrap/relay/upstream.go dialAndHandshakeWS

## Фаза 2: MVP-валидация

Цель: доказать правила и terminator-плечи до изменения bridge.

- [x] T2.1 Покрыть `PaddingActive` тестами — outcome: padded, подтверждённый и отклонённый no-pad различимы. Touches: src/internal/transport/websocket/websocket_test.go
      Proof: test src/internal/transport/websocket/websocket_test.go TestPaddingActiveReflectsNegotiation
- [x] T2.2 Покрыть terminator accept — outcome: заголовок подтверждается только при `realtime_unpadded`, клиентское состояние совпадает. Touches: src/internal/bootstrap/relay/handler_test.go
      Proof: test src/internal/bootstrap/relay/handler_test.go TestAcceptTerminatorWSNoPad
- [x] T2.3 Покрыть upstream dial — outcome: fake upstream accept/deny даёт ожидаемый `PaddingActive` и handshake не ломается. Touches: src/internal/bootstrap/relay/upstream_test.go
      Proof: test src/internal/bootstrap/relay/upstream_test.go TestDialAndHandshakeWSNoPad

## Фаза 3: Bridge и наблюдаемость

Цель: согласовать оба плеча bridge и сделать исходы переговоров видимыми.

- [x] T3.1 Согласовать padding в bridge — outcome: accept использует helper, при no-pad-запросе upstream диалится заранее, клиент подтверждается только после upstream; смешанного состояния нет. Touches: src/internal/bootstrap/relay/bridge.go
      Proof: code src/internal/bootstrap/relay/bridge.go relayWSHandler
- [x] T3.2 Добавить логи исходов — outcome: для каждой стороны есть запись confirmed/denied с peer, токены не логируются. Touches: src/internal/bootstrap/relay/bridge.go, src/internal/bootstrap/relay/upstream.go, src/internal/bootstrap/relay/handler.go
      Proof: code src/internal/bootstrap/relay/upstream.go logNoPadOutcome
- [x] T3.3 Покрыть bridge-матрицу — outcome: accept/deny/без заголовка сохраняют данные и ожидаемые состояния обоих плеч. Touches: src/internal/bootstrap/relay/bridge_test.go
      Proof: test src/internal/bootstrap/relay/bridge_test.go TestBridgePaddingNegotiation

## Фаза 4: Проверка и документация

Цель: закрыть размер/краевые случаи, обновить docs и прогнать полный gate.

- [x] T4.1 Покрыть `padding.size` и краевые случаи — outcome: 1024/default 512 выровнены, `realtime_unpadded: false` и старый клиент остаются padded. Touches: src/internal/bootstrap/relay/bridge_test.go, src/internal/bootstrap/relay/padding_test.go
      Proof: test src/internal/bootstrap/relay/bridge_test.go TestBridgePaddingSizeAlignment
- [x] T4.2 Обновить документацию — outcome: ru/en описывают padding relay и оба плеча; CHANGELOG фиксирует изменение семантики `obfuscation.enabled`. Touches: docs/ru/relay.md, docs/en/relay.md, docs/ru/config.md, docs/en/config.md, CHANGELOG.md
      Proof: docs docs/ru/relay.md
- [x] T4.3 Прогнать gate — outcome: relay/websocket/integration тесты и `go vet ./...` зелёные, race без находок. Touches: src/internal/bootstrap/relay/, src/internal/transport/websocket/, src/integration/
      Proof: test src/integration/tunnel_integration_test.go TestPaddingNegotiationPrimaryPaddedSecondaryUnpadded

## Покрытие критериев приемки

- AC-001 -> T1.2, T1.3, T1.4, T2.2, T4.3
- AC-002 -> T1.3, T2.2
- AC-003 -> T1.4, T2.3
- AC-004 -> T1.4, T2.3
- AC-005 -> T3.1, T3.3
- AC-006 -> T1.3, T2.2, T3.3, T4.1
- AC-007 -> T1.2, T3.3, T4.1
- AC-008 -> T1.4, T3.2, T4.1

## Заметки

- Data model: no change; contracts-файлы не создаются.
- Проверки добавляются отдельными задачами от реализации; после implement каждая закрытая задача получает строку `Proof:`.
