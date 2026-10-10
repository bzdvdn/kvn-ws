# Game Latency Задачи

## Phase Contract

Inputs: `specs/active/game-latency/plan.md`, `spec.md`.
Outputs: упорядоченные задачи с `Touches:`, Surface Map и покрытием AC.
Stop if: задачи расплывчаты или AC не сопоставляется задачами.

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/config/server.go | T1.1, T3.4 |
| src/internal/config/client.go | T3.4, T4.2 |
| src/internal/bootstrap/server/server.go | T1.2, T3.4 |
| src/internal/bootstrap/client/tun.go | T1.3, T3.3 |
| src/internal/bootstrap/client/dial.go | T3.3 |
| src/internal/tunnel/session.go | T2.1 |
| src/internal/transport/transport.go | T3.2 |
| src/internal/transport/websocket/websocket.go | T3.1, T3.2 |
| src/internal/transport/websocket/wsfactory.go | T3.2 |
| src/internal/bootstrap/client/client_test.go | T1.4 |
| src/internal/tunnel/session_test.go | T2.2 |
| src/internal/transport/websocket/websocket_test.go | T3.5, T4.2 |
| src/integration/tunnel_integration_test.go | T4.1 |
| docs/ru/config.md, docs/en/config.md | T4.2 |

## Implementation Context

- Цель MVP: real-time UDP без padding-раздувания и без задержки агрегации + согласованный MTU; старые клиенты/сервера продолжают работать.
- Инварианты/семантика:
  - padding на вторичном (UDP) канале отключается ТОЛЬКО по обоюдному подтверждению (upgrade-заголовок `X-KVN-NoPad` req+resp); нет подтверждения → прежний padding.
  - MTU клиента = `min(cfg.MTU, ServerHello.Mtu)`, где `0`/отсутствие = не ограничивать; серверный TUN = `cfg.MTU` (дефолт 1400).
  - real-time UDP во вторичном writer отправляется без таймера удержания (non-blocking drain).
- Контракты/протокол: handshake-фреймы НЕ меняются; добавляется только необязательный WS upgrade-заголовок `X-KVN-NoPad`, старым peer игнорируется.
- Ошибки/совместимость: старый сервер без заголовка → padding остаётся; старый клиент → padding остаётся; `advertised=0` → без клампа.
- Границы scope: не трогаем QUIC-путь, формат фреймов, аутентификацию, Android, приоритизацию задач.
- Proof signals: unit-тест формата кадра после SetPadding; unit-тест тайминга одиночного UDP-кадра; unit/integration на граничный MTU; integration unpadded secondary vs padded primary; regression `go test -race`.
- References: DEC-001..DEC-005 в plan.md.

## Фаза 1: MTU-консистентность (MVP-1)

Цель: единый MTU туннеля — сервер применяет и анонсирует, клиент клампит.

- [x] T1.1 Дефолт MTU сервера 1400 в конфиге — outcome: `cfg.MTU>0` даже без ключа `mtu`; Touches: src/internal/config/server.go
      Proof: code src/internal/config/server.go DefaultServerMTU
- [x] T1.2 Применить `cfg.MTU` к серверному TUN после Open — outcome: `SetMTU(cfg.MTU)` вызывается, TUN MTU = конфиг; Touches: src/internal/bootstrap/server/server.go
      Proof: code src/internal/bootstrap/server/server.go New
- [x] T1.3 Кламп клиентского MTU по ServerHello — outcome: TUN MTU = `min(cfg.MTU, advertised)`, при advertised==0 без клампа; Touches: src/internal/bootstrap/client/tun.go
      Proof: code src/internal/bootstrap/client/tun.go effectiveMTU
- [x] T1.4 Тесты MTU — outcome: unit-тест клампа + проверка дефолта конфига; Touches: src/internal/bootstrap/client/client_test.go, src/internal/config/client_test.go
      Proof: test src/internal/bootstrap/client/client_test.go TestEffectiveMTU
      Proof: test src/internal/config/client_test.go TestServerMTUDefault

## Фаза 2: Zero-hold real-time UDP (MVP-2)

Цель: снять искусственную задержку агрегации на вторичном канале.

- [x] T2.1 Non-blocking drain в `startSecondaryWriter` — outcome: одиночный UDP-кадр уходит сразу, бёрст коалесцируется без ожидания; Touches: src/internal/tunnel/session.go
      Proof: code src/internal/tunnel/session.go startSecondaryWriter
