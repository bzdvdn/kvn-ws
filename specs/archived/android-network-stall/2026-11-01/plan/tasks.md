# Android: устранение залипаний и зависаний сети — Задачи

## Phase Contract

Inputs: `spec.md` (express-режим, plan отсутствует).
Outputs: упорядоченные задачи с `Touches:` и покрытием AC.
Stop if: задача не имеет измеримого outcome или coverage не сходится.

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/tcpstack/stack.go | T1.1, T4.3 |
| src/internal/tcpstack/stack_test.go | T1.2, T4.4 |
| src/internal/transport/client.go | T2.1, T2.2 |
| src/internal/tlsprofile/profile.go | T2.1 |
| src/internal/client/client.go | T2.2, T3.1, T4.3 |
| src/internal/client/direct.go | T3.1 |
| src/internal/client/direct_test.go | T3.2 |
| src/internal/transport/transport_test.go | T2.4 |
| src/mobile/mobile.go | T2.2, T4.1 |
| src/mobile/fddevice_test.go | T4.4 |
| src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt | T2.3, T4.2 |

## Implementation Context

- Цель MVP: убрать утечку forward-запросов и сделать остановку TUN ограниченной (AC-001, AC-005).
- Инварианты/семантика:
  - `ForwarderRequest` завершается ровно один раз: `Complete(true)` при ошибке `CreateEndpoint`, `Complete(false)` сразу после успеха.
  - Go не закрывает TUN fd; Kotlin закрывает его после того, как Go прекратил чтение.
  - Health-проверка идёт по тому же `transport.Client`/соединению, без нового TLS-рукопожатия.
  - Резолв excluded-доменов не выполняется в цикле чтения TUN; перегрузка пула — дроп пакета.
  - Dial flow использует контекст, производный от контекста сессии, и отменяется вместе с flow.
- Контракты/протокол: HTTP/2 keepalive — `ReadIdleTimeout=15s`, `PingTimeout=10s`; dial timeout — 10s; health ctx — 5s.
- Proof signals: `go test ./src/internal/tcpstack ./src/internal/client ./src/internal/transport ./src/mobile` зелёный; тесты на forward-budget, async DNS, stop fd и ctx-отмену.
- Вне scope: NetworkCallback/reconnect-цикл, unlock-restart, QUIC-handshake timeout, пользовательский DNS в VpnService, легаси-Kotlin транспорт.

## Фаза 1: MVP — жизненный цикл forward-запроса

Цель: новые TCP-соединения перестают блокироваться после исчерпания forward-бюджета.

- [x] T1.1 Завершать каждый успешный forward-запрос: `Complete(false)` сразу после `CreateEndpoint`, `Complete(true)` только в ошибке. Outcome: запись удаляется из forwarder, бюджет не растёт. Touches: src/internal/tcpstack/stack.go
      Proof: code src/internal/tcpstack/stack.go handle
- [x] T1.2 Покрыть тестом: последовательно открыть больше соединений, чем тестовый forward-лимит, и убедиться, что последнее успешно, `ForwardMaxInFlightDrop == 0`. Outcome: регрессия утечки ловится тестом. Touches: src/internal/tcpstack/stack.go, src/internal/tcpstack/stack_test.go
      Proof: test src/internal/tcpstack/stack_test.go TestForwardRequestsReleased

## Фаза 2: Живучесть транспорта и health-проверка

Цель: молча умершее соединение распознаётся и пересоздаётся; health смотрит на живую сессию.

- [x] T2.1 Включить HTTP/2 keepalive и таймауты: `ReadIdleTimeout`, `PingTimeout` для uTLS-транспорта; `net.Dialer{Timeout:10s, KeepAlive:15s}` в uTLS-диаллере. Outcome: blackhole-соединение закрывается с ошибкой запросов, а не висит. Touches: src/internal/transport/client.go, src/internal/tlsprofile/profile.go
      Proof: code src/internal/transport/client.go NewClientWithOptions
