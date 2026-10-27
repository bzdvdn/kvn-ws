# Stealth surface hardening — Задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/config/client.go | T1.1, T1.3, T2.2, T2.3 |
| src/internal/transport/transport.go | T1.1 |
| src/internal/transport/websocket/wsfactory.go | T1.1 |
| src/internal/transport/websocket/websocket.go | T1.1, T1.2, T1.3, T2.3 |
| src/internal/bootstrap/client/dial.go | T1.1, T2.2, T2.3 |
| src/internal/bootstrap/client/tun.go | T2.2 |
| src/internal/bootstrap/server/server.go | T1.1, T2.1 |
| src/internal/bootstrap/server/handler.go | T2.1 |
| src/internal/transport/websocket/websocket_test.go | T3.1, T3.2, T3.3, T3.4 |
| src/integration/tunnel_integration_test.go | T3.1, T3.2, T3.4 |
| src/internal/bootstrap/server/ | T3.2 |
| src/internal/webui/frontend/src/types.ts | T2.4 |
| src/internal/webui/frontend/src/TabbedForm.tsx | T2.4 |
| src/internal/webui/handler_connect.go | T2.4 |
| src/internal/webui/multiserver_test.go | T2.4 |
| src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt | T5.1 |
| src/android/app/src/main/kotlin/com/kvn/client/transport/WebSocketClient.kt | T5.2, T5.3 |
| src/android/app/src/main/kotlin/com/kvn/client/transport/reconnect/ReconnectManager.kt | T5.3 |
| src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt | T5.2, T5.4 |
| src/android/app/src/main/kotlin/com/kvn/client/ui/SettingsScreen.kt, src/android/app/src/main/kotlin/com/kvn/client/ui/QrScannerScreen.kt | T5.1 |
| src/android/app/src/test/java/com/kvn/client/config/ConfigSerializationTest.kt, src/android/app/src/test/java/com/kvn/client/transport/ | T5.5 |
| CHANGELOG.md, docs/ru|en/config.md, docs/ru|en/architecture.md | T4.1 |
| src/internal/transport/websocket/, src/internal/bootstrap/client/, src/integration/ | T4.2 |

## Implementation Context

- Цель MVP: `padding.mode: random` без изменения wire-формата и с полной mixed-version совместимостью; дефолт `fixed`.
- Инварианты: reader использует только length prefix; `fixed` и `random` совместимы при `padding.enabled: true`; `padding.enabled: false` не меняется.
- Legacy: старые клиенты/серверы не должны получать новые обязательные заголовки или менять handshake; alias-списки дефолтятся на `X-KVN-NoPad`/`kvn-ws-mux`.
- Decoy: только не-WS запросы; валидный Upgrade идёт прежним путём.
- Jitter/headers: opt-in, с границами; серверный keepalive по умолчанию без изменений.
- Границы: не трогаем ALPN, HTTP/2/3, Android, relay, split-connection.
- Proof signals: mixed-version roundtrip, decoy+WS, jitter bounds, benchmark ≤5%.
- Trace: `@sk-task`/`@sk-test` на новых декларациях.

## Фаза 1: Padding mode

- [x] T1.1 Добавить `padding.mode` в конфиг и transport — outcome: `fixed` default, `random` прокидывается до `WSConfig`/`FactoryConfig` на client/server. Touches: src/internal/config/client.go, src/internal/transport/transport.go, src/internal/transport/websocket/wsfactory.go, src/internal/bootstrap/client/dial.go, src/internal/bootstrap/server/server.go
      Proof: code src/internal/config/client.go PaddingCfg
- [x] T1.2 Реализовать random padding — outcome: малые кадры падятся случайно, крупные без фиксированного выравнивания, `fixed` не меняется. Touches: src/internal/transport/websocket/websocket.go
      Proof: test src/internal/transport/websocket/websocket_test.go TestWSPaddingRandomMode
- [x] T1.3 Добавить alias-списки no-pad/subprotocol — outcome: клиент шлёт все no-pad имена, сервер принимает любое и отвечает совпавшим, дефолты старые. Touches: src/internal/config/client.go, src/internal/transport/websocket/websocket.go
      Proof: code src/internal/transport/websocket/websocket.go NoPadHeaders

## Фаза 2: Surface

- [x] T2.1 Добавить decoy API — outcome: GET без Upgrade на `/`, `/api/v1/events`, `/api/v1/health` получает правдоподобный JSON/401, WS не затронут. Touches: src/internal/bootstrap/server/server.go, src/internal/bootstrap/server/handler.go
      Proof: code src/internal/bootstrap/server/handler.go handleDecoy
- [x] T2.2 Добавить jitter — outcome: keepalive/reconnect клиента в заданных границах, дефолт legacy. Touches: src/internal/config/client.go, src/internal/bootstrap/client/dial.go, src/internal/bootstrap/client/tun.go
      Proof: code src/internal/bootstrap/client/reconnect.go jitterDelay
- [x] T2.3 Добавить headers profile — outcome: opt-in профиль реалистичных WS-заголовков, Origin не ставится по умолчанию. Touches: src/internal/config/client.go, src/internal/transport/websocket/websocket.go, src/internal/bootstrap/client/dial.go
      Proof: code src/internal/transport/websocket/websocket.go HeadersProfile
