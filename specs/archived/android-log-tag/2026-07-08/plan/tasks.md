# Android Logging System — Задачи

## Phase Contract

Inputs: `plan.md`, `data-model.md`, `spec.md`.
Outputs: исполнимые задачи с Touches и покрытием AC.
Stop if: AC без покрытия — все 13 AC покрыты.

## Surface Map

| Surface | Тип | Tasks |
|---|---|---|
| `src/.../logger/LogEntry.kt` | NEW | T1.1 |
| `src/.../logger/AppLogger.kt` | NEW | T1.2, T4.2 |
| `src/.../logger/LogViewerScreen.kt` | NEW | T2.1, T3.3, T3.4, T3.5, T4.1, T4.3, T4.4, T4.5 |
| `src/.../ui/MainActivity.kt` | MODIFY | T2.2 |
| `src/.../ui/SettingsScreen.kt` | MODIFY | T2.4 |
| `src/.../dns/FakeDnsResolver.kt` | MODIFY | T2.3 |
| `src/.../vpn/KvnVpnService.kt` | MODIFY | T3.1 |
| `src/.../ui/QrScannerScreen.kt` | MODIFY | T3.2 |
| `src/.../dns/LogBuffer.kt` | KEEP | — |
| `src/android/.../test/.../logger/AppLoggerTest.kt` | NEW | T2.5, T5.1 |

## Implementation Context

- **Цель MVP:** AppLogger + LogEntry + LogViewerScreen (live лента, auto-scroll) + 4-й таб + FakeDnsResolver мигрирован
- **Инварианты/семантика:**
  - LogLevel: DEBUG(0) < INFO(1) < WARN(2) < ERROR(3) — enum с ординалами
  - LogEntry — data class, не изменяется после создания (immutable)
  - AppLogger — object (глобальный синглтон), как текущий LogBuffer
  - Ring buffer: ArrayDeque, @Synchronized на write/clear, maxSize configurable (default 2000)
  - SharedFlow: `val logFlow: SharedFlow<LogEntry>` — `MutableSharedFlow(replay = 0, extraBufferCapacity = 64)`
  - Thread-safe: все публичные методы AppLogger @Synchronized
- **Ошибки/коды:**
  - Export: try/catch → Snackbar с сообщением
  - Буфер не бросает исключения при overflow — oldest entry silently evicted
- **Контракты/протокол:**
  - Export file: `kvn-logs-YYYY-MM-dd-HHmmss.txt`
  - Export path: API≥29 → MediaStore.Downloads; API<29 → `context.getExternalFilesDir(DIRECTORY_DOWNLOADS)`
  - Share: `Intent.ACTION_SEND` с `text/plain`
- **Границы scope:**
  - Не читаем system Logcat
  - Не сохраняем логи на диск между сессиями
  - Не добавляем external библиотеки (Timber, Logcat и т.д.)
  - Не трогаем Go-сервер, Web UI, LogBuffer.kt
- **Proof signals:**
  - AC-011 (export error): при отзыве storage permission → Snackbar
  - AC-006 (copy): после long-press → clipboard + Snackbar
  - AC-012 (migration): grep не показывает старые импорты в потребителях
- **References:** `DEC-001`–`DEC-007`, `DM-001`–`DM-003`

## Фаза 1: Основа — Core Logger

Цель: создать ядро системы логирования: модель LogEntry, логгер AppLogger с буфером и SharedFlow.

- [x] **T1.1 Создать LogEntry + LogLevel модели**
  Создать `LogEntry.kt` с data class `LogEntry` и enum `LogLevel`. Все поля согласно `DM-001`, `DM-002`.
  Touches: `src/android/app/src/main/kotlin/com/kvn/client/logger/LogEntry.kt`

- [x] **T1.2 Создать AppLogger синглтон**
  Создать `AppLogger.kt` — object с методами `d(tag, msg)`, `i(tag, msg)`, `w(tag, msg)`, `e(tag, msg)`.
  Внутренний ring buffer (ArrayDeque, maxSize=2000 по умолчанию, конфигурируется через init).
  `SharedFlow<LogEntry>` для подписки UI. Метод `clear()`. Метод `snapshot(): List<LogEntry>` для текущего состояния буфера.
  Все публичные методы — `@Synchronized` (thread safety, `AC-013`).
  Touches: `src/android/app/src/main/kotlin/com/kvn/client/logger/AppLogger.kt`

## Фаза 2: MVP Slice

Цель: живой log viewer + первый потребитель + 4-й таб. Минимально демонстрируемая ценность.

- [x] **T2.1 Создать LogViewerScreen с живой лентой и auto-scroll**
  Compose экран: `@Composable fun LogViewerScreen()`. Подписка на `AppLogger.logFlow` через `collectAsState()`. `LazyColumn` с auto-scroll (LazyListState + animateScrollToItem при новых записях, если пользователь внизу списка). Monospace шрифт, чередование фона строк для читаемости. Формат: `[HH:mm:ss.SSS][LEVEL][TAG] message`.
  Touches: `src/android/app/src/main/kotlin/com/kvn/client/logger/LogViewerScreen.kt`

- [x] **T2.2 Добавить 4-й таб Logs в Bottom Navigation**
  В `MainActivity.kt` добавить 4-й `NavigationBarItem` с иконкой (напр. `Icons.Default.List` или `Icons.Default.Description`) и текстом "Logs". Индекс 2 между Settings (1) и Traffic (3). При selectedTab == 2 показывать `LogViewerScreen()`.
  Touches: `src/android/app/src/main/kotlin/com/kvn/client/ui/MainActivity.kt`

