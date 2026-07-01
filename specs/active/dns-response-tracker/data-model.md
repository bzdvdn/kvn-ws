# DNS Response Tracker — Модель данных

## Scope

- Связанные `AC-*`: `AC-001`, `AC-003`
- Связанные `DEC-*`: `DEC-001`
- Статус: `no-change`

## No-Change Stub

- Статус: `no-change`
- Причина: фича не добавляет и не меняет persisted entities, value objects, state transitions или contract-relevant payload shapes.
  - `DNSCacheCfg` — runtime config struct в `RoutingCfg`, не сохраняется отдельно, не требует миграции.
  - `Tracker` — in-memory runtime state (`map[netip.Addr]trackedEntry`), не персистится, живёт только в течение сессии клиента.
- Revisit triggers:
  - появляется новое сохраняемое состояние (например, persistent DNS cache между сессиями)
  - API/event payload shape меняется для передачи tracker-данных
