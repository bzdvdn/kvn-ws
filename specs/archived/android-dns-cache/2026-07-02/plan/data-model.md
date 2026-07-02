# DNS Cache + Exclude Domains + WS: EOF — Модель данных

## Scope

- Связанные AC: AC-008 (toggle), AC-009 (config propagation)
- Связанные DEC: DEC-003 (QR format)
- Статус: `changed`

## Сущности

### DM-001 ConnectionConfig (Android)

- Назначение: полная конфигурация Android-клиента, сериализуется в JSON (DataStore), передаётся через QR-код
- Источник истины: `AppConfigStore` (DataStore on-device), QR-код при импорте
- Инварианты: все поля имеют значения по умолчанию; `ignoreUnknownKeys = true` для обратной совместимости
- Связанные AC-*: AC-008, AC-009
- Связанные DEC-*: DEC-003

Новое поле:

```kotlin
@Serializable
data class ConnectionConfig(
    // ... существующие поля ...
    val dnsCacheEnabled: Boolean = false  // NEW: DNS cache + exclude domains toggle
)
```

- **`dnsCacheEnabled: Boolean`** — default `false` (обратная совместимость)
- Не сохраняется в DataStore если `false` (оптимизация размера)
- Жизненный цикл: создаётся при десериализации QR/JSON, обновляется через UI toggle, применяется при следующем старте VPN

### DM-002 WebRoutingCfg (QR web JSON, Android)

- Назначение: web-совместимая модель для QR-импорта/экспорта из kvn-web
- Источник истины: JSON из QR-кода

Новое поле:

```kotlin
@Serializable
data class WebRoutingCfg(
    // ... существующие поля (exclude_ranges, exclude_domains и т.д.) ...
    val dns_cache_enabled: Boolean? = null  // NEW: snake_case
)
```

- `dns_cache_enabled` — optional (`Boolean?`) для совместимости со старыми QR-кодами
- Маппинг в `ConnectionConfig.dnsCacheEnabled` через `webToAndroidConfig()`

### DM-003 DnsCache (runtime)

- Назначение: in-memory TTL-кэш domain→IP
- Источник истины: in-memory `Mutex`-protected `LinkedHashMap`
- Инварианты: MAX_ENTRIES = 1024, LRU эвикция; TTL min 1s, max 86400s
- Поля:
  - `entries: LinkedHashMap<String, CacheEntry>` — ordered map (access-order for LRU)
  - `CacheEntry(ips: List<InetAddress>, deadline: Instant)`
- Жизненный цикл: создаётся при `doStart()` если `dnsCacheEnabled=true`, no-op иначе; живёт пока жив service scope

### DM-004 DnsTracker (runtime)

- Назначение: in-memory IP→domain reverse map для классификации data-пакетов
- Источник истины: in-memory `Mutex`-protected `HashMap`
- Поля:
  - `entries: HashMap<InetAddress, TrackedEntry>`
  - `TrackedEntry(domain: String, expires: Instant)`
- Жизненный цикл: создаётся вместе с DnsCache, очищается при stop

## Связи

- `DM-001.dnsCacheEnabled` → управляет созданием `DM-003` + `DM-004` (if true → create, if false → no-op)
- `DM-002.dns_cache_enabled` → десериализуется в `DM-001.dnsCacheEnabled`
- `DM-003` (domain→IP) + `DM-004` (IP→domain) — взаимодополняющие кэши

## Производные правила

- При `dnsCacheEnabled=false`:
  - DnsCache и DnsTracker инициализируются как no-op заглушки (null object pattern)
  - `tunReader()` не перехватывает UDP/53
  - `FRAME_TYPE_DNS` forwarding без изменений
  - exclude_domains НЕ pre-resolv-ятся

## Переходы состояний

| Trigger | Source | Target |
|---|---|---|
| User toggles OFF | dnsCacheEnabled=true | dnsCacheEnabled=false |
| User toggles ON | dnsCacheEnabled=false | dnsCacheEnabled=true |
| QR import with field | any | dnsCacheEnabled=fromQR |
| QR import without field | any | dnsCacheEnabled=false (default) |

## Вне scope

- Server-side data model (не меняется)
- Persistent DNS cache between app restarts (in-memory only)
- UI state for toggle (ViewModel StateFlow, не модель данных)
