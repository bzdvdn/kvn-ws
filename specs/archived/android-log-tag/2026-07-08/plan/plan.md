# Android Logging System — План

## Phase Contract

Inputs: `spec.md`, кодовая база Android (`src/android/`).
Outputs: `plan.md`, `data-model.md`.
Stop if: spec расплывчата — spec чёткая, план безопасен.

## Цель

Реализовать структурированный логгер `AppLogger` (пакет `com.kvn.client.logger`) с уровнями, in-memory буфером и `SharedFlow`, мигрировать всех существующих потребителей, и создать Compose Log Viewer как 4-й таб Bottom Navigation. Старый `LogBuffer` остаётся как deprecated wrapper. Никаких новых external библиотек.

## MVP Slice

Минимальный независимо демонстрируемый инкремент: `AppLogger` + LogEntry + LogViewerScreen с живой лентой и auto-scroll + миграция одного потребителя (FakeDnsResolver).

Покрываемые AC: AC-001 (live streaming), AC-012 (existing consumers migrated — частично), AC-013 (thread safety).

## First Validation Path

1. Запустить Android-приложение, подключиться к VPN
2. Переключиться на таб Logs — видна живая лента DNS/TUN/ROUTE логов с auto-scroll
3. Закрыть/открыть таб — лента продолжает пополняться

## Scope

- **Новое:** `com.kvn.client.logger` — LogEntry, LogLevel, AppLogger (SharedFlow + ring buffer)
- **Новое:** `LogViewerScreen.kt` — Compose UI для логов
- **Изменение:** `FakeDnsResolver.kt` — 11 LogBuffer.log → AppLogger
- **Изменение:** `KvnVpnService.kt` — 11 LogBuffer.log → AppLogger
- **Изменение:** `QrScannerScreen.kt` — 3 android.util.Log → AppLogger
- **Изменение:** `SettingsScreen.kt` — удалить старый AlertDialog лог-вьюер
- **Изменение:** `MainActivity.kt` — добавить 4-й таб (Logs) в Bottom Navigation
- **Не трогаем:** Go-сервер, Web UI, `LogBuffer.kt` (остаётся)

## Performance Budget

- Memory: буфер 2000 записей < 2 MB (усреднённая запись ~100 символов ≈ 200-300 bytes в памяти)
- UI: FPS > 55 при скролле с 2000 записей (LazyColumn виртуализация)
- `none` для остального

## Implementation Surfaces

| Surface | Тип | Зачем |
|---|---|---|
| `src/android/.../logger/AppLogger.kt` | Новая | Синглтон-логгер с уровнями, буфером, SharedFlow |
| `src/android/.../logger/LogEntry.kt` | Новая | Data model: level, tag, message, timestamp, thread |
| `src/android/.../logger/LogViewerScreen.kt` | Новая | Compose экран: живая лента, фильтры, поиск, copy/export/share |
| `src/android/.../dns/FakeDnsResolver.kt` | Изменение | 11 вызовов LogBuffer.log → AppLogger |
| `src/android/.../vpn/KvnVpnService.kt` | Изменение | 11 вызовов LogBuffer.log → AppLogger |
| `src/android/.../ui/QrScannerScreen.kt` | Изменение | 3 вызова android.util.Log → AppLogger |
| `src/android/.../ui/SettingsScreen.kt` | Изменение | Удалить старый AlertDialog (Routing Logs) |
| `src/android/.../ui/MainActivity.kt` | Изменение | Добавить 4-й таб Logs |

## Bootstrapping Surfaces

Новый пакет `src/android/app/src/main/kotlin/com/kvn/client/logger/` должен быть создан первым — всё остальное зависит от него.

## Влияние на архитектуру

- Локальное: новый пакет `logger` для всего, что связано с логированием (выделено из `dns`).
- Миграция: все потребители переходят с `LogBuffer.log(tag, msg)` на `AppLogger.i(tag, msg)` и т.д. Старый `LogBuffer` остаётся нетронутым (deprecated wrapper).
- Навигация: 3 таба → 4 таба, индекс сдвигается (Traffic был index 2 → становится index 3).

## Acceptance Approach

