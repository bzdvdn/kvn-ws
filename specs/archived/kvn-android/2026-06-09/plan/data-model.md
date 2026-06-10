# KVN Android Client — Модель данных

## Scope

- Связанные `AC-*`: `AC-001`, `AC-002`, `AC-003`, `AC-006`
- Связанные `DEC-*`: `DEC-001`, `DEC-002`, `DEC-004`
- Статус: `no-change`

## No-Change Stub

- **Статус:** `no-change`
- **Причина:** Фича не добавляет и не меняет persisted entities, value objects, state transitions или contract-relevant payload shapes на сервере. Локальное состояние Android (сохранённый конфиг в DataStore, runtime state VpnService) является implementation detail UI-слоя, а не моделью данных системы.
- **Revisit triggers:**
  - появляется новое сохраняемое состояние на сервере
  - появляются новые инварианты или lifecycle states в wire protocol
  - API/event payload shape нужно отслеживать именно здесь
