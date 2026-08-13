# Dual WebSocket Channel — Модель данных

## Scope

- Связанные `AC-*`: `AC-001`, `AC-007`
- Связанные `DEC-*`: `DEC-001`, `DEC-003`
- Статус: `no-change`
- Persisted data model (BoltDB allocations, session registry, DNS cache) не меняется. Фича вводит только wire-contract расширение ClientHello (tag-based TLV), не затрагивающее хранение.

## Сущности

- Persisted-сущности (Session, IPPool allocation, токены) — без изменений.

## Связи

- Межсущностных изменений нет: связь «session_id ↔ secondary-подключение» сидит в рантайм-реестре `tunnelSessRefs` на сервере (DEC-002), не в хранилище.

## Производные правила

- Классификация пакета по IP-протоколу (`parseIPProto`) — вычисляемое правило без persistence; source of truth — сам IP-пакет.

## Переходы состояний

- Новое рантайм-состояние «secondary attached» живёт в `tunnel.Session`/реестре; persisted lifecycle (session TTL/reclaim, `SessionManager`) не расширяется.

## Вне scope

- `@sk-task`-лог «secondary attached» и счётчик — аудит, не data model.
- Wire-контракт ClientHello — документируется в `plan.md` → «Данные и контракты», не в data model.

## No-Change Stub

- Статус: `no-change`
- Причина: фича не добавляет и не меняет persisted entities, value objects, state transitions или contract-relevant persisted payload shapes; расширяется только wire-контракт handshake, который хранится в `protocol/handshake.yaml`, а не в data model.
- Revisit triggers:
  - появляется новое сохраняемое состояние (напр., постоянное хранение secondary-привязки между реконнектами)
  - появляются новые инварианты или lifecycle states в persistence
  - API/event payload shape для рукопожатия нужно отслеживать именно в data model