| AC | Подход | Surfaces | Наблюдение |
|---|---|---|---|
| AC-001 Live streaming | AppLogger публикует SharedFlow, LogViewerScreen подписывается через collectAsState, LazyColumn с auto-scroll | AppLogger, LogViewerScreen | Открыть таб Logs при активном VPN — строки появляются без ручного обновления |
| AC-002 Filter by level | FilterChips для уровней, фильтрация списка на стороне UI | LogViewerScreen | Выбрать WARN+ERROR — строки DEBUG/INFO исчезают |
| AC-003 Filter by tag | Multi-select из активных тегов, полученных из буфера | LogViewerScreen | Выбрать TUN — строки с другими тегами скрыты |
| AC-004 Text search | TextField + debounce, фильтрация + highlight | LogViewerScreen | Ввести "timeout" — отображаются только matching строки с подсветкой |
| AC-005 Pause/resume | LazyListState + detect scroll to top/bottom, кнопка FAB | LogViewerScreen | Скролл вверх → пауза; кнопка "↓" → resume |
| AC-006 Copy single | Long-press → ClipboardManager + Snackbar | LogViewerScreen | После long-press вставить в другое приложение — текст записи |
| AC-007 Export | Кнопка → сохранение в Downloads через getExternalFilesDir / MediaStore.Downloads | LogViewerScreen | Файл в Downloads, открывается текстовым редактором |
| AC-008 Share | Кнопка → Intent.ACTION_SEND с текстом логов | LogViewerScreen | Share sheet открывается с текстом |
| AC-009 Clear | Кнопка → AppLogger.clear() → empty state | AppLogger, LogViewerScreen | После Clear — "No logs yet" |
| AC-010 Empty state | if (filteredList.isEmpty()) → empty state composable | LogViewerScreen | Открыть таб без активности — empty state |
| AC-011 Export error | try/catch + Snackbar | LogViewerScreen | Отозвать storage permission → Export → Snackbar |
| AC-012 Consumers migrated | Замена всех LogBuffer.log + android.util.Log на AppLogger | FakeDnsResolver, KvnVpnService, QrScannerScreen | grep не показывает старые импорты |
| AC-013 Thread safety | @Synchronized на write + ConcurrentLinkedDeque-like структура | AppLogger | Unit test: 10 threads × 1000 записей |

## Данные и контракты

- `data-model.md` обязателен: новая модель LogEntry + LogLevel
- Contracts: нет внешних API/event контрактов (pure internal)

## Стратегия реализации

### DEC-001 Новый пакет `com.kvn.client.logger`

- **Why:** логи не относятся к DNS; отдельный пакет чище архитектурно и следует принципу единственной ответственности
- **Tradeoff:** нужно обновить импорты во всех потребителях (но это механическая замена)
- **Affects:** новый пакет + все файлы-потребители
- **Validation:** grep не показывает `import com.kvn.client.dns.LogBuffer` в потребителях (кроме LogBuffer.kt)

### DEC-002 LogLevel enum vs boolean flags

- **Why:** sealed enum даёт type safety, when-исчерпываемость, просто маппится в UI цвет/иконку
- **Tradeoff:** нужно маппить уровень в цвет для UI (но это тривиально)
- **Affects:** LogEntry.kt, AppLogger.kt, LogViewerScreen.kt
- **Validation:** `AppLogger.i("tag", "msg")` создаёт LogEntry с level=INFO

### DEC-003 SharedFlow for live streaming vs callback

- **Why:** coroutines-native, Compose собирает через `collectAsState()`, в проекте уже есть kotlinx-coroutines
- **Tradeoff:** требует активного CoroutineScope (жизненный цикл таба)
- **Affects:** AppLogger.kt, LogViewerScreen.kt
- **Validation:** LogViewerScreen видит новые записи без polling

### DEC-004 ArrayDeque ring buffer vs circular array

- **Why:** ArrayDeque уже используется в LogBuffer, O(1) add/removeFirst, просто
- **Tradeoff:** @Synchronized может быть bottleneck при > 1000 записей/сек, но для Android-клиента это нереально
- **Affects:** AppLogger.kt
- **Validation:** буфер не теряет записи при конкурентном доступе

### DEC-005 Tab-based navigation (selectedTab) vs Navigation Component

- **Why:** в MainActivity уже используется `selectedTab` паттерн; добавление NavComponent тянет лишние зависимости и меняет архитектуру без необходимости
- **Tradeoff:** нет back stack, deep link — но они и не нужны для Logs таба
- **Affects:** MainActivity.kt
- **Validation:** таб Logs работает как 4-й элемент Bottom Navigation

### DEC-006 LazyColumn with auto-scroll vs Column

- **Why:** 2000 записей требуют виртуализации; LazyColumn + LazyListState + animateScrollToItem
- **Tradeoff:** auto-scroll при появлении новых записей требует ручного управления (detect если пользователь внизу)
- **Affects:** LogViewerScreen.kt
- **Validation:** скролл работает при 2000 записей без лагов, auto-scroll возобновляется при скролле в самый низ

### DEC-007 Export: getExternalFilesDir (< API 29) + MediaStore.Downloads (API 29+)

- **Why:** minSdk=26, MediaStore.Downloads доступен с API 29; для 26-28 fallback на `getExternalFilesDir` (не требует permission)
- **Tradeoff:** два пути экспорта; файлы в app-specific external storage на старых API
- **Affects:** LogViewerScreen.kt
- **Validation:** экспорт работает на API 26 и API 35

