# Android: оптимизация latency hot path и устранение battery settings popup при connect

## Scope Snapshot

- In scope: снижение per-packet latency в TUN read/write path и устранение системного диалога battery optimization при каждом connect.
- Out of scope: Go server/relay path, QUIC transport, desktop/web UI, рефакторинг архитектуры Android-приложения.

## Цель

Пользователь Android-клиента получает стабильный VPN-туннель без всплывающих системных диалогов при каждом подключении и с ощутимо меньшей задержкой при передаче трафика (прирост latency на hot path). Успех измеряется отсутствием `Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` в service lifecycle при повторных connect и снижением per-packet allocation на TUN read/write path.

## Основной сценарий

1. Пользователь открывает приложение, переходит в Settings.
2. Battery exemption запрашивается однократно (в UI), а не при каждом старте VpnService.
3. При каждом connect VpnService не дёргает системный диалог.
4. TUN reader/writer используют buffer pool и кешированный Cipher instance — без per-packet аллокаций в hot loop.
5. Traffic counters обновляются батчами, а не на каждый пакет.
6. Per-packet логирование выключено на release-сборках.

## User Stories

- P1: Пользователь подключается повторно — battery settings диалог не появляется.
- P1: Пользователь передаёт трафик через туннель — latency не растёт со временем из-за GC pressure.
- P2: Release-сборка не логирует каждый пакет (снижение CPU/IO noise).

## MVP Slice

Вынос `requestBatteryExemptionIfNeeded()` из `doStart()` (AC-001) + buffer pool для TUN read (AC-002). Остальное — следующий раунд.

## First Deployable Outcome

APK, в котором при повторном connect battery exemption не запрашивается, а TUN reader не аллоцирует `ByteArray` на каждый пакет. Проверяется: `adb logcat | grep -i battery` не показывает `ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` после первого connect/disconnect/connect цикла.

## Scope

- `src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt`
- `src/android/app/src/main/kotlin/com/kvn/client/crypto/AesGcmCipher.kt`
- `src/android/app/src/main/kotlin/com/kvn/client/protocol/FrameCodec.kt`
- `src/android/app/src/main/kotlin/com/kvn/client/ui/MainViewModel.kt`
- `src/android/app/src/main/kotlin/com/kvn/client/ui/SettingsScreen.kt`
- `src/android/app/src/main/kotlin/com/kvn/client/transport/WebSocketClient.kt`

## Контекст

- VpnService.run() вызывается при каждом connect; `requestBatteryExemptionIfNeeded()` стартует Activity, что недопустимо в hot restart path.
- AES-GCM `Cipher.getInstance()` — тяжёлая операция (JCE provider lookup), не должна вызываться на пакет.
- OkHttp WebSocket listener вызывает `onMessage` в том же thread, блокируя чтение следующих фреймов — любые аллокации в этом колбэке увеличивают latency для последующих пакетов.
- Release-сборка не должна содержать per-packet `AppLogger.i()` — это I/O на каждый пакет.
- `@sk-task` trace-маркеры обязательны на изменяемых объявлениях.

## Зависимости

- none (все изменения в рамках одного Android-модуля)

## Требования

### RQ-001 Battery exemption request только из UI

Система НЕ ДОЛЖНА вызывать `Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` из `VpnService.onStartCommand()` или `doStart()`. Battery exemption запрашивается только из SettingsScreen (или при первом запуске).

### RQ-002 Buffer pool для TUN read

TUN reader НЕ ДОЛЖЕН аллоцировать `ByteArray(config.mtu)` в каждой итерации цикла. Используется переиспользуемый буфер или пул.

### RQ-003 Кеширование Cipher.getInstance()

AES-GCM encrypt/decrypt НЕ ДОЛЖНЫ вызывать `Cipher.getInstance()` на каждый пакет. Cipher instance создаётся один раз и переиспользуется с новым IV.

### RQ-004 Per-packet allocation в frame encode/decode

`Frame.encode()` и `ByteArray.toFrame()` НЕ ДОЛЖНЫ аллоцировать промежуточные `ByteBuffer`/`ByteArray` сверх единственного результирующего массива (payload для `toFrame()`, wire bytes для `encode()`). Использовать direct buffer pool или zero-copy для промежуточных структур.

### RQ-005 Traffic counters — batch update

`onTrafficUpdate` НЕ ДОЛЖЕН вызываться на каждый пакет. AtomicLong счётчики обновляются, но колбэк триггерится не чаще 1 раза в 100ms.

### RQ-006 No per-packet logging в release

`AppLogger.i("TUN", "fwd ...")` не выполняется в release-сборке. Логи только для debug.

## Вне scope

