# Android: Per-Server App Settings Overrides + UI Redesign — Задачи

## Phase Contract

Inputs: plan, data-model, spec.
Outputs: исполнимые задачи с покрытием всех 10 AC.
Stop if: coverage не удаётся сопоставить — все AC покрыты.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `config/AppConfig.kt` | T1.1, T1.2, T4.1 |
| `ui/MainViewModel.kt` | T2.1, T3.1, T4.1 |
| `ui/ConnectScreen.kt` | T2.3, T2.4 |
| `ui/SettingsScreen.kt` | T2.2, T3.2 |
| `ui/TrafficScreen.kt` | T3.3 |
| `ui/TrafficHistory.kt` | T3.3 |
| `ui/MainActivity.kt` | T2.5 |
| `vpn/KvnVpnService.kt` | T2.1 |
| `config/ConfigSerializationTest.kt` | T4.1 |
| `config/AppConfigStore test` | T4.1 |

## Implementation Context

- **Цель MVP**: nullable override-поля в ConnectionConfig + Bottom Navigation + Server cards + Mini traffic + Settings tab с override индикацией + resolution on connect
- **Инварианты/семантика**:
  - `null` override = "use global"; non-null = "use this"
  - `appIncludeListOverride` и `appExcludeListOverride` независимы на уровне модели; conflict резолвится в `KvnVpnService` (include wins)
  - Migration копирует global → override только если все три override-поля `null`
- **Контракты/протокол**:
  - JSON-сериализация `ConnectionConfig` с nullable полями — обратно совместима (`ignoreUnknownKeys = true` в `json {}`)
  - `encodeDefaults = true` — null будет записан в JSON, что ожидаемо
- **Границы scope**:
  - НЕ меняем Go-ядро
  - НЕ трогаем routing/ranges поля ConnectionConfig
  - НЕ добавляем NavHost — `NavigationBar` + `when (selectedTab)`
- **Proof signals**:
  - APK собирается, табы переключаются
  - Override badge показывается на Settings tab
  - При коннекте резолвятся правильные DNS/apps
  - Старый JSON без override-полей загружается без ошибок
- **References**: DEC-001, DEC-002, DM-001, RQ-001–RQ-009

## Фаза 1: Data Model + Migration

Цель: расширить ConnectionConfig nullable полями, обеспечить миграцию существующих конфигов.

- [x] T1.1 Добавить nullable override-поля в ConnectionConfig. Touches: `config/AppConfig.kt`
  - `dnsServersOverride: List<String>? = null`
  - `appIncludeListOverride: List<String>? = null`
  - `appExcludeListOverride: List<String>? = null`
  - AC: AC-001, AC-002

- [x] T1.2 Migration v1→v2 в AppConfigStore — при первом чтении скопировать global в override активного сервера. Touches: `config/AppConfig.kt`
  - Guard: копировать только если все три override-поля `null`
  - AC: AC-010

## Фаза 2: MVP Slice

Цель: override logic + Settings tab + Server cards + Mini traffic + Bottom Navigation.

- [x] T2.1 Override resolution в MainViewModel + KvnVpnService. Touches: `ui/MainViewModel.kt`, `vpn/KvnVpnService.kt`
  - Добавить `resolveEffectiveDns(cfg)`, `resolveEffectiveAppInclude(cfg)`, `resolveEffectiveAppExclude(cfg)`
  - `connect()` передаёт resolved значения вместо raw config полей
  - AC: AC-009

- [x] T2.2 SettingsScreen composable с override индикацией. Touches: `ui/SettingsScreen.kt`, `ui/MainViewModel.kt`
  - Новый экран: DNS, DNS Cache, Per-app filtering; badge "override" если есть, иначе "Use global: <value>"
  - Clear override: кнопка "Use global" → устанавливает override в null
  - AC: AC-003

- [x] T2.3 Server cards в ConnectScreen (заменить dropdown). Touches: `ui/ConnectScreen.kt`
  - LazyColumn с карточками: статус-точка, имя, адрес, режим/транспорт, badge "Active"
  - Empty state при отсутствии серверов
  - AC: AC-007

- [x] T2.4 Mini traffic panel на Connect tab. Touches: `ui/ConnectScreen.kt`, `ui/MainViewModel.kt`
  - При CONNECTED: две цветные карточки RX/TX с total + speed
  - Speed = bytes/sec из дельты onTrafficUpdate
  - AC: AC-006

- [x] T2.5 Bottom Navigation в MainActivity. Touches: `ui/MainActivity.kt`
  - NavigationBar с 3 табами: Connect, Settings, Traffic
  - `when (selectedTab)` показывает соответствующий экран
  - AC: AC-005

## Фаза 3: Increments (Copy-to-server + Traffic)

Цель: расширить MVP copy-to-server и traffic graph.

- [x] T3.1 DuplicateAppSettingsToServer в MainViewModel. Touches: `ui/MainViewModel.kt`
  - `duplicateAppSettingsToServer(targetName)` — копирует global app/dns в override таргет-сервера
  - `clearAppSettingsOverride()` — сбрасывает override в null
  - AC: AC-004

- [x] T3.2 Copy-to-server UI в SettingsScreen. Touches: `ui/SettingsScreen.kt`
  - Селект таргет-сервера + "Copy" button
  - "Clear override" рядом с каждым полем
  - AC: AC-004

- [x] T3.3 TrafficScreen + Traffic graph. Touches: `ui/TrafficScreen.kt`, `ui/TrafficHistory.kt`, `ui/MainViewModel.kt`
  - TrafficHistory: ring buffer ArrayDeque<TrafficPoint> (60 max)
  - TrafficScreen: 4 stat cards + Canvas line chart (RX синяя, TX зелёная)
  - Placeholder при DISCONNECTED
  - Wire третий таб
  - AC: AC-008

## Фаза 4: Проверка

Цель: automated тесты, подтверждающие корректность модели и миграции.

- [x] T4.1 Unit тесты для модели и миграции. Touches: `config/ConfigSerializationTest.kt`, `config/AppConfigTest.kt`
  - JSON roundtrip c override-полями и без
  - Десериализация старого JSON → поля = null
  - Migration test: старый AppConfig → override активного сервера
  - Override resolution: effective = override ?? global
  - AC: AC-001, AC-002, AC-009, AC-010

## Покрытие критериев приемки

- AC-001 -> T1.1, T2.1, T4.1
- AC-002 -> T1.1, T2.1, T4.1
- AC-003 -> T2.2
- AC-004 -> T3.1, T3.2
- AC-005 -> T2.5
- AC-006 -> T2.4
- AC-007 -> T2.3
- AC-008 -> T3.3
- AC-009 -> T2.1, T4.1
- AC-010 -> T1.2, T4.1

## Заметки

- T1.1 и T1.2 — строгая последовательность (модель → миграция)
- T2.1 (resolution) должен быть до T2.2 (Settings UI использует resolved значения)
- T2.5 (Bottom Nav) — после T2.2 (Settings), T2.3 (Cards), T2.4 (Traffic placeholder) чтобы табы не были пустыми
- T3.1 (ViewModel) → T3.2 (UI) — последовательно
- T3.3 (Traffic) независим от T3.1/T3.2 — можно параллелить
- Trace-маркеры `@sk-task` и `@sk-test` — на implementation phase, над owning class/function declaration
