# Per-App Filtering + DNS Configuration — План

## Phase Contract

Inputs: spec `android-per-app-dns/spec.md`, текущий код Android-клиента.
Outputs: plan, data model.
Stop if: spec неоправданно расширяет scope за пределы AppPicker + DNS.

## Цель

Заменить текстовый ввод package name на полноценный App Picker (список установленных приложений с иконками, поиском, чекбоксами), реализовать выбор режима (allowlist/blocklist) и сохранить DNS-серверы в app-level конфиг. Базовая структура AppConfig и KvnVpnService уже подготовлены — задача в UI и связывании.

## MVP Slice

App Picker Screen с поиском, чекбоксами, режимом allowlist/blocklist + сохранение выбора. DNS-конфигурация (текстовое поле) уже работает.

## First Validation Path

Собрать APK, установить, открыть секцию Apps → нажать "Select Apps" → убедиться, что список приложений отображается, можно выбрать, сохранить, и при подключении применяется фильтрация.

## Scope

- `src/android/app/src/main/kotlin/com/kvn/client/ui/AppPickerScreen.kt` — новый Compose экран
- `src/android/app/src/main/kotlin/com/kvn/client/ui/ConnectScreen.kt` — обновление секции Apps
- `src/android/app/src/main/kotlin/com/kvn/client/ui/MainViewModel.kt` — методы для App Picker
- `src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt` — модель уже готова

## Performance Budget

- `none` — список приложений загружается один раз, поиск < 100ms

## Implementation Surfaces

| Surface | Тип | Зачем |
|---------|-----|-------|
| `ui/AppPickerScreen.kt` | Новая | Полноэкранный список приложений с чекбоксами |
| `ui/ConnectScreen.kt` | Существующая | Заменить текстовое поле на кнопку "Select Apps" |
| `ui/MainViewModel.kt` | Существующая | Методы для App Picker |
| `config/AppConfig.kt` | Существующая | Уже содержит appIncludeList/appExcludeList/dnsServers |

## Bootstrapping Surfaces

- `none` — структура уже готова

## Влияние на архитектуру

- Локальное: новый Compose экран, новый navigation route (или conditional rendering)
- Нет изменений в VpnService, data model уже расширена

## Acceptance Approach

- AC-001 App Picker Screen: новый `AppPickerScreen` + навигация
- AC-002 Search/Filter: `searchBar` + `filter` в списке
- AC-003 Mode Switch: `SegmentedButton` (Allowlist / Blocklist) на ConnectScreen
- AC-004 DNS Configuration: уже сделано
- AC-005 Settings Persist: уже сделано (AppConfig)

## Данные и контракты

- `AppConfig.appIncludeList: List<String>` — список package name для allowlist
- `AppConfig.appExcludeList: List<String>` — список package name для blocklist
- `AppConfig.dnsServers: List<String>` — список DNS-серверов
- Контракты не меняются — расширение существующей data model

## Стратегия реализации

- DEC-001 App Picker как отдельный Compose Screen
  Why: Chrome-подобный experience, полноэкранный список, поиск — не вписать в ConnectScreen
  Tradeoff: нужна навигация назад (или back handler)
  Affects: ConnectScreen, AppPickerScreen
  Validation: AC-001

- DEC-002 Режим через SegmentedButton (Allowlist / Blocklist)
  Why: Material 3 компонент, явно показывает выбор, нельзя выбрать оба сразу
  Tradeoff: SegmentedButton не кастомный
  Affects: ConnectScreen
  Validation: AC-003

- DEC-003 Сохранение через ViewModel.saveAppSettings()
  Why: единая точка сохранения app-level настроек, DataStore — единственный источник истины
  Tradeoff: при каждом изменении — перезапись всего AppConfig
  Affects: MainViewModel
  Validation: AC-005

## Incremental Delivery

### MVP (Первая ценность)

- App Picker Screen + поиск + чекбоксы
- Mode switch на ConnectScreen
- Сохранение выбора через ViewModel

### Итеративное расширение

- `none` — фича достаточно узкая, всё в MVP

## Порядок реализации

1. AppPickerScreen — создание UI с поиском
2. ConnectScreen — замена текстового поля на кнопку + навигация
3. MainViewModel — связывание App Picker с AppConfig
4. Trace-маркеры + проверка сборки

## Риски

- `PackageManager.getInstalledApplications()` требует `QUERY_ALL_PACKAGES` permission на Android 11+
  Mitigation: добавить `<uses-permission android:name="android.permission.QUERY_ALL_PACKAGES" />` (нормальное разрешение, auto-granted)
- `addAllowedApplication()` бросает `IllegalArgumentException` если пакет не найден
  Mitigation: try-catch при вызове в establishTun (уже есть)

## Rollout и compatibility

- Настройки apps/DNS сохраняются в AppConfig, старые конфиги с пустыми списками работают как "всё через VPN"
- QR-код не экспортирует app-level настройки (device-specific)
- `none` — специальных rollout действий не требуется

## Проверка

- `./gradlew compileDebugKotlin` — успешная сборка
- Ручная проверка: APK → открыть App Picker → выбрать приложения → подключиться → проверить tcpdump/ip route
- AC-001..AC-005 покрываются задачами

## Соответствие конституции

- нет конфликтов
