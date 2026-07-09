---
status: minor-change
reason: добавлено одно поле в Android ConnectionConfig + одно поле в Go ServerConfig
---

# Data Model: doze-resilience

## Изменения

### Android — ConnectionConfig (`src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt`)

| Поле | Тип | Default | Хранилище |
|------|-----|---------|-----------|
| `keepAwakeEnabled` | `Boolean` | `false` | DataStore (protobuf/preferences) |

- Не требует миграции DataStore для новых полей (DataStore игнорирует неизвестные ключи).
- Константа `companion object` или inline default.

### Go — ServerConfig (`src/internal/config/server.go`)

```yaml
transport:
  ws:
    pong_timeout: 120s   # new, optional, duration format
```

- Добавить `PongTimeout time.Duration` в `TransportWSCfg` с `mapstructure:"pong_timeout"`.
- Zero value → fallback к `DefaultPongTimeout` (константа в websocket.go).

## Без изменений

- Session/Token/ACL/Routing model — без изменений.
- Wire protocol (Frames) — без изменений.
- Android ConnectionConfig serialization (QR-код) — поле `keepAwakeEnabled` добавляется, обратно совместимо с декодингом старых QR (field missing → default false).
