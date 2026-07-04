# Android: Per-Server App Settings Overrides + UI Redesign — План

## Phase Contract

Inputs: spec, inspect (pass), минимальный repo-контекст.
Outputs: plan, data-model.
Stop if: spec расплывчата — spec чёткая, inspect pass, без блокеров.

## Цель

Инкрементально расширить `AppConfig`/`ConnectionConfig` nullable override-полями, добавить Bottom Navigation с 3 табами, переработать Connect tab (карточки серверов, мини-панель трафика), вынести Settings и Traffic на отдельные экраны. Весь код — Kotlin/Compose в Android-модуле; Go-ядро не затрагивается.

## MVP Slice

Override-модель (AC-001, 002, 003, 009, 010) + Bottom Navigation (AC-005) + Server cards (AC-007) + Mini traffic panel (AC-006). Traffic Graph (AC-008) и Copy-to-server (AC-004) — в следующих инкрементах.

## First Validation Path

1. Собрать APK (`./gradlew assembleDebug`)
2. Открыть приложение — видны 3 пустых таба (Connect placeholder, Settings, Traffic placeholder)
3. Добавить 2 сервера через QR/Add — карточки отображаются
4. Переключить сервер — настройки DNS/App фильтров переключаются (или показывают "Use global")
5. Подключиться — мини-панель RX/TX на Connect tab

## Scope

- `ConnectionConfig`: nullable override-поля (`dnsServersOverride`, `appIncludeListOverride`, `appExcludeListOverride`)
- `AppConfig`: глобальные поля остаются без изменений
- `AppConfigStore`: миграция v1→v2 (copy global → override активного сервера)
- `MainViewModel`: `duplicateAppSettingsToServer`, `clearAppSettingsOverride`, `resolveEffective*`
- `ConnectScreen`: cards вместо dropdown, mini traffic panel под статусом
- `SettingsScreen`: новый экран (DNS + App filtering + override UI + Copy to server)
- `TrafficScreen`: новый экран (stat cards + line chart)
- `MainActivity`: Bottom Navigation (NavigationBar)

## Performance Budget

- `none` — изменения не затрагивают горячие пути туннеля; UI-операции (обновление графика 1/s) тривиальны

## Implementation Surfaces

| Surface | Почему | Новая/Существующая |
|---|---|---|
| `config/AppConfig.kt` | nullable override-поля + миграция | существующая |
| `ui/MainViewModel.kt` | CRUD override, resolution logic | существующая |
| `ui/ConnectScreen.kt` | server cards + mini traffic | существующая |
| `ui/SettingsScreen.kt` | override UI + copy-to-server | новая |
| `ui/TrafficScreen.kt` | graph + stats | новая |
| `ui/SettingsSection.kt` | переиспользуемый компонент | существующая |
| `ui/MainActivity.kt` | Bottom Navigation | существующая |
| `vpn/KvnVpnService.kt` | resolution effective values on start | существующая (минор) |

## Bootstrapping Surfaces

- `ui/SettingsScreen.kt` — новая
- `ui/TrafficScreen.kt` — новая
- `TrafficHistory.kt` — data class ring buffer (может быть в config/ или ui/)

## Влияние на архитектуру

- **Data model**: `ConnectionConfig` расширяется nullable полями; старый JSON без них десериализуется корректно (`ignoreUnknownKeys = true`). Migration one-shot в `AppConfigStore`.
- **UI**: Scaffold → Scaffold + NavigationBar. Каждый таб — отдельный composable. Состояние таба хранится в `MainViewModel`.
- **Compatibility**: полная обратная совместимость. Старые конфиги (без override-полей) работают как global-only.
- **Traffic data**: `TrafficHistory` — in-memory ring buffer, не сохраняется. Живёт пока жив ViewModel/сессия.

## Acceptance Approach

- **AC-001** → nullable `dnsServersOverride` в ConnectionConfig; resolution в `connect()` — проверить через Notification или log
- **AC-002** → nullable `appIncludeListOverride`/`appExcludeListOverride`; resolution в `connect()` — проверить через `Builder.addAllowedApplication` trace
- **AC-003** → SettingsScreen: if override != null → show badge + value + global underneath — визуально
- **AC-004** → `duplicateAppSettingsToServer()` в VM — после вызова badge появляется на таргете
- **AC-005** → `NavigationBar` с 3 item-ами в `MainActivity` — визуально/тап
- **AC-006** → mini traffic panel на Connect tab при CONNECTED — 2 card-композы
- **AC-007** → server cards: `LazyColumn` of `ServerCard` composable — визуально
- **AC-008** → TrafficScreen с `Canvas` line chart + `TrafficHistory` ring buffer (60 точек) — визуально, обновление 1/s
- **AC-009** → `connect()` берёт override ?? global — проверить через notification/log
- **AC-010** → `AppConfigStore` migration block — при первом чтении старого формата копировать в override

