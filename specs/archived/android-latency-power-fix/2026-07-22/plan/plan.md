# Android: оптимизация latency hot path и устранение battery settings popup при connect — План

## Phase Contract

Inputs: spec + inspect (pass) + repo surfaces (KvnVpnService, AesGcmCipher, FrameCodec, WebSocketClient, MainViewModel, SettingsScreen).
Outputs: plan, data-model stub.
Stop if: spec расплывчата — не применимо, spec чёткая.

## Цель

6 независимых изменений в Android-модуле: вынос battery exemption из service lifecycle (AC-001), buffer pool для TUN read (AC-002), кеширование Cipher.getInstance (AC-003), zero-copy frame codec (AC-004), batch traffic counters (AC-005), no per-packet log в release (AC-006). Изменения не затрагивают Go/desktop/web.

## MVP Slice

**MVP = AC-001 + AC-002** (battery fix + buffer pool):
- battery: `KvnVpnService.doStart()` → убрать `requestBatteryExemptionIfNeeded()`, перенести в `SettingsScreen` / `MainViewModel`
- buffer pool: `tunReader()` — заменить `ByteArray(config.mtu)` в цикле на pre-allocated buffer

Покрывает AC-001, AC-002. Валидация: ручной connect/disconnect cycle + review кода.

## First Validation Path

1. Собрать APK: `./gradlew assembleDebug`
2. Установить, подключиться, отключиться, подключиться снова — проверить `adb logcat | grep -i battery` — нет ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS
3. Android Studio Profiler: allocation recording в `tunReader()` — 1 alloc на 1000 пакетов (было 1000)

## Scope

- Battery exemption: `KvnVpnService.kt` (удалить вызов), `SettingsScreen.kt` или `MainViewModel.kt` (добавить UI-triggered запрос)
- Buffer pool: `KvnVpnService.kt` (tunReader loop)
- Cipher cache: `AesGcmCipher.kt` (кешировать instance), `KvnVpnService.kt` (инициализация)
- Frame codec: `FrameCodec.kt` (encode — прямой ByteArray без ByteBuffer)
- Traffic batcher: `KvnVpnService.kt` (debounce в onTrafficUpdate)
- Per-packet log guard: `KvnVpnService.kt` (BuildConfig.DEBUG guard)

Границы: Go server-side, desktop/web UI, QUIC transport, архитектура MVVM не меняются.

## Performance Budget

- SC-001: аллокации `≤1 alloc/packet` на горячем пути (было ~5+). Проверка: Android Studio Profiler.
- SC-002: battery dialog не показывается после первого connect.
- P99 latency: не ухудшается (ожидаемое улучшение — убрать Cipher.getInstance из hot path).

## Implementation Surfaces

| Surface | Файл | Что меняется |
|---|---|---|
| VpnService main | `KvnVpnService.kt` | doStart (убрать battery), tunReader (buffer pool), handleFrame (traffic batcher), per-packet log guard |
| Cipher | `AesGcmCipher.kt` | cache Cipher instance, expose reset(newKey) для reconnect |
| Frame Codec | `FrameCodec.kt` | encode — писать прямо в ByteArray без ByteBuffer; toFrame — без ByteBuffer.wrap |
| WebSocket | `WebSocketClient.kt` | send — убрать Buffer (уже ok, только encode + ByteString) |
| ViewModel | `MainViewModel.kt` | battery exemption trigger (опционально) |
| Settings UI | `SettingsScreen.kt` | кнопка "Request battery exemption" |

## Bootstrapping Surfaces

- `none` — все изменения in-place, новые файлы не требуются.

## Влияние на архитектуру

- Локальное: все изменения в рамках одного Android-модуля.
- `AesGcmCipher` получает метод `init()` / `reset()` вместо создания нового instance при reconnect.
- `FrameCodec.encode()` теряет `require(payload.size <= FRAME_MAX_PAYLOAD)` — валидация переносится на caller (или остаётся, если копнуть глубже: размер известен до encode, проверить можно без ByteBuffer).
- `onTrafficUpdate` колбэк остаётся, но обёрнут debounce внутри VpnService — external API не меняется.
- Data model (`ConnectionConfig`, `AppConfig`) не меняется — `data-model.md: no-change`.

