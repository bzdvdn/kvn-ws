# Doze-устойчивость VPN-соединения — План

## Phase Contract

Inputs: spec + inspect (pass) + repo surfaces (KvnVpnService, SmartSocketFactory, WSConn, server config).
Outputs: plan, data-model.
Stop if: нет.

## Цель

Реализовать WakeLock, TCP keepalive, screen-on listener на Android и конфигурируемый pong_timeout на Go-сервере. Изменения локальны: Android — 3 файла ядра + UI + manifest; Go — 2 файла (config + transport). Обратная совместимость полная: toggle off по умолчанию, старый сервер без pong_timeout использует 45s.

## MVP Slice

Server-side pong_timeout конфиг (120s) + Android WAKE_LOCK permission + WakeLock acquire/release + TCP keepalive на сокете. Закрывает AC-001, AC-002, AC-005, AC-006. Не требует UI-изменений (WakeLock всегда on на этом этапе).

## First Validation Path

1. Развернуть сервер с `pong_timeout: 120s`
2. Установить APK с WakeLock + TCP keepalive
3. Подключиться, выключить экран на 2 мин — проверить что `dumpsys power` показывает WakeLock, соединение не рвётся
4. Включить экран — трафик идёт без reconnect

## Scope

- Android: `KvnVpnService.kt` — WakeLock lifecycle, screen-on listener
- Android: `SmartSocketFactory` — `setKeepAlive(true)`
- Android: `ConnectionConfig` / `AppConfig.kt` — `keepAwakeEnabled` поле
- Android: UI — toggle в экране настроек/подключения
- Android: `AndroidManifest.xml` — `WAKE_LOCK` permission
- Go: `src/internal/transport/websocket/websocket.go` — `DefaultPongTimeout = 120s`, параметризация
- Go: `src/internal/config/server.go` — `pong_timeout` в `[transport.ws]`
- Go: `src/internal/bootstrap/server/handler.go` — передача pong_timeout в SetKeepalive

**Границы:** ReconnectManager, TUN read/write, FrameCodec, Handshake — не меняются.

## Performance Budget

- `none` — WakeLock и TCP keepalive не создают заметной дополнительной нагрузки; pingInterval 15s→10s — тривиально.

## Implementation Surfaces

### Android (Kotlin)

| Surface | Файл | Изменение |
|---------|------|-----------|
| Service | `KvnVpnService.kt` | +WakeLock acquire/release, +screen BroadcastReceiver, +TCP keepalive в SmartSocketFactory |
| Config model | `AppConfig.kt` | +`keepAwakeEnabled: Boolean = false` |
| UI | `ConnectScreen.kt` / `SettingsScreen.kt` | +toggle "Удерживать при выкл. экране" |
| Manifest | `AndroidManifest.xml` | +`WAKE_LOCK` permission |

### Go (server)

| Surface | Файл | Изменение |
|---------|------|-----------|
| Config | `src/internal/config/server.go` | +`PongTimeout` в `TransportWSCfg` |
| Transport | `src/internal/transport/websocket/websocket.go` | +`DefaultPongTimeout=120s`, SetKeepalive принимает timeout из config |
| Bootstrap | `src/internal/bootstrap/server/handler.go` | чтение `cfg.Transport.WS.PongTimeout` → `SetKeepalive` |

## Bootstrapping Surfaces

- `none` — все нужные файлы существуют.

## Влияние на архитектуру

- WakeLock изолирован в `KvnVpnService` — не просачивается в TransportClient/WebSocketClient.
- Server pong_timeout — backwards-compatible: zero value = 45s (старое поведение).
- UI toggle пишется в `ConnectionConfig` (DataStore) — существующий паттерн.

## Acceptance Approach

- AC-001: `dumpsys power | grep "LOCK.*kvn"` при screen-off. Поверхность: KvnVpnService.
- AC-002: `Socket.getKeepAlive()` в тесте или `adb shell cat /proc/net/tcp`. Поверхность: SmartSocketFactory.
- AC-003: лог `"SCREEN_ON: connection dead, reconnecting"`. Поверхность: KvnVpnService + screen BroadcastReceiver.
- AC-004: замер <5000ms между RECONNECTING и CONNECTED по логам. Поверхность: KvnVpnService + ReconnectManager (bypass).
- AC-005: стартовый лог сервера `"ws pong_timeout"=120s`. Поверхность: server config + bootstrap.
- AC-006: screen-off 120s → CONNECTED после screen-on. Интеграционная проверка.
- AC-007: toggle visible в UI, по умолчанию off, логи WakeLock acquired/released. Поверхность: UI + AppConfig + KvnVpnService.

## Данные и контракты

- `data-model.md` содержит изменения модели.
- Контракты API/events не меняются — pong_timeout только server-side config, не влияет на wire protocol.
- ConnectionConfig добавляет поле `keepAwakeEnabled` — обратно совместимо (default false, DataStore migration не нужна для нового поля).

## Стратегия реализации

