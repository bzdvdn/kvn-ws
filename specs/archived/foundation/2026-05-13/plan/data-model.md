# Foundation — Модель данных

## Scope

- Связанные `AC-*`: none
- Связанные `DEC-*`: none
- Статус: `no-change`
- Причина: foundation не добавляет и не меняет persisted entities, value objects, state transitions или contract-relevant payload shapes. Все пакеты — заглушки без логики.

## No-Change Stub

- Статус: `no-change`
- Причина: фича не добавляет и не меняет persisted entities, value objects, state transitions или contract-relevant payload shapes
- Revisit triggers:
  - появляется новое сохраняемое состояние (напр. IP pool в BoltDB)
  - появляются новые инварианты или lifecycle states
  - API/event payload shape нужно отслеживать именно здесь
