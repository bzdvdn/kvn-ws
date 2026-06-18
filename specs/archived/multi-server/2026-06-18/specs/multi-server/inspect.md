---
report_type: inspect
slug: multi-server
status: pass
docs_language: ru
generated_at: 2026-06-18
---

# Inspect Report: multi-server

## Scope

- snapshot: управление несколькими серверами в KVN Web UI (CRUD + переключение + адаптация Import/Export/QR)
- artifacts:
  - CONSTITUTION.md
  - specs/active/multi-server/spec.md

## Verdict

- status: pass

## Errors

- none

## Warnings

- AC-002 ("Почему это важно"): содержит "быстрая смена" — subjective. Исправлено на "переключение между серверами без потери контекста". Сами Given/When/Then конкретны.

## Questions

- none

## Suggestions

- При планировании учесть миграцию существующего config.yaml: при отсутствии секции `servers` обернуть текущий конфиг в `servers[0]` с именем Default.
- UI-дизайн селектора: рекомендую выпадающий список в хедере рядом со статусом, чтобы не менять текущую двухпанельную структуру (левая панель — настройки, правая — логи).

## Traceability

- AC-001 (Список серверов) → новый API GET /api/servers + UI-селектор
- AC-002 (Переключение) → dirty-флаг + диалог подтверждения + загрузка конфига
- AC-003 (CRUD) → POST/PUT/DELETE /api/servers/<name>
- AC-004 (Import → новый сервер) → адаптация POST /api/servers
- AC-005 (Export/QR для выбранного) → GET /api/servers/active + адаптация Export/QR UI
- AC-006 (Connect по выбранному) → адаптация POST /api/connect

## Next Step

- safe to continue to plan
