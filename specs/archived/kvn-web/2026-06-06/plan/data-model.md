# KVN Web UI Модель данных

## Scope

- Связанные `AC-*`: `AC-005`
- Связанные `DEC-*`: `DEC-005`
- Статус: `no-change`

## No-Change Stub

- Статус: `no-change`
- Причина: фича не добавляет и не меняет persisted entities. Конфиг — существующий `config.ClientConfig`, сериализованный в YAML. Никаких новых БД, кэшей, очередей.
- Revisit triggers:
  - появляется новое сохраняемое состояние (например, список подключений, presets)
  - меняется формат конфига
  - API/event payload shape нужно отслеживать отдельно
