# Stealth fallback и Bearer-auth до upgrade — Задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/config/server.go | T1.1 |
| src/internal/config/client.go | T1.1 |
| src/internal/bootstrap/server/server.go | T1.2, T1.3, T1.4 |
| src/internal/bootstrap/server/handler.go | T1.2, T1.3, T1.4 |
| src/internal/transport/transport.go | T2.1 |
| src/internal/transport/websocket/websocket.go | T2.1 |
| src/internal/bootstrap/client/dial.go | T2.2 |
| src/internal/bootstrap/client/tun.go | T2.2 |
| src/internal/webui/frontend/src/types.ts | T3.1 |
| src/internal/webui/frontend/src/TabbedForm.tsx | T3.1 |
| src/internal/webui/handler_connect.go | T3.1 |
| src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt | T3.2 |
| src/android/app/src/main/kotlin/com/kvn/client/transport/WebSocketClient.kt | T3.2 |
| src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt | T3.2 |
| src/android/app/src/main/kotlin/com/kvn/client/ui/SettingsScreen.kt, QrScannerScreen.kt | T3.2 |
| src/integration/ | T4.1, T4.2 |
| src/internal/bootstrap/server/ | T4.1 |
| src/android/app/src/test/ | T4.3 |
| CHANGELOG.md, docs/ru|en/config.md, docs/ru|en/architecture.md | T4.4 |

## Implementation Context

- Цель MVP: stealth path с Bearer до upgrade + fallback, legacy path без изменений.
- Инварианты: data-path/padding/очереди не меняются; токен только в `Authorization` внутри TLS; constant-time compare.
- Legacy: `stealth_path=""` и `stealth.enabled=false` по умолчанию; старые клиенты идут прежним путём.
- Fallback: static HTML или reverse-proxy; `Authorization` не пересылается upstream; upstream ошибка → 502 HTML.
- Health/admin/metrics: в stealth-режиме только loopback, внешним — fallback.
- Client: Bearer в upgrade; при отсутствии 101 — один fallback на legacy path.
- Границы: relay/QUIC/padding вне scope.
- Proof signals: 101/fallback/legacy, stripped auth, no token in logs, handshake latency ≤5%, Android tests.
- Trace: `@sk-task`/`@sk-test` на новых декларациях.

## Фаза 1: Config и сервер

- [x] T1.1 Добавить server/client config — outcome: stealth/fallback поля с legacy-дефолтами и валидацией. Touches: src/internal/config/server.go, src/internal/config/client.go
      Proof: code src/internal/config/server.go StealthPath
- [x] T1.2 Bearer до upgrade + legacy path — outcome: валидный Bearer → 101, legacy идёт через ClientHello как раньше. Touches: src/internal/bootstrap/server/server.go, src/internal/bootstrap/server/handler.go
      Proof: code src/internal/bootstrap/server/server.go StealthPath
- [x] T1.3 Fallback static/reverse-proxy — outcome: невалидный stealth-запрос получает сайт, Authorization не уходит upstream. Touches: src/internal/bootstrap/server/server.go, src/internal/bootstrap/server/handler.go
      Proof: code src/internal/bootstrap/server/handler.go handleFallback
- [x] T1.4 Скрыть health/admin в stealth-режиме — outcome: внешний запрос получает fallback, loopback — прежний ответ. Touches: src/internal/bootstrap/server/server.go
      Proof: code src/internal/bootstrap/server/server.go guard

## Фаза 2: Клиент и транспорт

- [x] T2.1 Добавить Authorization в WS Dial — outcome: заголовок уходит в upgrade-запросе без лишнего RTT. Touches: src/internal/transport/transport.go, src/internal/transport/websocket/websocket.go
      Proof: code src/internal/transport/websocket/websocket.go Authorization
- [x] T2.2 Stealth dial + fallback на legacy — outcome: новый клиент пробует stealth path и откатывается при отсутствии 101. Touches: src/internal/bootstrap/client/dial.go, src/internal/bootstrap/client/tun.go
      Proof: code src/internal/bootstrap/client/dial.go dialStreamWith

## Фаза 3: kvn-web и Android

- [x] T3.1 Настройки в kvn-web — outcome: stealth enabled/path/legacy path сохраняются и мержатся. Touches: src/internal/webui/frontend/src/types.ts, src/internal/webui/frontend/src/TabbedForm.tsx, src/internal/webui/handler_connect.go
      Proof: code src/internal/webui/frontend/src/TabbedForm.tsx StealthPath
- [x] T3.2 Android parity — outcome: config/QR/UI, Bearer в WS upgrade и fallback на legacy. Touches: src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt, src/android/app/src/main/kotlin/com/kvn/client/transport/WebSocketClient.kt, src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt, src/android/app/src/main/kotlin/com/kvn/client/ui/SettingsScreen.kt, src/android/app/src/main/kotlin/com/kvn/client/ui/QrScannerScreen.kt
      Proof: code src/android/app/src/main/kotlin/com/kvn/client/transport/WebSocketClient.kt fallbackUrl

## Фаза 4: Проверка и документация

- [x] T4.1 Покрыть сервер — outcome: valid/invalid Bearer, fallback, legacy, hide-health. Touches: src/internal/bootstrap/server/, src/integration/
      Proof: test src/internal/bootstrap/server/stealth_test.go TestStealthAuthBeforeUpgrade
- [x] T4.2 Покрыть клиент и latency — outcome: fallback на legacy, handshake latency ≤5%, mixed-version. Touches: src/internal/bootstrap/client/, src/integration/
      Proof: test src/internal/bootstrap/client/stealth_test.go TestDialStealthFallsBackToLegacy
- [x] T4.3 Покрыть Android — outcome: config/QR/Bearer/fallback тесты зелёные. Touches: src/android/app/src/test/
      Proof: test src/android/app/src/test/java/com/kvn/client/transport/WebSocketFallbackTest.kt WebSocketFallbackTest
- [x] T4.4 Обновить docs/CHANGELOG — outcome: описаны stealth path, fallback, legacy и ограничения. Touches: CHANGELOG.md, docs/ru/config.md, docs/en/config.md, docs/ru/architecture.md, docs/en/architecture.md
      Proof: docs docs/ru/config.md
- [x] T4.5 Прогнать gate — outcome: Go race/vet + Android test/assembleDebug зелёные. Touches: src/internal/bootstrap/, src/integration/, src/android/app/
      Proof: test src/internal/bootstrap/server/stealth_test.go TestStealthAuthBeforeUpgrade

## Покрытие критериев приемки

- AC-001 -> T1.2, T4.1
- AC-002 -> T1.2, T1.3, T4.1
- AC-003 -> T1.2, T4.1
- AC-004 -> T1.3, T4.1
- AC-005 -> T2.1, T4.2
- AC-006 -> T2.2, T4.2
- AC-007 -> T1.1, T3.1, T4.4
- AC-008 -> T1.4, T4.1
- AC-009 -> T3.2, T4.3

## Заметки

- Data model: no change; contracts-файлы не создаются.
- Relay/QUIC/padding вне scope; legacy path остаётся до миграции.