- [x] T2.2 Добавить health по живому клиенту: `transport.Client.Health(ctx)` (переиспользует существующий `http.Client`); `client.Client` хранит и отдаёт его; `mobile.Client.Health()` с ctx 5s. Outcome: проверка доступности без нового рукопожатия. Touches: src/internal/transport/client.go, src/internal/client/client.go, src/mobile/mobile.go
      Proof: code src/mobile/mobile.go Health
- [x] T2.3 Перевести Android health-probe с `Mobile.resolve` на `active.health()`; при ошибке — те же 2 неудачи → restart. Outcome: проверка отражает реальную сессию. Touches: src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt startV2
- [x] T2.4 Тест: `Health` успешен на живом h2-сервере и завершается ошибкой в пределах ctx на «замолчавшем». Outcome: bounded health проверяется автоматически. Touches: src/internal/transport/transport_test.go
      Proof: test src/internal/transport/transport_test.go TestClientHealthReusesConnection

## Фаза 3: Неблокирующий direct DNS

Цель: медленный резолв excluded-домена не останавливает packet pump.

- [x] T3.1 Вынести резолв из read-loop: инъектируемый lookup, диспетчеризация в ограниченный пул горутин, дроп при перегрузке. Outcome: обработка пакета-триггера возвращается сразу. Touches: src/internal/client/direct.go, src/internal/client/client.go
      Proof: code src/internal/client/direct.go dispatchDirectDNS
- [x] T3.2 Тест с медленным (блокирующимся) lookup: вызов обработчика DNS возвращается быстро, параллельный пакет обрабатывается. Outcome: блокирующий резолв не влияет на pump. Touches: src/internal/client/direct_test.go
      Proof: test src/internal/client/direct_test.go TestHandleDirectDNSEscapesReadLoop

## Фаза 4: Ограниченная остановка и проброс контекста

Цель: stop/restart освобождает TUN за секунды; отмена flow освобождает dial.

- [x] T4.1 Сделать fd неблокирующим: `SetNonblock`, ожидание готовности через `poll` с коротким таймаутом и проверкой stop-сигнала; `fdDevice.Close()` сигналит остановку, не закрывая чужой fd. Outcome: чтение прерывается за ≤200 мс. Touches: src/mobile/mobile.go
      Proof: code src/mobile/mobile.go Read
- [x] T4.2 Kotlin сначала останавливает Go-клиент, затем закрывает fd; fd больше не освобождается под активным чтением. Outcome: нет чтения по переиспользованному дескриптору. Touches: src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt stopClientAndTun
- [x] T4.3 Пробросить контекст сессии в `tcpstack.New` и использовать `dctx` в dial-функциях. Outcome: отмена flow отменяет исходящий dial. Touches: src/internal/tcpstack/stack.go, src/internal/client/client.go
      Proof: code src/internal/tcpstack/stack.go New
- [x] T4.4 Тесты: чтение fd-устройства прекращается после `Close()`; отмена flow отменяет блокирующийся dial. Outcome: bounded stop и ctx-отмена подтверждены тестами. Touches: src/mobile/fddevice_test.go, src/internal/tcpstack/stack_test.go
      Proof: test src/mobile/fddevice_test.go TestFDDeviceReadStopsOnClose

## Покрытие критериев приемки

- AC-001 -> T1.1, T1.2
- AC-002 -> T2.1, T2.4
- AC-003 -> T2.2, T2.3, T2.4
- AC-004 -> T3.1, T3.2
- AC-005 -> T4.1, T4.2, T4.4
- AC-006 -> T4.3, T4.4

## Заметки

- Порядок: фазы 1 и 4 — основной эффект; фазы 2-3 можно верифицировать независимо.
- `go test ./src/...` — основной прогон; Android-сборка проверяется отдельно, если доступен SDK.
- `Touches:` сверяются через `speckeep trace android-network-stall --files`.
