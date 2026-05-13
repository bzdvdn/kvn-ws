# Core Tunnel MVP — Модель данных

## Scope

- Статус: `no-change`
- Причина: все данные живут in-memory в runtime структурах (Frame, Session, IPPool). Никаких persisted entities, БД или stateful контрактов. Модель данных будет добавлена в Sprint 3 (BoltDB для IP pool).

## No-Change Stub

- Статус: `no-change`
- Причина: фича не добавляет и не меняет persisted entities, value objects, state transitions или contract-relevant payload shapes
- Revisit triggers:
  - появляется новое сохраняемое состояние (BoltDB/SQLite)
  - API/event payload shape нужно отслеживать