## Acceptance Approach

| AC | Approach | Surfaces | Validation |
|---|---|---|---|
| AC-001 | Убрать `requestBatteryExemptionIfNeeded()` из `doStart()`. Добавить в SettingsScreen или MainViewModel однократный вызов. | KvnVpnService, SettingsScreen | logcat + manual connect cycle |
| AC-002 | Буфер `ByteArray(mtu)` вынести за цикл, переиспользовать. | KvnVpnService.tunReader | Code review + alloc profiling |
| AC-003 | Два кешированных Cipher instance (encrypt/decrypt), `init()` с новым ключом при reconnect. | AesGcmCipher, KvnVpnService | Code review + Cipher.getInstance() call count |
| AC-004 | `encode()`: без `ByteBuffer.allocate`, писать в pre-alloc ByteArray. `toFrame()`: без `ByteBuffer.wrap`, читать напрямую. | FrameCodec | Code review + alloc profiling |
| AC-005 | Coroutine с debounce 100ms для `onTrafficUpdate`. AtomicLong счётчики без колбэка на каждый пакет. | KvnVpnService | Code review |
| AC-006 | `if (!BuildConfig.DEBUG) return` guard перед per-packet логами. | KvnVpnService.tunReader | Code review |

## Данные и контракты

- Data model не меняется. `data-model.md` — stub `no-change`.
- API/event contracts не меняются.

## Стратегия реализации

### DEC-001 Pre-allocated buffer вместо pool

- **Why:** TUN reader single-thread, нет конкуренции. Пул из 2 буферов (чтение + processing) — избыточен. Один буфер, allocated 1 раз, переиспользуется. `data.copyOf(len)` для отправки сохраняется (нужен snapshot), но сам буфер не пересоздаётся.
- **Tradeoff:** `copyOf` остаётся — без него нельзя, т.к. буфер перезаписывается на следующей итерации. Экономия: -1 alloc/packet (убираем `ByteArray(mtu)`).
- **Affects:** `KvnVpnService.tunReader()`
- **Validation:** alloc profiling — одна аллокация `ByteArray(mtu)` за сессию вместо N.

### DEC-002 Cipher instance cache в AesGcmCipher companion/factory

- **Why:** `Cipher.getInstance("AES/GCM/NoPadding")` — class lookup каждый раз. Один instance на encrypt, один на decrypt. При reconnect — `init()` с новым ключом (SecretKeySpec + GCMParameterSpec).
- **Tradeoff:** `Cipher` не thread-safe. Два instance (encrypt/decrypt) решают проблему — encrypt из tunReader (Dispatchers.IO), decrypt из OkHttp thread.
- **Affects:** `AesGcmCipher`, `KvnVpnService`
- **Validation:** `Cipher.getInstance()` call count = 2 за сессию (проверить debug log или breakpoint).

### DEC-003 Frame encode в pre-alloc заголовок + System.arraycopy

- **Why:** `ByteBuffer.allocate(4 + payload.size)` → `buf.put()` → `buf.array()` — два лишних объекта (ByteBuffer + array copy). Вместо: `ByteArray(4 + payload.size)`, заполнить header вручную, `System.arraycopy(payload, 0, result, 4, payload.size)`.
- **Tradeoff:** Код менее декларативный. Но выигрыш: -1 alloc/packet на send path.
- **Affects:** `FrameCodec.encode()`
- **Validation:** alloc profiling — нет `ByteBuffer` аллокаций в encode.

### DEC-004 toFrame без ByteBuffer.wrap

- **Why:** `ByteBuffer.wrap(this)` создаёт ByteBuffer object. Читать type/flags/length напрямую из `this` (byte array): `type = this[0]`, `flags = this[1]`, `length = ((this[2].toInt() and 0xFF) shl 8) or (this[3].toInt() and 0xFF)`. Payload `ByteArray(length)` остаётся (результат decode).
- **Tradeoff:** Чуть более низкоуровневый код. Экономия: -1 alloc/packet на receive path.
- **Affects:** `FrameCodec.toFrame()`
- **Validation:** alloc profiling — нет `ByteBuffer` аллокаций в toFrame.

### DEC-005 Traffic debounce через channel + ticker

