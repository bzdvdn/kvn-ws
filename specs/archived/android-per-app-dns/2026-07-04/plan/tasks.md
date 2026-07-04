# Per-App Filtering + DNS Configuration — Задачи

## Phase Contract

Inputs: plan `android-per-app-dns/plan.md`, текущий код Android.
Outputs: задачи с покрытием критериев.
Stop if: задачи пересекаются с split-tunnel или уходят в geoip/geosite.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `ui/AppPickerScreen.kt` | T1.1 |
| `ui/ConnectScreen.kt` | T1.2 |
| `ui/MainViewModel.kt` | T1.3 |
| `config/AppConfig.kt` | уже готово |
| `vpn/KvnVpnService.kt` | уже готово |

## Implementation Context

- Цель MVP: полноценный App Picker + режим allowlist/blocklist + сохранение
- Границы приемки: AC-001, AC-002, AC-003, AC-004, AC-005
- Ключевые правила: app-level настройки не привязаны к серверу; allowlist и blocklist нельзя использовать одновременно
- Инварианты данных: appIncludeList и appExcludeList хранятся в AppConfig; ConnectionConfig не содержит app/DNS полей
- Контракты/протокол: KvnVpnService.start() принимает appIncludeList, appExcludeList, dnsServers отдельными параметрами
- Proof signals: gradle compile success, AppPickerScreen.kt существует, trace-маркеры проставлены
- Вне scope: split-tunnel по доменам, geoip/geosite, QR-экспорт app-level настроек

## Фаза 1: App Picker UI

Цель: создать AppPickerScreen и связать с ConnectScreen.

- [x] T1.1 Создать AppPickerScreen (Compose) со списком установленных приложений, поиском, чекбоксами, кнопкой подтверждения. Touches: `ui/AppPickerScreen.kt`
  - Полноэкранный список с иконками, именем, package name + поиск
  - Checkbox для выбора, кнопка Save с количеством выбранных
  - Навигация через conditional rendering (как QrScannerScreen)

- [x] T1.2 Обновить ConnectScreen — заменить текстовые поля на FilterChip (Allowed/Blocked) + кнопка "Select apps" + навигация в AppPickerScreen. Touches: `ui/ConnectScreen.kt`

- [x] T1.3 Обновить MainViewModel — добавить flows `appIncludeList`/`appExcludeList`/`dnsServers` из AppConfig и метод `saveAppSettings()`. Touches: `ui/MainViewModel.kt`

## Фаза 2: Проверка

Цель: доказать, что фича работает, и оставить код в reviewable состоянии.

- [x] T2.1 Проставить trace-маркеры `@sk-task android-per-app-dns#T1.x` во все изменённые файлы. Проверить сборку `./gradlew compileDebugKotlin`.

## Покрытие критериев приемки

- AC-001 -> T1.1
- AC-002 -> T1.1
- AC-003 -> T1.2
- AC-004 -> T1.2
- AC-005 -> T1.3

## Заметки

- Файл AppPickerScreen.kt будет новый — trace-маркер на уровне класса
- Для отображения имени приложения из package name используем `PackageManager.getApplicationLabel()`
- Для иконки: `PackageManager.getApplicationIcon()`
- `QUERY_ALL_PACKAGES` permission может понадобиться в AndroidManifest.xml
