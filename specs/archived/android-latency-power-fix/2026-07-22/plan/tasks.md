# Android: оптимизация latency hot path и устранение battery settings popup при connect — Задачи

## Phase Contract

Inputs: plan (6 DEC, MVP=AC-001+AC-002), data-model (no-change), files из spec scope.
Outputs: упорядоченные задачи с Touches и AC coverage.
Stop if: задачи расплывчаты — не применимо.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/android/.../vpn/KvnVpnService.kt` | T1.1, T1.2, T2.1, T3.1, T3.2, T4.1 |
| `src/android/.../crypto/AesGcmCipher.kt` | T2.1 |
| `src/android/.../protocol/FrameCodec.kt` | T2.2 |
| `src/android/.../ui/MainViewModel.kt` | T1.1 |
| `src/android/.../ui/SettingsScreen.kt` | T1.1 |
| `src/android/.../transport/WebSocketClient.kt` | T2.2 (косвенно — send path) |

## Implementation Context

- **Цель MVP:** battery exemption не дёргается при connect + TUN reader без per-iteration аллокации ByteArray
- **Инварианты:**
  - `Cipher` thread-safety не нужен — два instance (encrypt/decrypt), вызываются из разных thread
  - TUN reader single-thread — buffer pool race невозможен
  - `Frame.encode()` — результирующий ByteArray неизбежен, убираем только промежуточный ByteBuffer
  - `toFrame()` — payload ByteArray неизбежен, убираем ByteBuffer.wrap
- **Границы scope:** не трогаем Go server-side, desktop/web UI, QUIC, архитектуру MVVM
- **Traceability:** `@sk-task` на всех изменённых объявлениях, `@sk-test` в тестах
- **Proof signals:** logcat не показывает battery dialog при reconnect; alloc profiling < 1 alloc/packet на hot path
- **DEC references:** DEC-001 (pre-alloc buffer), DEC-002 (Cipher cache), DEC-003/004 (frame zero-copy), DEC-005 (traffic debounce), DEC-006 (log guard)

## Фаза 1: MVP — Battery fix + Buffer pool

Цель: AC-001 + AC-002 — battery exemption не дёргается при reconnect, TUN reader без per-iteration аллокации.

- [x] T1.1 Вынести battery exemption из service lifecycle
  Убрать вызов `requestBatteryExemptionIfNeeded()` из `KvnVpnService.doStart()`. Добавить триггер в `SettingsScreen` (кнопка/автоматически при открытии SettingsScreen). Опционально: однократный запрос при первом connect через `MainViewModel` с флагом `batteryExemptionRequested`.
  Touches: `KvnVpnService.kt`, `SettingsScreen.kt`, `MainViewModel.kt`
  AC: AC-001

- [x] T1.2 Pre-allocated buffer для TUN reader
  Вынести `ByteArray(config.mtu)` за цикл `tunReader()` — один буфер, allocated 1 раз перед while. `data.copyOf(len)` остаётся (snapshot для отправки), но сам буфер не пересоздаётся.
  Touches: `KvnVpnService.kt` (tunReader)
  AC: AC-002
  DEC: DEC-001

## Фаза 2: Hot path — Cipher cache + Frame codec zero-copy

Цель: AC-003 + AC-004 — убрать Cipher.getInstance() из per-packet path и ByteBuffer аллокации в encode/decode.

- [x] T2.1 Кешировать Cipher instance (encrypt + decrypt)
  В `AesGcmCipher` добавить companion-фабрику или метод `reset()` для переиспользования instance с новым ключом при reconnect. Два instance: encrypt (tunReader), decrypt (handleFrame). `Cipher.getInstance()` вызывается не более 2 раз за сессию.
  Touches: `AesGcmCipher.kt`, `KvnVpnService.kt` (handleFrame — decrypt init)
  AC: AC-003
  DEC: DEC-002

- [x] T2.2 Zero-copy frame encode/decode
  `Frame.encode()`: заменить `ByteBuffer.allocate` на ручную запись в `ByteArray(4 + payload.size)` — type, flags, short length, затем `System.arraycopy(payload)`.
  `ByteArray.toFrame()`: читать type/flags/length напрямую из `this[0]`..`this[3]` без `ByteBuffer.wrap`. Payload `ByteArray(length)` остаётся (результат).
  Touches: `FrameCodec.kt`, `WebSocketClient.kt` (send — использует encode)
  AC: AC-004
  DEC: DEC-003, DEC-004

## Фаза 3: Traffic counters + Log guard

Цель: AC-005 + AC-006 — batch traffic updates, no per-packet log в release.

- [x] T3.1 Batch update для traffic counters
  `AtomicLong` счётчики обновляются на каждый пакет. `onTrafficUpdate` колбэк дёргается через debounce: coroutine с `delay(100)` или `channel` + ticker. Колбэк не чаще 1 раза в 100ms.
  Touches: `KvnVpnService.kt` (tunReader — tx update, handleFrame — rx update)
  AC: AC-005
  DEC: DEC-005

- [x] T3.2 Per-packet log guard
  `AppLogger.i("TUN", ...)` в `tunReader()` и `routePacket()` обернуть в `if (BuildConfig.DEBUG)` или `if (logLevel == "debug")`. В release-сборке код мёртвый.
  Touches: `KvnVpnService.kt` (tunReader, routePacket)
  AC: AC-006
  DEC: DEC-006

## Фаза 4: Проверка

Цель: доказать, что все AC работают, и оставить пакет reviewable.

- [x] T4.1 Проверить AC coverage
  - Android Studio Profiler: allocation recording до/после — снижение с ~5+ alloc/packet до ≤1
  - adb logcat: battery dialog не появляется при disconnect → connect ×5
  - adb logcat на release APK: нет `TUN` per-packet логов
  - Code review: Cipher.getInstance() call count = 2, FrameCodec без ByteBuffer, traffic debounce работает
  Touches: все файлы из Surface Map
  AC: AC-001 — AC-006, SC-001, SC-002

## Покрытие критериев приемки

- AC-001 -> T1.1, T4.1
- AC-002 -> T1.2, T4.1
- AC-003 -> T2.1, T4.1
- AC-004 -> T2.2, T4.1
- AC-005 -> T3.1, T4.1
- AC-006 -> T3.2, T4.1
- SC-001 -> T4.1
- SC-002 -> T4.1

## Заметки

- T1.1 и T3.2 независимы — можно параллелить
- T1.2 и T2.2 независимы — можно параллелить
- T2.1 зависит от AesGcmCipher интерфейса (добавить reset method)
- T4.1 — последняя, запускается после всех реализаций
- `@sk-task` trace-маркеры добавлять на каждое изменяемое объявление; `@sk-test` на тесты