- **Why:** `onTrafficUpdate` не должен дёргать StateFlow на каждый пакет. AtomicLong счётчики обновляются всегда, но колбэк через `launch { delay(100); emit() }`.
- **Tradeoff:** Потеря точности UI-обновлений (100ms лаг). Приемлемо для traffic display.
- **Affects:** `KvnVpnService` — `onTrafficUpdate` вызовы оборачиваются в debounce.
- **Validation:** UI обновляется не чаще 10 раз/сек.

### DEC-006 Per-packet log guard через BuildConfig.DEBUG

- **Why:** `AppLogger.i("TUN", ...)` в release — конкатенация строк и I/O. `if (BuildConfig.DEBUG)` делает код мёртвым в release (ProGuard может выкинуть).
- **Tradeoff:** Per-packet логи недоступны в release-сборках даже при проблемах. Но diagnostic можно получить через debug-сборку / adb logcat с logLevel.
- **Affects:** `KvnVpnService.tunReader()`, `routePacket()`
- **Validation:** `adb logcat | grep TUN` пуст на release APK.

## Incremental Delivery

### MVP (AC-001 + AC-002)

1. Убрать `requestBatteryExemptionIfNeeded()` из `doStart()`, перенести в SettingsScreen
2. Buffer pool: вынести `ByteArray(mtu)` за цикл tunReader

### Итеративное расширение

- **Раунд 2 (AC-003)**: Cipher cache — 2 instance, `init()` при reconnect
- **Раунд 3 (AC-004)**: Frame codec zero-copy (encode + toFrame)
- **Раунд 4 (AC-005)**: Traffic debounce
- **Раунд 5 (AC-006)**: Per-packet log guard (тривиально, можно в любом порядке)

## Порядок реализации

1. AC-001 (battery) — можно параллельно с AC-006 (log guard), независимы
2. AC-002 (buffer pool) + AC-004 (frame codec) — касаются разных файлов, независимы
3. AC-003 (cipher cache) — после AC-002 (оба тюнинг hot path)
4. AC-005 (traffic batcher) — после AC-002/AC-004
5. AC-006 — в любой момент

Параллельно: AC-001, AC-006 — стартуют первыми. Затем AC-002 + AC-004. Затем AC-003, затем AC-005.

## Риски

- **Риск 1: Cipher.init() c новым IV для каждого пакета — возможен performance hit.**
  Mitigation: `Cipher.init()` легче `Cipher.getInstance()`. Если `init()` окажется дорогим — обернуть в benchmark перед merge.

- **Риск 2: Buffer pool race при reconnect.**
  Mitigation: TUN reader запускается в новом coroutine после `establishTun()`, старый отменяется через `serviceScope.cancel()`. Race невозможен — один активный reader.

- **Риск 3: Release-сборка не показывает диагностику.**
  Mitigation: diagnostic через Debug-сборку + logLevel конфиг. AC-006 не прячет ошибки уровня выше per-packet.

## Rollout и compatibility

- Обратная совместимость: все изменения in-place, data model не меняется.
- Feature flag не требуется — изменения не меняют внешнее поведение (кроме battery dialog, который становится однократным).
- Monitoring: не требуется.

## Проверка

| Что | Как | AC/DEC |
|---|---|---|
| Battery dialog при reconnect | adb logcat + manual | AC-001 |
| Аллокации в tunReader | Android Studio Profiler — allocation recording | AC-002, DEC-001 |
| Cipher.getInstance call count | Debug log в AesGcmCipher | AC-003, DEC-002 |
| Frame encode/decode allocs | Profiler | AC-004, DEC-003, DEC-004 |
| Traffic update rate | UI observer — не чаще 10Hz | AC-005, DEC-005 |
| Release logcat пустой | adb logcat на release APK | AC-006, DEC-006 |
| SC-001 | Profiler before/after | SC-001 |
| SC-002 | Manual connect ×5 | SC-002 |

## Соответствие конституции

- нет конфликтов
- Traceability: все изменяемые объявления получат `@sk-task` + `@sk-test` в тестах
- Язык: doc на русском (spec), comments в коде на английском
- Ветка: `feature/android-latency-power-fix` уже создана