- Рефакторинг архитектуры (MVVM, DI)
- Изменения Go server-side
- QUIC или relay transport
- Web UI
- Unit-тесты покрытия (но trace-маркеры `@sk-test` необходимы)

## Критерии приемки

### AC-001 Battery exemption не дёргается при reconnect

- Почему важно: пользователь не видит системный диалог при каждом подключении.
- **Given** пользователь подключился к VPN и battery exemption уже предоставлена (или отклонена)
- **When** происходит disconnect + повторный connect (без переустановки приложения)
- **Then** `Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` НЕ вызывается
- Evidence: `adb logcat | grep -i battery` не содержит `ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` после recycle

### AC-002 TUN reader не аллоцирует ByteArray на каждую итерацию

- Почему важно: снижение GC pressure и jitter на hot path.
- **Given** VpnService запущен и туннель установлен
- **When** через TUN проходит 1000 пакетов
- **Then** количество аллокаций `ByteArray(config.mtu)` в `tunReader()` равно 1 (или размеру пула), а не 1000
- Evidence: review кода — buffer pool или single pre-allocated buffer в цикле

### AC-003 Cipher.getInstance() не вызывается на пакет

- Почему важно: JCE provider lookup — тяжёлая операция; влияет на latency каждого пакета.
- **Given** cryptoEnabled=true и соединение установлено
- **When** encrypt или decrypt вызывается для пакета
- **Then** `Cipher.getInstance()` вызван не более 2 раз за сессию (один encrypt + один decrypt)
- Evidence: review кода — Cipher instance создаётся один раз, переиспользуется с `cipher.init()`

### AC-004 Frame encode/decode без per-call аллокаций

- Почему важно: снижение allocation rate на send/receive path.
- **Given** фрейм отправляется или принимается
- **When** `encode()` или `toFrame()` выполняется
- **Then** нет аллокации `ByteBuffer` или `ByteArray` внутри метода (кроме результирующего массива)
- Evidence: review кода + allocation profiling

### AC-005 Traffic counters батчатся

- Почему важно: StateFlow propagation в UI thread не на каждый пакет.
- **Given** туннель активен и трафик идёт
- **When** пакеты проходят через `tunReader()` и `handleFrame()`
- **Then** `onTrafficUpdate` вызывается не чаще 1 раза в 100ms
- Evidence: review кода — debounce/coroutine batcher

### AC-006 Per-packet логи только в debug

- Почему важно: `AppLogger.i()` в release — лишний I/O и конкатенация строк.
- **Given** release-сборка (BuildConfig.DEBUG=false)
- **When** пакет проходит через `tunReader()`
- **Then** `AppLogger.i("TUN", ...)` не выполняется
- Evidence: review кода — проверка `BuildConfig.DEBUG` или `if (logLevel == "debug")`

## Допущения

- Battery exemption уже предоставлена (или отклонена) при первом запуске — spec не требует автоматического granting.
- Размер пула буферов = 2 (один для чтения, один для processing) — достаточно для single-thread TUN reader.
- `Cipher` thread-safety не требуется — используются отдельные instance для encrypt и decrypt (два вызова `Cipher.getInstance()` за сессию, по одному на encrypt и decrypt).
- Release-сборка собирается с `BuildConfig.DEBUG = false` (стандартное поведение Gradle).

## Критерии успеха

### SC-001 Allocation rate на hot path снижен на 80%+

- Измерение: allocation profiling (Android Studio Profiler) до/после — аллокации в `tunReader()` + `FrameCodec` + `AesGcmCipher` должны снизиться с ~5+ аллокаций/пакет до ≤1.

### SC-002 Battery settings dialog не появляется после первого connect

- Измерение: ручное тестирование — disconnect → connect × 5 без перезапуска приложения. Ни одного системного диалога.

## Краевые случаи

- **Battery exemption не предоставлена**: пользователь впервые открыл приложение — диалог показывается один раз из UI (SettingsScreen), но не из service lifecycle.
- **Cipher пересоздание при reconnect**: при новом connect с новым session key — `Cipher.getInstance()` может быть вызван снова (с новым ключом). Это ок.
- **Buffer pool race**: TUN reader работает в одном coroutine — race невозможен.
- **LogLevel switch с runtime**: если пользователь в рантайме переключил logLevel — per-packet логи контролируются конфигом, а не только `BuildConfig.DEBUG`.

## Открытые вопросы

- Стоит ли добавить `BatteryManager.EXTRA_BATTERY_EXEMPTION` для API 35+? — `none` пока, отложить.
- Использовать `ObjectPool<ByteArray>` от сторонней библиотеки или самописный? — самописный pool в рамках этого spec.