### DEC-001 WakeLock lifecycle привязан к service, не transport

- Why: WakeLock должен переживать reconnect (транспорт может пересоздаваться). Service.onStartCommand/onDestroy — естественные границы.
- Tradeoff: WakeLock удерживается даже если соединение временно потеряно (при toggle on). Это intentional — пользователь хочет стабильность.
- Affects: KvnVpnService (doStart/onDestroy), ConnectionConfig (keepAwakeEnabled)
- Validation: AC-001

### DEC-002 TCP keepalive управляется тем же toggle

- Why: отдельный toggle для TCP keepalive избыточен — WakeLock уже даёт CPU wake, TCP keepalive только резерв.
- Tradeoff: нельзя включить keepalive без WakeLock. Для screen-off сценария это ok.
- Affects: SmartSocketFactory.createSocket()
- Validation: AC-002

### DEC-003 Screen-on listener в KvnVpnService, не отдельный BroadcastReceiver

- Why: service уже имеет ServiceScope, регистрация в onStartCommand + unregister в onDestroy.
- Tradeoff: не переживает перезапуск service — но при перезапуске соединение переустанавливается заново.
- Affects: KvnVpnService (registerReceiver)
- Validation: AC-003, AC-004

### DEC-004 PongTimeout конфигурация как `transport.ws.pong_timeout` duration

- Why: следует существующему паттерну viper + mapstructure. Hanya tambah field baru.
- Tradeoff: требует знания YAML duration формата (10s, 2m). Документируется в пример-конфиге.
- Affects: config/server.go, websocket.go (SetKeepalime), handler.go
- Validation: AC-005

### DEC-005 Bypass backoff при screen-on — отдельный метод `forceReconnect()`

- Why: ReconnectManager.reset() + немедленный reconnect нарушает инкапсуляцию. Проще вызвать существующий reconnect напрямую.
- Tradeoff: дублирование логики создания транспорта. Приемлемо — 2 строки в KvnVpnService.
- Affects: KvnVpnService (onScreenOn), ReconnectManager не меняется
- Validation: AC-004

## Incremental Delivery

### MVP (Первая ценность)

- Server: pong_timeout default 120s + конфиг
- Android: WAKE_LOCK permission + WakeLock всегда on (без toggle)
- Android: Socket.setKeepAlive(true)
- AC: 001, 002, 005, 006

### Итерация 2 (UI toggle)

- Android: keepAwakeEnabled в ConnectionConfig (default false)
- Android: toggle в UI
- AC: 007
- WakeLock acquire/release по состоянию toggle

### Итерация 3 (Screen-on listener)

- Android: BroadcastReceiver SCREEN_ON
- Android: forceReconnect() при screen-on без backoff
- AC: 003, 004

## Порядок реализации

1. **Server** (можно параллельно с Android): pong_timeout config + default → deploy → проверка AC-005
2. **Android MVP**: manifest + WakeLock + TCP keepalive — AC-001, AC-002, AC-006
3. **Android toggle**: AppConfig + UI + conditional WakeLock — AC-007
4. **Android screen-on**: BroadcastReceiver + forceReconnect — AC-003, AC-004

Пункты 2 и 3 можно делать последовательно или в одной сборке — toggle без WakeLock тривиально добавляется.

## Риски

- **Риск 1: MIUI блокирует PARTIAL_WAKE_LOCK на уровне прошивки**
  Mitigation: AC-003/AC-004 (reconnect при screen-on) — fallback, если WakeLock не сработал.
- **Риск 2: Battery Saver отзывает WakeLock**
  Mitigation: корректный shutdown TUN при revocation (уже есть в safeStop).
- **Риск 3: OkHttp pingInterval=10s увеличивает трафик**
  Mitigation: 10s — стандарт для WebSocket ping, overhead < 1 KB/час.

## Rollout и compatibility

- Server config: zero value `PongTimeout = 0` → fallback к `DefaultPongTimeout` (45s для обратной совместимости). Новый default 120s — только при явном `pong_timeout: 120s` или при установке в config.
- Android toggle: default false — поведение не меняется для существующих установок.
- Android WAKE_LOCK permission: добавляется в манифест — не требует runtime-запроса (normal permission).
- Специальных rollout-действий не требуется.

## Проверка

- **Go**: `go test -race ./src/internal/transport/websocket/...` — тесты SetKeepalive с разными timeout
- **Go**: `go test -race ./src/internal/config/...` — парсинг pong_timeout
- **Android unit**: `./gradlew test` — тесты ConnectionConfig c keepAwakeEnabled
- **Manual**: APK + server с pong_timeout=120s — screen-off 2 мин, проверка логов и WakeLock
- **AC-001**: `dumpsys power`
- **AC-002**: `adb shell cat /proc/net/tcp`
- **AC-005**: server log при старте
- **AC-006**: Connectivity check после screen-on
- **AC-007**: UI visible + toggle default off

## Соответствие конституции

- нет конфликтов