## Данные и контракты

Связанные AC: AC-001, 002, 003, 009, 010.
Data model меняется — см. `data-model.md`.
Контракты: JSON-сериализация `ConnectionConfig` — обратно совместима (nullable поля с дефолтом null).

## Стратегия реализации

### DEC-001 Nullable override fields вместо flattened model

- **Why**: минимальное изменение существующей схемы; `null` = "use global", не нужно добавлять boolean флаг "override enabled"
- **Tradeoff**: резолвинг на каждый connect — небольшой overhead; nullable поля в JSON занимают место при `encodeDefaults = true`
- **Affects**: `ConnectionConfig`, `AppConfigStore`, `MainViewModel`, `KvnVpnService`
- **Validation**: старый JSON без override-полей загружается, миграция не вызывается

### DEC-002 Bottom Navigation через NavigationBar + when (не NavHost)

- **Why**: просто 3 таба без back stack; NavHost избыточен, добавляет boilerplate для простого случая
- **Tradeoff**: при росте числа табов (>5) придётся переходить на NavHost; состояние таба хранится в VM
- **Affects**: `MainActivity`
- **Validation**: тапы переключают контент, состояние таба сохраняется

### DEC-003 TrafficGraph как Canvas composable (не библиотека)

- **Why**: нет зависимостей, полный контроль над рендерингом; 60 точек — тривиальная нагрузка
- **Tradeoff**: нет touch-интерактивности (zoom, tooltip) без дополнительного кода
- **Affects**: `TrafficScreen.kt`
- **Validation**: график отрисовывается, обновляется каждую секунду

### DEC-004 TrafficHistory — in-memory ring buffer в ViewModel

- **Why**: данные живут только в сессии, не нужно сохранять; кольцевой буфер фиксированного размера (60 точек) без аллокаций
- **Tradeoff**: при survive config change (поворот) данные теряются — можно пережить
- **Affects**: `MainViewModel`, `TrafficScreen`
- **Validation**: после 60s буфер циклически перезаписывается

## Incremental Delivery

### MVP (Первая ценность)

- Data model: nullable поля + миграция
- Bottom Navigation (Connect + Settings)
- Server cards (Connect tab)
- Override UI (Settings tab) — просмотр, badge
- Mini traffic panel (Connect tab)
- Resolution on connect
- **AC покрывает**: 001, 002, 003, 005, 006, 007, 009, 010

### Итеративное расширение

- Инкремент 2: Copy-to-server (AC-004) — селект + кнопка в Settings
- Инкремент 3: Traffic Screen + Graph (AC-008) — stat cards + line chart + третий таб

## Порядок реализации

1. **Data model** — ConnectionConfig nullable поля, AppConfigStore migration
2. **MainViewModel** — new methods, resolution logic, TrafficHistory
3. **ConnectScreen** — server cards, mini traffic panel
4. **SettingsScreen** — new screen with override UI
5. **MainActivity** — Bottom Navigation, wire screens
6. *Copy-to-server* — селект + кнопка в SettingsScreen
7. *TrafficScreen* — stat cards + graph + третий таб

Шаги 1-5 можно делать последовательно; 6 и 7 независимы друг от друга и от 5.

## Риски

- **Риск**: миграция перезаписывает существующий override активного сервера
  **Mitigation**: копировать только если все три override-поля null (AC-010 suggestion)
- **Риск**: Bottom Navigation ломает существующий пользовательский поток (muscle memory)
  **Mitigation**: сохранить Connect tab как дефолтный при открытии
- **Риск**: Traffic graph потребляет CPU при 1fps ререндеринге
  **Mitigation**: `drawWithCache` / проверка что перерисовка только при изменении данных

## Rollout и compatibility

- Полностью backward-compatible: старые установки без override-полей работают как global-only
- Migration один раз при первом запуске; если неактуальна — `runBlocking` в DataStore flow
- Feature flag не требуется — изменения не деструктивны

## Проверка

- **Unit tests**: ConfigSerializationTest — сериализация/десериализация с override-полями и без
- **Unit tests**: AppConfigStore migration test — старый JSON → override в активном сервере
- **Manual**: подключиться к двум серверам с разными override → разные DNS в notification
- **Manual**: Copy-to-server → badge появляется на таргете
- **Manual**: Traffic graph отображается при CONNECTED, placeholder при DISCONNECTED
- **AC coverage**: все 10 AC покрыты визуальной проверкой или unit-тестом

## Соответствие конституции

- нет конфликтов. MVVM + Compose сохраняется. Traceability (`@sk-task`) будет добавлена на implementation phase. DataStore — существующий механизм persistence.
