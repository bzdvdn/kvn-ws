# Multi-server + Dark Theme для Android-клиента — Задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| `config/AppConfig.kt` | T1.1, T4.1 |
| `ui/theme/Color.kt` (new) | T1.2 |
| `res/values/themes.xml` | T1.2 |
| `ui/MainViewModel.kt` | T2.1, T3.2 |
| `ui/ConnectScreen.kt` | T2.2, T3.1, T3.2 |
| `ui/QrScannerScreen.kt` | T3.1 |
| `ui/QrExportScreen.kt` | T3.1 |
| `config/*Test.kt` (existing/new) | T4.1 |

## Implementation Context

- **Цель MVP:** Пользователь может добавить 2+ сервера в Android-клиенте, переключаться между ними (с dirty-диалогом) и коннектиться к выбранному. Тёмная тема в цветах kvn-web.
- **Инварианты/семантика:**
  - DataStore: один ключ `config_json`, значение — JSON `AppConfig { activeServer, servers[] }`
  - Миграция: старый `ConnectionConfig` оборачивается в `ServerEntry("Default", ...)`
  - Сортировка: активный сервер всегда первый, остальные по алфавиту (A→Z)
  - Dirty: ViewModel хранит `isDirty: Boolean`, сбрасывается при save/switch with discard
- **Ошибки/коды:** нет новых кодов ошибок; QR import -> Toast при невалидном JSON
- **Контракты/протокол:**
  - QR-формат JSON не меняется (тот же `ConnectionConfig` / web JSON)
  - `ConnectionConfig` не изменяется — полная обратная совместимость
- **Границы scope:**
  - Не делаем глобальные настройки отдельно от per-server
  - Не делаем drag-to-reorder (только сортировка по имени)
- **Proof signals:**
  - DataStore: старый конфиг после миграции = ServerEntry("Default", ...)
  - UI: селектор, dirty-диалог, Add/Duplicate/Delete работают
  - Тема: Material 3 dark с цветами #161616 / #222 / #1a5a9e / #2e7d32 / #b71c1c

## Фаза 1: База (data model + theme)

Цель: подготовить DataStore для multi-server и тёмную тему. Две независимые параллельные задачи.

- [x] T1.1 Добавить `ServerEntry` и `AppConfig` типы, DataStore миграцию и load/save
  - Новые `@Serializable` data class: `ServerEntry(name, config)`, `AppConfig(activeServer, servers)`
  - `AppConfig.load()`: читает `config_json`, если старый `ConnectionConfig` -> обёртка в `ServerEntry("Default", ...)`, сохраняет как `AppConfig`
  - `AppConfig.save()`: сериализует `AppConfig` в `config_json`
  - `AppConfig.configFlow`: `Flow<AppConfig>`
  - Trace: `@sk-task multi-server-android-client#T1.1` над функциями
  - Touches: `config/AppConfig.kt`

