# Transport Factory — Модель данных

## Scope

- Связанные `AC-*`: все
- Связанные `DEC-*`: DEC-001, DEC-002, DEC-003, DEC-004
- Статус: `no-change`

## No-Change Stub

- Статус: `no-change`
- Причина: фича не добавляет и не меняет persisted entities, value objects, state transitions или contract-relevant payload shapes. Все изменения — Go interface types и их реализации, без сериализации, без конфигурационных полей, без новых состояний.
- Revisit triggers:
  - появляется новое сохраняемое состояние (persistence, BoltDB bucket и т.п.)
  - появляются новые инварианты или lifecycle states для существующих сущностей
  - API/event payload shape меняется
