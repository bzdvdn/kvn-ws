---
report_type: verify
slug: doze-resilience
status: pass
docs_language: ru
generated_at: 2026-07-09
---

# Verify Report: doze-resilience

## Scope

- snapshot: Doze-устойчивость VPN-соединения — WakeLock + WifiLock, TCP keepalive, screen-on listener, pong_timeout config, бесконечный reconnect loop
- verification_mode: deep (v2 — исправление reconnect loop, tunReader safeStop, WifiLock)
- artifacts:
  - specs/active/doze-resilience/tasks.md
  - specs/active/doze-resilience/spec.md
  - specs/active/doze-resilience/plan.md
- inspected_surfaces:
  - src/internal/config/server.go — PongTimeout field + fallback
  - src/internal/transport/websocket/websocket.go — DefaultPongTimeout + SetKeepalive
  - src/internal/bootstrap/server/handler.go — чтение PongTimeout из cfg
  - src/android/app/src/main/AndroidManifest.xml — WAKE_LOCK permission
  - src/android/app/.../vpn/KvnVpnService.kt — WakeLock, WifiLock, TCP keepalive, SCREEN_ON+USER_PRESENT receiver, reconnect, tunReader
  - src/android/app/.../config/AppConfig.kt — keepAwakeEnabled field
  - src/android/app/.../ui/SettingsScreen.kt — UI toggle
  - src/android/app/.../config/AppConfigTest.kt — keepAwakeEnabled default tests
  - src/internal/transport/websocket/websocket_test.go — Go unit tests
  - configs/server.yaml — pong_timeout: 120s
  - docs/ru/config.md, docs/en/config.md — pong_timeout docs

## Verdict

- status: pass
- archive_readiness: safe
- summary: 9 задач + 3 дополнительных исправления (WifiLock, tunReader safeStop, reconnect loop). Подтверждено пользовательским тестированием — VPN стабилен при screen-off с toggle ON.

## Verification Matrix

| AC-ID | Task IDs | Evidence | Verdict |
|-------|----------|----------|---------|
| AC-001 | T2.1, T4.2 | `AndroidManifest.xml:13` WAKE_LOCK permission; `KvnVpnService.kt:77,529-546` WakeLock + WifiLock acquire/release; `AppConfigTest.kt:51,58` testKeepAwakeDefaultFalse/testKeepAwakeCanBeTrue | pass |
| AC-002 | T2.2, T4.1 | `KvnVpnService.kt:163` TCP keepalive на raw socket; `TestSetKeepaliveWithCustomTimeout: pass` | pass |
| AC-003 | T3.3, T4.2 | `KvnVpnService.kt:63,81,548-585` SCREEN_ON + USER_PRESENT receiver, stop ReconnectManager, reset reconnect; configs/server.yaml содержит pong_timeout | pass |
| AC-004 | T3.3, T4.2 | `KvnVpnService.kt:548-585` registerReceiver SCREEN_ON+USER_PRESENT, unregister в onDestroy; reconnect через прямой createTransport().connect() после остановки ReconnectManager | pass |
| AC-005 | T1.1, T1.2, T4.1 | `server.go:23` PongTimeout c mapstructure; `server.go:236` fallback на DefaultPongTimeout; `handler.go:37` чтение cfg; `websocket.go:25` DefaultPongTimeout=120s; `TestDefaultPongTimeout: pass`; `TestSetKeepaliveWithCustomTimeout: pass` | pass |
| AC-006 | T2.1, T2.2, T4.2 | WakeLock+WifiLock (AC-001 proof) + TCP keepalive (AC-002 proof) — комбинация подтверждена пользователем | pass |
| AC-007 | T3.1, T3.2, T4.2 | `AppConfig.kt:80` keepAwakeEnabled=false default; `SettingsScreen.kt:80,309` UI toggle; `KvnVpnService.kt:164,525` conditional acquire; `AppConfigTest.kt:51,58` testKeepAwakeDefaultFalse/testKeepAwakeCanBeTrue | pass |

## Checks

- task_state: completed=9, open=0
- acceptance_evidence: 7/7 AC подтверждены (см. матрицу)
- implementation_alignment:
  - T1.1 — `server.go:23,236` PongTimeout + fallback
  - T1.2 — `handler.go:37`, `websocket.go:25` DefaultPongTimeout=120s
  - T2.1 — WAKE_LOCK permission + WakeLock + WifiLock acquire/release
  - T2.2 — TCP keepalive conditional на raw socket
  - T3.1 — keepAwakeEnabled=false default, conditional locks
  - T3.2 — Switch toggle в SettingsScreen
  - T3.3 — SCREEN_ON + USER_PRESENT receiver + bypass backoff
  - T4.1 — `TestDefaultPongTimeout` + `TestSetKeepaliveWithCustomTimeout`: pass
  - T4.2 — AppConfigTest, server.yaml, docs updated

## Дополнительные исправления (v2)

| Issue | Fix | Evidence |
|-------|-----|----------|
| tunReader вызывал `safeStop()` после `closeTun()` при reconnect | `catch { break }` без `safeStop()` | `KvnVpnService.kt:903-906` |
| writeToTun вызывал `safeStop()` при записи в закрытый TUN | `catch { /* swallow */ }` | `KvnVpnService.kt:917-921` |
| ReconnectManager блокировался флагом `reconnectStarted` (одноразовый gate) | `reconnectStarted` удалён; DISCONNECTED всегда вызывает `reconnectManager?.start()` | `KvnVpnService.kt:720-724` |
| WifiLock не использовался (WiFi радио засыпало при Doze) | Добавлен `WifiLock WIFI_MODE_FULL_HIGH_PERF` | `KvnVpnService.kt:535-545` |
| Screen-on ловил только `ACTION_SCREEN_ON`, не `ACTION_USER_PRESENT` | `IntentFilter` с обоими action | `KvnVpnService.kt:569-571` |
| `DefaultPongTimeout` в `websocket.go` оставался 45s (было только в `control.go`) | Исправлен на 120s | `websocket.go:25` |

## Traceability

- total annotations: 30 (26 code + 4 test)
- `@sk-task` found for: T1.1, T1.2, T2.1, T2.2, T3.1, T3.2, T3.3
- `@sk-test` found for: T4.1 (2 tests), T4.2 (2 tests)
- all 9 tasks have evidence

## Errors

- none

## Warnings

- `WIFI_MODE_FULL_HIGH_PERF` deprecated на API 29+, но функционален

## Not Verified

- Android UI toggle визуальная проверка (manual path) — не автоматизирована, но код присутствует и покрыт unit-тестами

## Next Step

- safe to archive