- [x] T1.2 Добавить тёмную тему с палитрой kvn-web
  - Новый файл `ui/theme/Color.kt`: кастомные цвета фона (#161616), поверхности (#222), текста (#d0d0d0), акцента (#1a5a9e), успеха (#2e7d32), ошибки (#b71c1c), предупреждения (#ff9800)
  - `themes.xml`: новая тема `Theme.KvnClient.Dark` extends `Theme.Material3.Dark.NoActionBar` или аналогичная
  - `MainActivity` (`setContent`): обернуть в `MaterialTheme(colorScheme = darkKvnWebColorScheme)`
  - Trace: `@sk-task multi-server-android-client#T1.2` над color scheme val и Activity
  - Touches: `ui/theme/Color.kt` (new), `res/values/themes.xml`, `ui/MainActivity.kt`

## Фаза 2: MVP (ViewModel + UI)

Цель: multi-server UI — селектор, CRUD, dirty-диалог, connect с активным сервером.

- [x] T2.1 Переписать `MainViewModel` для multi-server
  - `savedConfig: StateFlow<ConnectionConfig>` -> `savedAppConfig: StateFlow<AppConfig>` + `activeServerConfig: StateFlow<ConnectionConfig>`
  - `activeServerName: StateFlow<String>` — имя текущего выбранного сервера
  - `isDirty: StateFlow<Boolean>` — флаг несохранённых изменений
  - `servers: StateFlow<List<ServerEntry>>` — отсортированный список (активный первый, остальные A→Z)
  - `addServer(name, config)`, `duplicateServer(name)`, `deleteServer(name)`, `renameServer(oldName, newName)`, `setActiveServer(name)`, `saveCurrentServer(config)`
  - `connect()` -> использует `activeServerConfig`, вызывает сохранение
  - `qrConfig`: вместо fillForm -> `addServer("Imported <ts>", config)`
  - Trace: `@sk-task multi-server-android-client#T2.1` над class и методами
  - Touches: `ui/MainViewModel.kt`

- [x] T2.2 Добавить селектор серверов и кнопки CRUD в `ConnectScreen`
  - Server selector: `ExposedDropdownMenuBox` или `DropdownMenu` со списком имён серверов
  - Кнопки: "Add Server" (новый с именем "Server N"), "Duplicate", "Delete" (disabled при единственном)
  - Dirty-диалог: `AlertDialog` с "Save & Switch" / "Discard & Switch" / "Cancel" при попытке переключения с dirty=true
  - Форма: все поля подгружаются из `activeServerConfig` при смене сервера
  - Rename: редактирование имени в селекторе или через диалог
  - Trace: `@sk-task multi-server-android-client#T2.2` над composable и ключевыми обработчиками
  - Touches: `ui/ConnectScreen.kt`

## Фаза 3: QR + edge cases

Цель: QR импорт/экспорт для multi-server и краевые случаи.

- [x] T3.1 Адаптировать QR-сканер и QR-экспорт для multi-server
  - `QrScannerScreen`: при сканировании вызывает `vm.addServer("Imported <ts>", config)` вместо fillForm
  - `QrExportScreen`: экспортирует `vm.activeServerConfig` вместо `buildConfig()`
  - `ConnectScreen` QR-кнопки: import -> add, export -> active server
  - Trace: `@sk-task multi-server-android-client#T3.1`
  - Touches: `ui/QrScannerScreen.kt`, `ui/QrExportScreen.kt`, `ui/ConnectScreen.kt`

- [x] T3.2 Реализовать edge cases
  - Удаление активного сервера: автоматическое переключение на первый в списке
  - Delete disabled при единственном сервере
  - Duplicate при dirty=true: сначала Save/Discard диалог
  - Проверка пустого имени при rename
  - Сортировка: активный первый, остальные A→Z
  - Trace: `@sk-task multi-server-android-client#T3.2` над обработчиками
  - Touches: `ui/MainViewModel.kt`, `ui/ConnectScreen.kt`

## Фаза 4: Тесты и завершение

Цель: automated coverage + верификация.

- [x] T4.1 Добавить unit-тесты
  - Миграция: старый ConnectionConfig JSON -> `ServerEntry("Default", ...)`
  - AppConfig serialization: 3 server entry, verify round-trip
  - Сортировка: active on top, rest A→Z
  - Dirty flag: edit -> dirty, save -> clean
  - Trace: `@sk-test multi-server-android-client#T4.1`
  - Touches: `config/AppConfigTest.kt` (новый или existing)

- [x] T4.2 Проверить сборку и trace-маркеры
  - `./gradlew assembleDebug` — успешная сборка
  - `./gradlew test` — все unit-тесты проходят
  - Проверить наличие `@sk-task`/`@sk-test` маркеров на всех изменённых объявлениях
  - Touches: CI

## Фаза 5: Пост-MVP улучшения

Цель: стабильность и UX.

- [x] T5.1 Исправить парсинг web JSON QR с null-полями
  - `WebConfig.auto_reconnect` изменён с `Boolean` на `Boolean?` (kvn-web отправляет `"auto_reconnect": null`)
  - `webToAndroidConfig` использует `?: true` для fallback
  - Trace: `@sk-task kvn-android#T5.1`
  - Touches: `ui/QrScannerScreen.kt`

- [x] T5.2 Заменить текст кнопок CRUD на иконки с семантическими цветами
  - Add (`Icons.Default.Add`, `KvnSuccess` #2E7D32)
  - Copy (`Icons.Default.ContentCopy`, `KvnPrimary` #1A5A9E)
  - Rename (`Icons.Default.Edit`, `KvnWarning` #FF9800)
  - Delete (`Icons.Default.Delete`, `KvnError` #B71C1C)
  - Все кнопки: `Button` с `weight(1f).height(40.dp)`
  - Добавлена зависимость `material-icons-extended` для `ContentCopy`
  - Trace: `@sk-task kvn-android#T5.2`
  - Touches: `ui/ConnectScreen.kt`, `app/build.gradle.kts`

## Покрытие критериев приемки

- AC-001 (Multi-server CRUD) -> T1.1, T2.1, T2.2, T4.1
- AC-002 (Add/switch servers) -> T2.1, T2.2
- AC-003 (Dirty confirm) -> T2.1, T2.2
- AC-004 (Connect uses active) -> T2.1
- AC-005 (Dark theme) -> T1.2
- AC-006 (QR import -> add) -> T3.1
- AC-007 (QR export -> active) -> T3.1
- AC-008 (Duplicate) -> T2.1, T2.2, T3.2
- RQ-011 (Web JSON null fields) -> T5.1
- RQ-012 (Button icons) -> T5.2

## Заметки

- T1.1 и T1.2 независимы — можно параллелить
- T2.1 (ViewModel) обязателен перед T2.2 (UI) и T3.1 (QR)
- T3.2 (edge cases) может делаться параллельно с T3.1
- ConnectionConfig не меняется — все существующие unit-тесты (ConfigSerializationTest, crypto, protocol) остаются зелёными
