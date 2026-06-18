# Data Model: multi-server-android-client

## Status

- `changed` — data model расширяется с `ConnectionConfig` на `AppConfig` + `ServerEntry`

## Текущая модель (до изменений)

```kotlin
@Serializable
data class ConnectionConfig(
    val serverAddress: String = "",
    val port: Int = 443,
    val serverPath: String = "/kvn",
    val token: String = "",
    // ... ~30 полей
)

// DataStore: Preferences под ключом "config_json"
// Значение: JSON-сериализованный ConnectionConfig (единственный)
```

## Новая модель (после изменений)

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

- DataStore: ключ `"config_json"` остаётся, но значение — JSON `AppConfig`

## Миграция

При загрузке:
1. Прочитать raw JSON из `config_json`
2. Попробовать парсить как `AppConfig` (есть поля `activeServer`/`servers`)
3. Если не удалось — парсить как старый `ConnectionConfig`
4. Если удалось — обернуть в `ServerEntry("Default", oldConfig)`, сохранить как `AppConfig`

## Сериализация

- `json = Json { ignoreUnknownKeys = true; encodeDefaults = true }` — без изменений
- `ConnectionConfig` не меняется (полная совместимость с QR-форматом)

## Сортировка

- В памяти: активный сервер всегда первый, остальные по алфавиту (A→Z)
- В DataStore: порядок сохранения не гарантирован (сортируем при чтении)

## Затрагиваемые файлы

- `config/AppConfig.kt` — новые типы + обновлённый DataStore load/save
- `ui/MainViewModel.kt` — переключение с `StateFlow<ConnectionConfig>` на `StateFlow<AppConfig>`
- Остальные: только читают `ConnectionConfig` через активный сервер — не меняются
