# Doze-устойчивость VPN-соединения — Задачи

## Phase Contract

Inputs: plan (pass), data-model (minor-change), spec.
Outputs: tasks.md.
Stop if: нет.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/config/server.go` | T1.1 |
| `src/internal/transport/websocket/websocket.go` | T1.2 |
| `src/internal/bootstrap/server/handler.go` | T1.2 |
| `src/android/app/src/main/AndroidManifest.xml` | T2.1 |
| `src/android/app/src/main/kotlin/.../vpn/KvnVpnService.kt` | T2.1, T2.2, T3.2 |
| `src/android/app/src/main/kotlin/.../vpn/.../SmartSocketFactory` (внутри KvnVpnService) | T2.2 |
| `src/android/app/src/main/kotlin/.../config/AppConfig.kt` | T3.1 |
| `src/android/app/src/main/kotlin/.../ui/` (ConnectScreen/SettingsScreen) | T3.2 |
| `src/internal/transport/websocket/websocket_test.go` | T5.1 |
| `src/android/app/src/test/.../` | T5.2 |
| `configs/server.yaml`, `docs/` | T5.2 |

## Implementation Context

- **Цель MVP:** server pong_timeout=120s + Android WakeLock + TCP keepalive — screen-off 2 мин без разрыва
- **Инварианты/семантика:**
  - WakeLock lifecycle = service lifecycle, не transport; acquire при toggle ON + CONNECTED, release при STOP/DISCONNECTED
  - TCP keepalive включается на raw socket внутри SmartSocketFactory.createSocket() при toggle ON
  - Server pong_timeout: zero value → DefaultPongTimeout (45s fallback); явное значение → override
  - Screen-on reconnect: bypass backoff — прямой вызов createTransport().connect()
- **Ошибки/коды:**
  - WakeLock acquire/release — безопасный multiple release (isHeld check)
  - Socket.setKeepAlive — unchecked exception не выбрасывает; не требует try/catch
- **Контракты/протокол:**
  - `server.yaml: transport.ws.pong_timeout: 120s` — duration format (10s, 2m)
  - `ConnectionConfig.keepAwakeEnabled` — DataStore, default false
- **Границы scope:**
  - Не меняем ReconnectManager — только прямой вызов connect() при screen-on
  - Не меняем wire protocol, FrameCodec, Handshake
- **Proof signals:**
  - `dumpsys power | grep "LOCK.*kvn"` показывает WakeLock
  - `Socket.getKeepAlive()` = true
  - server log при старте `"ws pong_timeout"=120s`
  - UI toggle visible, default off
- **References:** DEC-001..DEC-005, DM (data-model.md)

## Фаза 1: Серверная инфраструктура

Цель: конфигурируемый PongTimeout на сервере, deployable независимо.

- [x] T1.1 Добавить `PongTimeout` в `TransportWSCfg` (`server.go`) с `mapstructure:"pong_timeout"` и fallback на `DefaultPongTimeout` при zero value. Touches: `src/internal/config/server.go`
- [x] T1.2 Параметризовать `WSConn.SetKeepalive(interval, timeout)` — передавать timeout из cfg; в `handler.go` читать `cfg.Transport.WS.PongTimeout`; обновить `DefaultPongTimeout` с 45s до 120s. Touches: `src/internal/transport/websocket/websocket.go`, `src/internal/bootstrap/server/handler.go`, `src/internal/protocol/control/control.go`

## Фаза 2: Android MVP — WakeLock + TCP keepalive

Цель: WakeLock и TCP keepalive без UI toggle (WakeLock всегда on на этом этапе — временно).

- [x] T2.1 Добавить `WAKE_LOCK` permission в `AndroidManifest.xml`; в `KvnVpnService.doStart()` — acquire `PARTIAL_WAKE_LOCK`, в `onDestroy()` — release; проверить `isHeld` перед release. Touches: `src/android/app/src/main/AndroidManifest.xml`, `KvnVpnService.kt`
- [x] T2.2 В `SmartSocketFactory.createSocket()` (внутри `KvnVpnService.kt`) — вызвать `raw.setKeepAlive(true)` на созданном raw socket. Touches: `KvnVpnService.kt` (SmartSocketFactory)

## Фаза 3: UI toggle + Screen-on listener

Цель: пользовательский контроль WakeLock и автоматическое восстановление при screen-on.

- [x] T3.1 Добавить поле `keepAwakeEnabled: Boolean = false` в `ConnectionConfig` (`AppConfig.kt`); обновить `WakeLock` acquire/release — проверять `config.keepAwakeEnabled` перед acquire. Touches: `src/android/app/src/main/kotlin/.../config/AppConfig.kt`, `KvnVpnService.kt`
- [x] T3.2 Добавить toggle "Удерживать соединение при выкл. экране" в экран настроек/подключения (ConnectScreen/SettingsScreen) — пишет в `keepAwakeEnabled`. Touches: `src/android/app/src/main/kotlin/.../ui/SettingsScreen.kt`
- [x] T3.3 Зарегистрировать `BroadcastReceiver` на `SCREEN_ON` в `KvnVpnService.doStart()`; при срабатывании — проверить `isConnected()`, если false → прямой вызов `createTransport().connect()` (без `ReconnectManager`). Touches: `KvnVpnService.kt`

## Фаза 4: Проверка

Цель: automated tests + manual validation path.

- [x] T4.1 Go unit tests: `TestSetKeepaliveWithCustomTimeout` — проверить что `SetKeepalive(25s, 120s)` выставляет read deadline = now+120s; `TestDefaultPongTimeoutFallback` — zero cfg → 45s. Touches: `src/internal/transport/websocket/websocket_test.go`
- [x] T4.2 Android unit test: `ConnectionConfig` default `keepAwakeEnabled` = false. Manual: APK + server с pong_timeout=120s, screen-off 2 мин → проверка `dumpsys power`, логов (`WakeLock acquired`), UI toggle visible. Touches: `src/android/app/src/test/`, `configs/server.yaml`, `docs/`

## Покрытие критериев приемки

- AC-001 -> T2.1, T4.2
- AC-002 -> T2.2, T4.1
- AC-003 -> T3.3, T4.2
- AC-004 -> T3.3, T4.2
- AC-005 -> T1.1, T1.2, T4.1
- AC-006 -> T2.1, T2.2, T4.2
- AC-007 -> T3.1, T3.2, T4.2

## Заметки

- T1.1 и T1.2 можно параллелить с T2.1/T2.2 (server & Android независимы)
- T3.1 зависит от T2.1 (нужен WakeLock код) — merge в одну сборку
- T3.2 зависит от T3.1 (нужно поле в конфиге)
- T3.3 независим от T3.1/T3.2 — можно делать параллельно после Фазы 2
