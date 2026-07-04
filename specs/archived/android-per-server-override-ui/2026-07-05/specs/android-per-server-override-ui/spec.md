# Android: Per-Server App Settings Overrides + UI Redesign

## Scope Snapshot

- In scope: per-server override model for DNS and per-app filtering, duplicate-settings-to-server, bottom navigation (Connect/Settings/Traffic), server card preview, real-time traffic graph.
- Out of scope: server-side changes, protocol changes, iOS client, per-server routing overrides (routing rules stay per-server as-is), widget/quick-tile.

## Цель

Пользователь KVN Android с несколькими серверами получает возможность привязать DNS и per-app фильтры к конкретному серверу (override) или использовать глобальные настройки приложения. UI разделён на три вкладки, серверы отображаются в виде карточек с превью, и добавлен экран мониторинга трафика с графиком скорости. Успех фичи измеряется по тому, что пользователь может за 2 tap-а скопировать настройки с одного сервера на другой и видеть график трафика в реальном времени.

## Основной сценарий

1. Пользователь открывает приложение и видит три вкладки снизу: Connect, Settings, Traffic.
2. На Connect: список серверов в виде карточек (статус, имя, адрес, режим), CRUD-кнопки.
3. На Settings: глобальные DNS и App Filtering; если у активного сервера есть override — отображается badge, иначе — надпись "Use global".
4. Пользователь выбирает "Copy to server" — выбирает целевой сервер, и текущие глобальные настройки копируются в override того сервера.
5. На Traffic: 4 карточки статистики (RX, TX, время сессии, пиковая скорость) и line chart скорости за последние 60 с.
6. При коннекте: если у сервера есть override — используются override-значения, иначе глобальные.

## User Stories

- **P1 Story**: Как пользователь с 3 серверами, я хочу чтобы на одном сервере были свои DNS/app-фильтры, а на двух других — общие, чтобы не настраивать одно и то же трижды.
- **P2 Story**: Как пользователь, я хочу быстро скопировать настройки с одного сервера на другой одной кнопкой.
- **P3 Story**: Как пользователь, я хочу видеть график скорости соединения, чтобы понимать качество канала.

## MVP Slice

P1 + P2 + Bottom Navigation + Server Cards. Traffic Graph — P3.

## First Deployable Outcome

APK с Bottom Navigation, карточками серверов, Settings-табом с override UI и кнопкой "Copy to server". Трафик-граф — следующим шагом.

## Scope

- `ConnectionConfig`: добавить nullable override-поля (`appIncludeListOverride`, `appExcludeListOverride`, `dnsServersOverride`)
- `AppConfig`: оставить глобальные поля `appIncludeList`, `appExcludeList`, `dnsServers` как есть
- `AppConfigStore`: миграция при первом запуске (скопировать глобальные значения в override активного сервера)
- `MainViewModel`: `duplicateAppSettingsToServer(name)`, `clearAppSettingsOverride()`, резолвер `resolveEffective*(cfg, global)`
- `ConnectScreen`: заменить Dropdown-селектор на список карточек, добавить Bottom Navigation (3 таба)
- `SettingsScreen`: новый экран с DNS + App Filtering, индикацией override, Copy to server
- `TrafficScreen`: новый экран с 4 stat-карточками + Canvas line chart (60s ring buffer)
- Файлы: `src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt`, `SettingsSection.kt`, `MainViewModel.kt`, `ConnectScreen.kt`, новый `SettingsScreen.kt`, новый `TrafficScreen.kt`, `MainActivity.kt` (навигация)

## Контекст

- `AppConfig` сейчас хранит `appIncludeList`, `appExcludeList`, `dnsServers` как глобальные поля (не per-server)
- `ConnectionConfig` не содержит этих полей — их нужно добавить как nullable
- Kotlinx serialization с `ignoreUnknownKeys = true` и `encodeDefaults = true` уже настроен — добавление nullable полей совместимо со старым форматом
- Bottom Navigation потребует рефакторинга `MainActivity` — сейчас Scaffold с одним контентом
- Трафик-граф: OkHttp WebSocket уже возвращает RX/TX в `onTrafficUpdate` — данные есть, нужен только UI
- Уже существует `KvnErorr`, `KvnPrimary`, `KvnSuccess` и `DarkKvnWebColorScheme` в `Color.kt`

