# Multi-server + Dark Theme для Android-клиента — План

## Цель

Добавить в Android-клиент (kvn-android) управление несколькими серверами и тёмную тему. Data model расширяется с одного `ConnectionConfig` на `AppConfig { activeServer, servers: List<ServerEntry> }`. DataStore сохраняет единый JSON с массивом серверов. UI получает селектор серверов, Add/Duplicate/Delete, dirty-диалог подтверждения. Тема переключается на Material 3 dark с палитрой kvn-web.

## MVP Slice

AC-001 (CRUD) + AC-002 (add/switch) + AC-003 (dirty confirm) + AC-005 (dark theme) — можно добавить второй сервер через форму, переключиться, увидеть dirty-диалог, всё в тёмной теме.

## First Validation Path

Собрать APK (`./gradlew assembleDebug`), установить, открыть → тёмная тема, один сервер "Default" в селекторе. Добавить второй, заполнить поля, переключиться обратно — dirty-диалог. Нажать Connect — подключается к выбранному.

## Scope

- Data model: `AppConfig(activeServer, servers)` + `ServerEntry(name, config)` — новые типы
- DataStore: замена единственного `config_json` на структурированное хранение с миграцией
- ViewModel: управление списком серверов, dirty state, connect/disconnect с активным сервером
- UI: серверный селектор (ExposedDropdownMenu/Menu), кнопки Add/Duplicate/Delete, dirty-диалог
- QR: импорт → add, экспорт → активный сервер
- Тема: Material 3 dark color scheme в `material3.darkColorScheme()` с кастомными цветами
- Файлы: `AppConfig.kt`, `MainViewModel.kt`, `ConnectScreen.kt` (все три большие правки), `themes.xml` (новая тема), `Color.kt` (новая палитра), `QrExportScreen.kt`, `QrScannerScreen.kt`

**Нетронуто:** KvnVpnService, WebSocketClient, transport/, protocol/, crypto/ — multi-server прозрачен для них (connect получает `ConnectionConfig` активного сервера).

## Implementation Surfaces

| Surface | Роль | Тип |
|---------|------|-----|
| `config/AppConfig.kt` | Data model (`ServerEntry`, `AppConfig`), DataStore migration + loader/saver | existing, heavy modify |
| `ui/MainViewModel.kt` | Состояние: список серверов, активный, dirty, connect/disconnect | existing, heavy modify |
| `ui/ConnectScreen.kt` | UI: селектор, кнопки, dirty-диалог, QR импорт/экспорт | existing, heavy modify |
| `ui/Color.kt` (new) | Кастомные цвета kvn-web | new |
| `res/values/themes.xml` | Material 3 dark theme | existing, light modify |
| `ui/QrExportScreen.kt` | Экспорт QR для активного сервера | existing, minor modify |
| `ui/QrScannerScreen.kt` | Импорт QR → add new server + null-safe web JSON парсинг | existing, minor modify |
| `config/AppConfigTest.kt` | Unit-тесты миграции, serialization | existing, new tests |

## Bootstrapping Surfaces

- `ui/theme/Color.kt` — новая палитра цветов kvn-web. Нужна до изменения `themes.xml`.
- `config/AppConfig.kt` — новые типы `ServerEntry`, `AppConfig`. Нужны до изменения ViewModel.

## Влияние на архитектуру

- DataStore: единый JSON с массивом серверов вместо одного `ConnectionConfig`. Обратная совместимость: миграция читает старый `config_json`, оборачивает в `ServerEntry("Default", ...)`, сохраняет как новый формат.
- ViewModel: добавляется `servers`, `activeServerName`, `dirty` состояния. `savedConfig` заменяется на `savedAppConfig: StateFlow<AppConfig>`. QrConfig теперь добавляет сервер, а не заполняет форму.
- UI: все локальные `mutableStateOf` переезжают в per-server структуру. При смене сервера — dirty check.
- Connect: `connect()` вызывается с `activeServer.config`, а не с `buildConfig()`.

## Acceptance Approach

