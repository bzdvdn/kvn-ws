# Docs & Release — Модель данных

## Scope

- Связанные `AC-*`: все AC
- Связанные `DEC-*`: все DEC
- Статус: `no-change`
- Фича не добавляет и не меняет persisted entities, value objects, state transitions или contract-relevant payload shapes — только документация и конфигурация.

## No-Change Stub

- Статус: `no-change`
- Причина: фича не добавляет и не меняет persisted entities, value objects, state transitions или contract-relevant payload shapes
- Revisit triggers:
  - появляется новое сохраняемое состояние
  - появляются новые инварианты или lifecycle states
  - API/event payload shape нужно отслеживать именно здесь