## Incremental Delivery

### MVP (Первая ценность)

AppLogger + LogEntry + LogViewerScreen с живой лентой + миграция FakeDnsResolver + 4-й таб.

- **AC:** AC-001, AC-012 (partial), AC-013
- **Проверка:** открыть таб, увидеть DNS логи в реальном времени

### Итеративное расширение 1 — Полная миграция

Мигрировать KvnVpnService и QrScannerScreen, удалить старый AlertDialog из SettingsScreen.

- **AC:** AC-012 (full)
- **Проверка:** все логи (TUN, TCP, ROUTE, DNS, QR) видны в Logs табе; старый диалог недоступен

### Итеративное расширение 2 — Фильтрация и поиск

Filter by level, filter by tag, text search, pause/resume.

- **AC:** AC-002, AC-003, AC-004, AC-005
- **Проверка:** фильтры работают, поиск подсвечивает, пауза останавливает auto-scroll

### Итеративное расширение 3 — Copy/Export/Share

Copy single entry, clear, export to file, share.

- **AC:** AC-006, AC-007, AC-008, AC-009, AC-010, AC-011
- **Проверка:** copy → clipboard, export → файл в Downloads, share → share sheet

## Порядок реализации

1. **AppLogger + LogEntry** (ядро, без UI) — всё остальное зависит от этого
2. **LogViewerScreen (MVP)** — live лента с auto-scroll, без фильтров
3. **4-й таб в MainActivity** — сделать таб видимым
4. **Миграция FakeDnsResolver** — первый потребитель, сразу видны DNS логи
5. **Миграция KvnVpnService + QrScannerScreen** — остальные потребители
6. **Удаление старого AlertDialog** из SettingsScreen (после миграции всех потребителей)
7. **Фильтрация** (уровень, тег, поиск) + пауза — может быть параллельно с шагами 4-6
8. **Copy/Export/Share** — финальная полировка

Безопасно параллелить: 4+5 (миграция потребителей независима друг от друга), 7 может начаться после 2.

## Риски

- **Риск 1:** `@Synchronized` на всех методах AppLogger может стать bottleneck при высокой частоте логирования из VpnService threads
  - **Mitigation:** для Android-клиента частота < 100 записей/сек — @Synchronized более чем достаточно; если упрёмся — перейти на `ConcurrentLinkedDeque` + атомарный счётчик

- **Риск 2:** auto-scroll LazyColumn конфликтует с появлением новых записей — пользователь скроллит историю, новые записи сбрасывают позицию
  - **Mitigation:** auto-scroll только когда список в самом низу (проверка `layoutInfo.visibleItemsInfo.last().index == layoutInfo.totalItemsCount - 1`); при скролле вверх — пауза

- **Риск 3:** Export на Android 26-28 (нет MediaStore.Downloads) — файлы сохраняются в app-specific external dir, пользователь не видит их в стандартном Downloads без File manager
  - **Mitigation:** Share как основной способ выгрузки логов; Export показывать Snackbar с полным путём

## Rollout и compatibility

- Старый `LogBuffer` остаётся: ни один существующий workflow не ломается при откате
- 4-й таб не влияет на существующие 3 таба — бинарная совместимость
- Миграция потребителей — чистая замена импорта + вызова, никаких новых permission
- Export не требует runtime permission на API 29+ (MediaStore), на API 26-28 не требует (ExternalFilesDir)
- Специальных rollout действий не требуется

## Проверка

| Тип | Что проверяем | AC / DEC |
|---|---|---|
| Unit test | AppLogger: запись 10k логов из 10 потоков, проверка размера и целостности | AC-013, DEC-004 |
| Unit test | AppLogger: SharedFlow доставляет все записи подписчику | AC-001, DEC-003 |
| Unit test | AppLogger: buffer overflow вытесняет старые записи | DEC-004 |
| Unit test | LogEntry: корректное создание всех уровней | DEC-002 |
| Manual | Открыть таб Logs при активном VPN — живые логи | AC-001 |
| Manual | Фильтр по уровню, тегу, текст поиск | AC-002, AC-003, AC-004 |
| Manual | Pause/resume скролла | AC-005 |
| Manual | Copy single entry | AC-006 |
| Manual | Export + Share | AC-007, AC-008 |
| Manual | Clear + empty state | AC-009, AC-010 |
| Regression | VPN функционал не сломан (connect/disconnect, routing) | SC-003 |
| Regression | grep no old imports after migration | AC-012 |

## Соответствие конституции

- нет конфликтов — фича чисто Android (Kotlin/Compose), не затрагивает Go-ядро, не меняет архитектурные принципы (MVVM, Clean Architecture)