| AC | Подход | Surfaces | Валидация |
|----|--------|----------|-----------|
| AC-001 | Data model + DataStore + and migration | `AppConfig.kt` | Unit test: config with old format loads as ServerEntry("Default"); empty DataStore creates default ServerEntry |
| AC-002 | UI: server selector dropdown + form fill on switch | `ConnectScreen.kt`, `MainViewModel.kt` | Manual: add server → appears in dropdown → select → fields fill |
| AC-003 | ViewModel dirty flag + AlertDialog | `MainViewModel.kt`, `ConnectScreen.kt` | Manual: edit field → switch server → dialog appears |
| AC-004 | ViewModel.connect uses activeServer.config | `MainViewModel.kt` | Manual: switch → connect → verify server address in logs |
| AC-005 | Material 3 darkColorScheme with kvn-web palette | `Color.kt`, `themes.xml` | Visual: screenshot comparison with kvn-web colors |
| AC-006 | QR import → ViewModel.addServer("Imported ...") | `QrScannerScreen.kt`, `MainViewModel.kt` | Manual: scan QR → new server in dropdown, old one intact |
| AC-007 | QR export → ViewModel.activeServer.config | `QrExportScreen.kt`, `MainViewModel.kt` | Manual: switch to server B → export → QR contains B's config |
| AC-008 | UI: Duplicate button → copy all fields + "(copy)" | `ConnectScreen.kt`, `MainViewModel.kt` | Manual: duplicate → new "Work (copy)" with same fields, auto-switch |

## Фактические отклонения от плана

- QR парсер: добавлена поддержка null-полей в web JSON (`auto_reconnect: Boolean?`), так как kvn-web отправляет `"auto_reconnect": null`. В плане не учтено — исправлено в процессе.
- Кнопки CRUD: заменены с `OutlinedButton` + текст на `Button` + иконки с семантическими цветами (Add=зелёный, Copy=синий, Rename=оранжевый, Delete=красный). Улучшение UX, выходящее за рамки original spec.
- Добавлена зависимость `material-icons-extended` для иконки `ContentCopy`.

## Данные и контракты

- Data model расширяется в `AppConfig.kt`:
  ```kotlin
  @Serializable
  data class ServerEntry(
      val name: String,
      val config: ConnectionConfig
  )

  @Serializable
  data class AppConfig(
      val activeServer: String = "",
      val servers: List<ServerEntry> = emptyList()
  )
  ```
- DataStore: ключ `config_json` остаётся, но значение — JSON `AppConfig`, а не `ConnectionConfig`
- Миграция: если старый `config_json` парсится как `ConnectionConfig` (нет полей `activeServer`/`servers`) → обернуть в `ServerEntry("Default", oldConfig)` → сохранить как `AppConfig`
- QR: формат JSON не меняется (тот же `ConnectionConfig`/web JSON). Импорт → `addServer()`, экспорт → `activeServer.config`
- ViewModel: `savedConfig` → `savedAppConfig: StateFlow<AppConfig>`. `connect()` принимает имя сервера вместо полного конфига
- `data-model.md` прилагается

## Стратегия реализации

### DEC-001: Единый JSON с массивом серверов (вместо нескольких DataStore ключей)

- **Why**: минимальное изменение хранилища — один ключ `config_json`, только тип меняется. Миграция тривиальна (обёртка). Нет блокировок/race conditions при записи массива как атомарной операции.
- **Tradeoff**: весь список перезаписывается при любом изменении (add/delete/rename/save одного сервера). Для <50 серверов несущественно.
- **Affects**: `AppConfig.kt`
- **Validation**: unit test: save 3 servers → close → reopen → all 3 restored

### DEC-002: Dirty state в ViewModel (не в UI)

- **Why**: dirty-флаг должен переживать рекомпозицию и быть доступным при переключении серверов. ViewModel — естественное место.
- **Tradeoff**: ViewModel знает о UI-состоянии (dirty). Альтернатива (snapshot сравнение в UI) менее надёжна.
- **Affects**: `MainViewModel.kt`
- **Validation**: edit field → dirty=true → switch → dialog → discard → dirty=false