## Зависимости

- `none` — внешних сервисов и меж-спековых зависимостей нет

## Требования

- RQ-001 Система ДОЛЖНА поддерживать per-server override для DNS серверов: nullable `dnsServersOverride` в `ConnectionConfig`; если null — используется глобальное значение из `AppConfig.dnsServers`
- RQ-002 Система ДОЛЖНА поддерживать per-server override для per-app фильтров: nullable `appIncludeListOverride` и `appExcludeListOverride` в `ConnectionConfig`; если null — используются глобальные значения из `AppConfig`
- RQ-003 Система ДОЛЖНА показывать в UI: badge `override` если у сервера установлен override, и `Use global`/дефолтные значения если нет
- RQ-004 Система ДОЛЖНА предоставлять кнопку "Copy to server" в Settings, которая копирует текущие глобальные DNS + App фильтры как override на выбранный сервер
- RQ-005 Система ДОЛЖНА предоставить Bottom Navigation с тремя вкладками: Connect, Settings, Traffic
- RQ-006 Connect tab ДОЛЖЕН отображать серверы в виде карточек: статус-точка (connected/disconnected), имя, адрес, режим, транспорт, активный badge
- RQ-007 Traffic tab ДОЛЖЕН показывать: total RX/TX, текущую скорость, пиковую скорость, время сессии, line chart скорости за последние 60 с
- RQ-008 Система ДОЛЖНА при подключении резолвить эффективные значения: override ?? global
- RQ-009 Migration: при первом запуске после обновления глобальные `appIncludeList`, `appExcludeList`, `dnsServers` ДОЛЖНЫ быть скопированы в `override`-поля активного сервера (если они ещё не установлены)

## Вне scope

- Per-server routing overrides — routing остаётся полностью в `ConnectionConfig` как и сейчас
- Multi-profile (несколько AppConfigs) — один конфиг, один набор глобальных настроек
- Dark/Light theme toggle — только dark как сейчас
- Push-уведомления о трафике или лимитах
- Export/import настроек с override-полями (QR уже экспортирует весь ConnectionConfig — override-поля поедут автоматически)

## Критерии приемки

### AC-001 Per-server DNS override

- Почему важно: разные серверы могут требовать разных DNS (например, AdGuard vs Cloudflare)
- **Given** пользователь имеет сервер "Home" и сервер "Work"
- **When** пользователь устанавливает DNS "9.9.9.9" глобально, а для "Home" устанавливает override "1.1.1.1"
- **Then** при подключении к "Home" используются DNS "1.1.1.1", при подключении к "Work" — "9.9.9.9"
- Evidence: VPN-логи или notification показывают resolv-сервера, соответствующие серверу

### AC-002 Per-server app filtering override

- Почему важно: на рабочих серверах может быть нужно пускать все приложения, на домашних — только некоторые
- **Given** у сервера "Work" установлен override `appIncludeListOverride = ["com.telegram"]`
- **When** пользователь подключается к "Work"
- **Then** VPN.Builder.addAllowedApplication содержит только com.telegram
- Evidence: проверить через `dumpsync` или UI что список приложений соответствует override

### AC-003 UI indicator for override state

- Почему важно: пользователь должен видеть, что настройки переопределены для конкретного сервера
- **Given** у сервера есть DNS override
- **When** пользователь открывает Settings tab
- **Then** рядом с DNS полем отображается badge "override" и показано override-значение, а под ним серым — глобальный дефолт
- Evidence: визуальный badge на экране

### AC-004 Copy to server

- Почему важно: быстрый перенос настроек без ручного ввода
- **Given** глобальные DNS = "1.1.1.1,8.8.8.8", appInclude = ["com.whatsapp"]
- **When** пользователь выбирает сервер "Tokyo" в "Copy to server" и нажимает Copy
- **Then** `ConnectionConfig` для "Tokyo" получает `dnsServersOverride = ["1.1.1.1","8.8.8.8"]` и `appIncludeListOverride = ["com.whatsapp"]`
- Evidence: после копирования на Settings tab виден badge override для Tokyo

