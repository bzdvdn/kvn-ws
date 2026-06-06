# Архитектурный рефакторинг kvn-ws — Модель данных

## Scope

- Связанные `AC-*`: `AC-001`, `AC-002`, `AC-006`
- Связанные `DEC-*`: `DEC-001`, `DEC-006`
- Статус: `changed`
- Расширение `config.ClientConfig` новыми опциональными полями без ломающих изменений.

## Сущности

### DM-001 ClientConfig (config/client.go)

- Назначение: конфигурация клиента VPN-туннеля; загружается из YAML.
- Источник истины: `~/.config/kvn/config.yaml` на диске; редактируется через Web UI.
- Инварианты: все новые поля опциональны — старые конфиги валидны без изменений.
- Связанные `AC-*`: `AC-001`, `AC-006`
- Связанные `DEC-*`: `DEC-001`
- Новые поля:
  - `MaxMessageSize` — `int`, опциональный, default `10 * 1024 * 1024` (10MB). YAML: `max_message_size`. JSON: `max_message_size`. Определяет макс. размер QUIC-сообщения.
  - `TunnelTimeout` — `time.Duration`, опциональный, default `30s`. YAML: `tunnel_timeout`. JSON: `tunnel_timeout`. Определяет таймаут ожидания данных в туннеле.
  - `ProxyMaxConcurrency` — `int`, опциональный, default `1000`. YAML: `proxy_max_concurrency`. JSON: `proxy_max_concurrency`. Лимит параллельных прокси-соединений.
- Жизненный цикл:
  - Создаётся: `LoadClientConfig()` из файла или `defaultConfig()` при первом запуске.
  - Обновляется: `SaveClientConfig()` через Web UI POST /api/config.
  - Удаляется: только удалением файла.
- Замечания по консистентности:
  - При значении `0` (не задано) — применяется default.
  - Отрицательные значения игнорируются с fallback на default.

### DM-002 MaxMessageSize runtime parameter (internal/transport/quic/)

- Назначение: runtime-параметр, передаваемый в QUIC ReadMessage при dial/listen.
- Источник истины: копия значения из ClientConfig при создании соединения.
- Инварианты: не сохраняется в БД, не передаётся по сети.
- Связанные `AC-*`: `AC-001`, `AC-002`
- Связанные `DEC-*`: `DEC-001`
- Поля:
  - `MaxMessageSize` — `int`, передаётся как аргумент конструктора/параметр.
- Жизненный цикл: живёт в течение соединения; пересоздаётся при reconnect с перечитыванием конфига.

## Связи

- `DM-001 ClientConfig.MaxMessageSize` → `DM-002 MaxMessageSize`: чтение при dial, кэширование на сессию.

## Производные правила

- Если `MaxMessageSize == 0` — применить system default (10MB).
- Если `TunnelTimeout` или `ProxyMaxConcurrency` < 0 — log.Warn + default.

## Переходы состояний

Не применимо — stateless конфиг.

## Вне scope

- ServerConfig — не меняется.
- Admin API модели — не меняются (кроме поля MaxMessageSize в конфиге, которое уже покрыто).
- BoltDB persistence — не меняется.