- [x] **T2.3 Мигрировать FakeDnsResolver на AppLogger**
  Заменить 11 вызовов `LogBuffer.log("DNS", msg)` на `AppLogger.i("DNS", msg)` (все существующие логи — informational).
  Удалить `import com.kvn.client.dns.LogBuffer`.
  Touches: `src/android/app/src/main/kotlin/com/kvn/client/dns/FakeDnsResolver.kt`

- [x] **T2.4 Удалить старый AlertDialog Log Viewer из SettingsScreen**
  Удалить блок кода "Routing Logs" (AlertDialog + associated state + кнопка OutlinedButton). Удалить `import com.kvn.client.dns.LogBuffer`.
  Touches: `src/android/app/src/main/kotlin/com/kvn/client/ui/SettingsScreen.kt`

- [x] **T2.5 Валидация MVP: AppLogger thread-safety + flow тест**
  Создать `AppLoggerTest.kt`. Unit-тесты:
  1. Однопоточная запись 2000 записей — буфер содержит 2000, overflow вытесняет старые
  2. 10 потоков × 1000 записей — ни одного исключения, total == 10000 (или maxSize при overflow)
  3. SharedFlow доставляет все записанные записи подписчику
  4. clear() очищает буфер, snapshot пуст
  Touches: `src/android/app/src/test/java/com/kvn/client/logger/AppLoggerTest.kt`

## Фаза 3: Основная реализация — Full Migration + Filters

Цель: мигрировать оставшихся потребителей, добавить фильтрацию и поиск.

- [x] **T3.1 Мигрировать KvnVpnService на AppLogger**
- [x] **T3.2 Мигрировать QrScannerScreen на AppLogger**
- [x] **T3.3 Реализовать фильтрацию по уровню и тегу**
- [x] **T3.4 Реализовать текстовый поиск с highlight**
- [x] **T3.5 Реализовать pause/resume auto-scroll**
  Auto-scroll приостанавливается когда пользователь скроллит вверх (проверка `layoutInfo.visibleItemsInfo.last().index < layoutInfo.totalItemsCount - 1`). FAB-кнопка с иконкой "↓" появляется при паузе, по нажатию — scroll to last item + возобновление auto-scroll. При паузе новые записи добавляются в список, но позиция не сдвигается.
  Touches: `src/android/app/src/main/kotlin/com/kvn/client/logger/LogViewerScreen.kt`

## Фаза 4: Полировка — Copy / Export / Share

Цель: добавить копирование, экспорт и share.

- [x] **T4.1 Реализовать copy single entry**
- [x] **T4.2 Реализовать clear buffer**
- [x] **T4.3 Реализовать export to file**
- [x] **T4.4 Реализовать share**
- [x] **T4.5 Реализовать empty state и export error handling**
  Empty state: когда `filteredEntries.isEmpty()` — показывать `Column` с иконкой, заголовком "No logs", текстом "Use the app to generate log entries." Export error: try/catch → Snackbar "Export failed: {message}". Share disabled при пустом буфере (кнопка greyed out или Snackbar).
  Touches: `src/android/app/src/main/kotlin/com/kvn/client/logger/LogViewerScreen.kt`

## Фаза 5: Проверка

Цель: automated coverage + manual validation.

- [x] **T5.1 Unit-тесты для фильтрации и AppLogger edge cases**
  Добавить в `AppLoggerTest.kt`:
  - Фильтрация snapshot() по уровню возвращает только записи нужного уровня
  - Фильтрация snapshot() по комбинации level + tag
  - Буфер при максимальной ёмкости вытесняет старые записи (FIFO)
  - Пустой буфер — snapshot пуст, clear не бросает исключение
  - `@sk-test` маркеры на каждом тестовом методе
  Touches: `src/android/app/src/test/java/com/kvn/client/logger/AppLoggerTest.kt`

- [x] **T5.2 Manual validation по Acceptance Approach из plan**
  Пройти по всем AC согласно `plan.md` Acceptance Approach таблице:
  - AC-001: открыть таб при активном VPN — живые логи
  - AC-002/003/004: фильтры/поиск
  - AC-005: pause/resume
  - AC-006: copy
  - AC-007: export → файл в Downloads
  - AC-008: share → share sheet
  - AC-009/010: clear + empty state
  - AC-011: export без storage permission
  - AC-012: grep `LogBuffer.log|android.util.Log` в потребителях = 0 matches (кроме LogBuffer.kt)
  Touches: (manual, no code changes)

## Покрытие критериев приемки

- AC-001 Live streaming → T1.2, T2.1, T2.5
- AC-002 Filter by level → T3.3, T5.1
- AC-003 Filter by tag → T3.3, T5.1
- AC-004 Text search → T3.4, T5.1
- AC-005 Pause/resume → T3.5
- AC-006 Copy single → T4.1, T5.2
- AC-007 Export → T4.3, T5.2
- AC-008 Share → T4.4, T5.2
- AC-009 Clear → T4.2, T5.1
- AC-010 Empty state → T4.5, T5.2
- AC-011 Export error → T4.5, T5.2
- AC-012 Consumers migrated → T2.3, T3.1, T3.2, T5.2
- AC-013 Thread safety → T1.2, T2.5