### AC-005 Bottom Navigation with 3 tabs

- Почему важно: разделение функциональности на логические экраны
- **Given** приложение открыто
- **When** пользователь тапает на "Settings"
- **Then** отображается экран настроек, а не Connect
- **When** пользователь тапает на "Traffic"
- **Then** отображается экран трафика (или placeholder если не подключен)
- Evidence: визуальное переключение контента

### AC-006 Mini traffic panel on Connect tab

- Почему важно: пользователь видит RX/TX не отрываясь от Connect tab, без переключения на Traffic
- **Given** пользователь подключен к серверу
- **When** Connect tab активен
- **Then** под статусом отображаются две мини-карточки с RX total + speed и TX total + speed, стилизованные (цветные иконки/полоски)
- Evidence: компактная traffic-панель видна на Connect tab

### AC-007 Server cards in Connect tab

- Почему важно: быстрый обзор всех серверов и их статуса
- **Given** у пользователя 3 сервера, один подключен
- **Then** Connect tab показывает 3 карточки; активный сервер с зелёной точкой и "Active" badge, остальные с серой точкой
- Evidence: карточки с точками, именем, адресом, мета-информацией

### AC-008 Traffic graph

- Почему важно: визуальная обратная связь о качестве соединения
- **Given** пользователь подключен к серверу
- **When** открыт Traffic tab
- **Then** отображается line chart скорости за последние 60 с с двумя линиями (RX синяя, TX зелёная), 4 stat-карточки
- Evidence: график обновляется каждую секунду

### AC-009 Effective value resolution on connect

- Почему важно: консистентность между UI и реальным VPN
- **Given** у сервера установлен DNS override "1.0.0.1"
- **When** пользователь нажимает Connect
- **Then** в `KvnVpnService.dnsServers` передаётся "1.0.0.1", а не глобальное значение
- Evidence: лог/notification показывают правильные DNS

### AC-010 Migration from v1 to v2

- Почему важно: пользователи с существующими настройками не должны их потерять
- **Given** у пользователя сохранён `AppConfig` без override-полей (старый формат)
- **When** приложение запускается в новой версии
- **Then** глобальные `appIncludeList`, `appExcludeList`, `dnsServers` копируются в `override`-поля активного сервера
- Evidence: после миграции на Settings tab отображаются badge override для активного сервера с теми же значениями

## Допущения

- Nullable override со значением `null` эквивалентен "использовать глобальное" — нет отдельного флага "use global"
- При копировании через "Copy to server" всегда копируются текущие глобальные значения, а не override-значения с другого сервера
- Traffic graph хранит данные только за текущую сессию; при дисконнекте/реконнекте история сбрасывается
- Bottom Navigation использует стандартный Material 3 `NavigationBar` с 3 иконками

## Критерии успеха

- SC-001 Переключение между вкладками — <100ms отклик
- SC-002 Traffic graph обновляется 1 раз в секунду без пролагов UI

## Краевые случаи

- Нет серверов: Connect tab показывает empty state "Add your first server"
- Нет override: Settings tab показывает "Use global: <value>" без badge
- Все серверы удалены: запретить удаление последнего (как сейчас)
- Copy to server когда глобальные настройки пусты: копируются пустые списки
- Traffic tab при DISCONNECTED: показать placeholder "Connect to see traffic"
- Connect tab при CONNECTED: показывать мини-панель трафика (2 карточки RX/TX с тоталами и текущей скоростью), отдельно от полного Traffic tab
- Traffic history при reconnect: сброс буфера
- Migration когда нет активного сервера: копировать в первый сервер или создать дефолтный

## Открытые вопросы

- Нужна ли кнопка "Clear override" для сброса к глобальным значениям? Пока — да, добавить.
- Traffic graph: достаточно bar chart или нужен smooth line chart? В MVP — bar, при переходе на Canvas — line.
- Нужна ли поддержка IPv6 в traffic graph? Пока только IPv4 total bytes.