- [x] T2.2 Тест задержки/коалесцинга вторичного writer — outcome: unit-тест подтверждает отправку одиночного кадра < 1 мс и склейку доступных кадров; Touches: src/internal/tunnel/session_test.go
      Proof: test src/internal/tunnel/session_test.go TestSecondaryWriterNoHold
      Proof: test src/internal/tunnel/session_test.go TestSecondaryWriterCoalescesQueued

## Фаза 3: Unpadded secondary с негоциацией (MVP-3)

Цель: UDP-кадры вторичного канала не паддятся, при этом совместимость со старыми peer.

- [x] T3.1 Рантайм-флаг padding на `WSConn` — outcome: `SetPadding(bool)`; Read/Write используют atomic-флаг; Touches: src/internal/transport/websocket/websocket.go
      Proof: code src/internal/transport/websocket/websocket.go SetPadding
- [x] T3.2 Негоциация через upgrade-заголовок — outcome: `DialContext` шлёт `RequestNoPad` и при подтверждении снимает padding; `Accept` при `AllowNoPad` ставит ответный заголовок и снимает padding; поля в `WSConfig`/`FactoryConfig`; Touches: src/internal/transport/websocket/websocket.go, src/internal/transport/websocket/wsfactory.go, src/internal/transport/transport.go
      Proof: code src/internal/transport/websocket/websocket.go DialContext
      Proof: code src/internal/transport/websocket/websocket.go Accept
- [x] T3.3 Клиент просит unpadded вторичный канал — outcome: `dialSecondaryChannel`/`dialStream` пробрасывают `RequestNoPad` по конфигу; Touches: src/internal/bootstrap/client/tun.go, src/internal/bootstrap/client/dial.go
      Proof: code src/internal/bootstrap/client/dial.go dialStreamWith
      Proof: code src/internal/bootstrap/client/tun.go dialSecondaryChannel
- [x] T3.4 Серверный AllowNoPad и конфиг-флаг — outcome: добавляется `obfuscation.padding.realtime_unpadded` (default true) в `PaddingCfg`, сервер прокидывает его в `WSConfig.AllowNoPad`; Touches: src/internal/config/client.go, src/internal/bootstrap/server/server.go
      Proof: code src/internal/config/client.go RealtimeUnpaddedEnabled
      Proof: code src/internal/bootstrap/server/server.go buildMux
- [x] T3.5 Тесты негоциации/формата — outcome: unit-тест «new+new → unpadded», «без заголовка → padded»; Touches: src/internal/transport/websocket/websocket_test.go
      Proof: test src/internal/transport/websocket/websocket_test.go TestNoPadNegotiationAccepted
      Proof: test src/internal/transport/websocket/websocket_test.go TestNoPadNegotiationDenied

## Фаза 4: Проверка и совместимость

Цель: доказать end-to-end и зафиксировать обратную совместимость.

- [x] T4.1 Integration: unpadded secondary + padded primary + граничный MTU — outcome: e2e-тест фиксирует размеры кадров и отсутствие дропов; Touches: src/integration/tunnel_integration_test.go
      Proof: test src/integration/tunnel_integration_test.go TestPaddingNegotiationPrimaryPaddedSecondaryUnpadded
- [x] T4.2 Документация флага и regression-проверка старого peer — outcome: docs ru/en описывают `realtime_unpadded`; тест «старый peer без заголовка сохраняет padding»; Touches: docs/ru/config.md, docs/en/config.md, src/internal/transport/websocket/websocket_test.go
      Proof: docs docs/ru/config.md
      Proof: docs docs/en/config.md
      Proof: test src/internal/transport/websocket/websocket_test.go TestNoPadOldPeerKeepsPadding

## Покрытие критериев приемки

- AC-001 -> T3.1, T3.2, T3.3, T3.4, T3.5, T4.1
- AC-002 -> T2.1, T2.2, T4.1
- AC-003 -> T1.1, T1.2, T1.3, T1.4
- AC-004 -> T3.2, T3.5, T4.1, T4.2

## Заметки

- Порядок фаз = порядок плана (MTU → zero-hold → unpadded); Фазы 1 и 2 независимы и могут делаться параллельно.
- Proof для каждой закрытой задачи — строкой `Proof:` под задачей при реализации.
