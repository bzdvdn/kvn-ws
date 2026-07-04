# Android: Per-Server App Settings Overrides — Модель данных

## Scope

- Связанные `AC-*`: `AC-001`, `AC-002`, `AC-009`, `AC-010`
- Связанные `DEC-*`: `DEC-001`
- Статус: `changed`
- ConnectionConfig расширяется nullable override-полями. AppConfig без изменений.

## Сущности

### DM-001 ConnectionConfig

- Назначение: per-server конфигурация соединения. Теперь также содержит опциональные override для глобальных настроек.
- Источник истины: DataStore JSON в `kvn_config` preferences
- Инварианты:
  - Если override-поле `null` — используется глобальное значение из `AppConfig`
  - Если override-поле не `null` — оно имеет приоритет над глобальным
- Связанные `AC-*`: AC-001, AC-002, AC-009
- Связанные `DEC-*`: DEC-001
- Поля (новые):
  - `dnsServersOverride: List<String>? = null` — per-server DNS; null = use global
  - `appIncludeListOverride: List<String>? = null` — per-server allowed apps; null = use global
  - `appExcludeListOverride: List<String>? = null` — per-server blocked apps; null = use global
- Жизненный цикл:
  - Создаётся: при добавлении сервера (новый пустой ConnectionConfig) или при QR-импорте
  - Обновляется: через Settings UI (override) или Copy-to-server
  - Удаляется: при удалении сервера
- Замечания по консистентности:
  - `appIncludeListOverride` и `appExcludeListOverride` не должны быть non-null одновременно — это инвариант VPN.Builder (only one mode). На уровне ConnectionConfig они независимы; конфликт резолвится на connect: если оба non-null — приоритет у include.

### DM-002 AppConfig (без изменений)

- Назначение: глобальный конфиг приложения
- Поля: `activeServer`, `servers: List<ServerEntry>`, `appIncludeList`, `appExcludeList`, `dnsServers`
- Глобальные поля — fallback, когда у сервера нет override

## Связи

- `AppConfig.servers[*].config` (он же `ConnectionConfig`) — владеет override-полями
- `AppConfig.appIncludeList` → fallback для `ConnectionConfig.appIncludeListOverride`
- `AppConfig.dnsServers` → fallback для `ConnectionConfig.dnsServersOverride`

## Производные правила

- **Effective DNS**: `connectionConfig.dnsServersOverride ?: appConfig.dnsServers`
- **Effective app include**: `connectionConfig.appIncludeListOverride ?: appConfig.appIncludeList`
- **Effective app exclude**: `connectionConfig.appExcludeListOverride ?: appConfig.appExcludeList`
- **Include/exclude conflict**: если `appIncludeListOverride` non-null, `appExcludeListOverride` игнорируется (и наоборот) — логика принудительного режима в `KvnVpnService`

## Переходы состояний

- **Add override**: пользователь нажимает Copy-to-server → глобальные значения копируются в override-поля таргет-сервера
- **Clear override**: пользователь нажимает "Use global" → override-поля устанавливаются в null
- **Migration v1→v2**: при первом запуске новой версии глобальные поля копируются в override активного сервера (только если все три override-поля null)

## Вне scope

- Override для routing/ranges — routing остаётся полностью в `ConnectionConfig` без fallback
- Override для MTU/TLS/Encryption и других ConnectionConfig полей