### DEC-003: Сортировка в ViewModel

- **Why**: правило (активный сверху, остальные A→Z) применяется независимо от UI и консистентно при любых изменениях.
- **Affects**: `MainViewModel.kt`
- **Validation**: servers ["C", "A", "B"] → active="A" → sorted list ["A", "B", "C"]

## Incremental Delivery

### MVP (Первая ценность)

- Data model + DataStore миграция + ViewModel с multi-server + UI: селектор + Add/Duplicate/Delete + dirty confirm + dark theme
- AC-001, AC-002, AC-003, AC-005
- Валидация: APK → первый запуск → "Default" создан → добавить сервер → переключить → dirty-диалог → тёмная тема

### Итеративное расширение

- QR multi-server (AC-006, AC-007): импорт → add, экспорт → активный
- Connect по активному серверу (AC-004): финальная привязка connect → выбранный сервер
- Duplicate (AC-008): кнопка дублирования

## Порядок реализации

1. **Data model + миграция** (AppConfig.kt): типы `ServerEntry`, `AppConfig`, обновлённые `load/save`. Независимый шаг, тестируется unit-тестами.
2. **Тёмная тема** (Color.kt, themes.xml): палитра kvn-web + darkColorScheme. Визуально проверяется сразу.
3. **ViewModel** (MainViewModel.kt): `activeServer`, `servers` state, dirty flag, сортировка, connect с активным сервером.
4. **UI: селектор + CRUD кнопки** (ConnectScreen.kt): server selector, Add, Duplicate, Delete, dirty-диалог.
5. **QR multi-server** (QrScannerScreen.kt, QrExportScreen.kt): импорт → add, экспорт → активный.

Шаги 1 и 2 независимы и могут параллелиться. Шаги 4-5 зависят от шага 3.

## Риски

- **Миграция существующих пользователей**: старый формат `ConnectionConfig` не имеет полей `activeServer`/`servers`. ❗ Решение: `ignoreUnknownKeys = true` уже включён. Попробуем парсить как `ConnectionConfig`; если нет полей AppConfig — оборачиваем.
  - Mitigation: unit test с реальным старым JSON.
- **DataStore race при быстрых add/delete/save**: DataStore Preferences атомарен на уровне `edit {}`, но частые записи могут конфликтовать.
  - Mitigation: операции модификации списка последовательные через viewModelScope.
- **Размер JSON при 10+ серверах**: каждый сервер содержит ConnectionConfig (~30 полей).
  - Mitigation: < 4KB JSON для 10 серверов — не проблема для DataStore Preferences.

## Rollout и compatibility

- Старый `config_json` автоматически мигрируется при первом запуске новой версии → нет потери данных.
- Если пользователь откатится на старую версию: старый код прочитает `config_json` (теперь AppConfig), не найдёт полей ConnectionConfig (ignoreUnknownKeys) и покажет пустой конфиг → пользователь увидит пустую форму. Это приемлемый регресс при откате.
- Feature flag не требуется — ломающих изменений API/сетевых нет.

## Проверка

| Вид | Что проверяем | AC/DEC |
|-----|--------------|--------|
| Unit test | Миграция старого JSON → ServerEntry("Default") | AC-001, DEC-001 |
| Unit test | AppConfig serialization/deserialization (3 servers) | AC-001 |
| Unit test | Сортировка: active on top, rest A→Z | DEC-003 |
| Unit test | Dirty flag: edit → dirty; save → clean | AC-003 |
| Manual | Запуск → тёмная тема, Default server | AC-005 |
| Manual | Add → edit → switch → dirty dialog → Discard | AC-002, AC-003 |
| Manual | Duplicate → copy created, auto-switch | AC-008 |
| Manual | Switch → Connect → verify server address | AC-004 |
| Manual | QR scan → new server imported | AC-006 |
| Manual | Export → QR contains active server's config | AC-007 |

## Соответствие конституции

- нет конфликтов: Kotlin 1.9+, MVVM + Compose, DDD, @sk-task/@sk-test маркеры на function/method/type declarations