- [x] T2.4 Добавить настройки в kvn-web — outcome: per-server/global поля mode, aliases, headers и jitter сохраняются и мержатся. Touches: src/internal/webui/frontend/src/types.ts, src/internal/webui/frontend/src/TabbedForm.tsx, src/internal/webui/handler_connect.go, src/internal/webui/multiserver_test.go
      Proof: code src/internal/webui/frontend/src/TabbedForm.tsx StealthSurface

## Фаза 3: Валидация

- [x] T3.1 Покрыть padding и mixed-version — outcome: random↔random, random↔fixed в обе стороны работают. Touches: src/internal/transport/websocket/websocket_test.go, src/integration/tunnel_integration_test.go
      Proof: test src/internal/transport/websocket/websocket_test.go TestWSPaddingMixedModes
- [x] T3.2 Покрыть decoy/jitter/headers/subprotocol — outcome: decoy не ломает WS, jitter в границах, заголовки opt-in, subprotocol отсутствует при multiplex=false. Touches: src/internal/bootstrap/server/, src/internal/transport/websocket/websocket_test.go, src/integration/tunnel_integration_test.go
      Proof: test src/internal/bootstrap/server/decoy_test.go TestHandleDecoy
- [x] T3.3 Покрыть benchmark — outcome: random throughput/allocs в пределах бюджета. Touches: src/internal/transport/websocket/websocket_test.go
      Proof: test src/internal/transport/websocket/websocket_test.go BenchmarkWSPaddingModes
- [x] T3.4 Покрыть alias-негоциацию — outcome: old↔new no-pad и subprotocol матрица проходит. Touches: src/internal/transport/websocket/websocket_test.go, src/integration/tunnel_integration_test.go
      Proof: test src/internal/transport/websocket/websocket_test.go TestNoPadHeaderAliases

## Фаза 4: Документация и gate

- [x] T4.1 Обновить docs/CHANGELOG — outcome: описаны режимы, совместимость и ограничения. Touches: CHANGELOG.md, docs/ru/config.md, docs/en/config.md, docs/ru/architecture.md, docs/en/architecture.md
      Proof: docs docs/ru/config.md
- [x] T4.2 Прогнать gate — outcome: race-тесты и `go vet ./...` зелёные. Touches: src/internal/transport/websocket/, src/internal/bootstrap/client/, src/integration/
      Proof: test src/internal/transport/websocket/websocket_test.go TestWSPaddingRandomMode

## Фаза 5: Android parity

- [x] T5.1 Добавить Android config/QR/UI поля — outcome: mode, no-pad headers, subprotocols, headers profile и jitter сохраняются/экспортируются. Touches: src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt, src/android/app/src/main/kotlin/com/kvn/client/ui/SettingsScreen.kt, src/android/app/src/main/kotlin/com/kvn/client/ui/QrScannerScreen.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt ConnectionConfig
- [x] T5.2 Добавить random padding и no-pad aliases в WebSocketClient — outcome: mixed fixed/random работает, secondary снимает padding только после подтверждения alias. Touches: src/android/app/src/main/kotlin/com/kvn/client/transport/WebSocketClient.kt, src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/transport/WebSocketClient.kt requestNoPad
- [x] T5.3 Добавить subprotocol/headers/reconnect jitter — outcome: alias subprotocol, browser headers и jitter применяются только по конфигу. Touches: src/android/app/src/main/kotlin/com/kvn/client/transport/WebSocketClient.kt, src/android/app/src/main/kotlin/com/kvn/client/transport/reconnect/ReconnectManager.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/transport/reconnect/ReconnectManager.kt delayForAttempt
- [x] T5.4 Прокинуть поля через KvnVpnService — outcome: primary/secondary получают mode/headers/subprotocols/jitter из ConnectionConfig. Touches: src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt createTransport
- [x] T5.5 Покрыть Android тестами — outcome: padding, aliases, config/QR roundtrip и jitter покрыты unit-тестами. Touches: src/android/app/src/test/java/com/kvn/client/config/ConfigSerializationTest.kt, src/android/app/src/test/java/com/kvn/client/transport/
      Proof: test src/android/app/src/test/java/com/kvn/client/transport/WsPaddingTest.kt WsPaddingTest
- [x] T5.6 Прогнать Android gate — outcome: `./gradlew test assembleDebug` зелёный. Touches: src/android/app/
      Proof: test src/android/app/src/test/java/com/kvn/client/transport/ReconnectJitterTest.kt ReconnectJitterTest

## Покрытие критериев приемки

- AC-001 -> T1.1, T1.2, T3.1
- AC-002 -> T1.2, T3.1
- AC-003 -> T1.1, T3.1, T4.2
- AC-004 -> T2.1, T3.2
- AC-005 -> T2.2, T3.2
- AC-006 -> T2.3, T2.4, T3.2
- AC-007 -> T2.3, T3.2
- AC-008 -> T1.2, T3.3
- AC-009 -> T1.3, T2.4, T3.4
- AC-010 -> T1.3, T2.4, T3.4
- AC-011 -> T5.1, T5.2, T5.5
- AC-012 -> T5.1, T5.2, T5.4, T5.5
- AC-013 -> T5.1, T5.5
- AC-014 -> T5.1, T5.3, T5.5

## Заметки

- Data model: no change; contracts-файлы не создаются.
- Дефолт `fixed` и отсутствие новых обязательных заголовков — главный compatibility gate.
