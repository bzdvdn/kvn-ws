# DNS Setup — Модель данных

## Scope

- Связанные `AC-*`: none
- Связанные `DEC-*`: none
- Статус: `no-change`

## No-Change Stub

- Статус: `no-change`
- Причина: фича не добавляет и не меняет persisted entities, value objects, state transitions или contract-relevant payload shapes. Добавляется только метод `SetDNS([]string) error` в Go interface `TunDevice` — это кодовая абстракция, а не модель данных.
- Revisit triggers:
  - появляется новое сохраняемое состояние (например, persisted DNS backup)
  - появляются новые инварианты или lifecycle states для DNS конфигурации
  - API/event payload shape нужно отслеживать именно здесь
