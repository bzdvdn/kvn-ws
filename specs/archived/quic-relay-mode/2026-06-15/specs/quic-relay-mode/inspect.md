---
report_type: inspect
slug: quic-relay-mode
status: pass
docs_language: ru
generated_at: 2026-06-13
---

# Inspect Report: quic-relay-mode

## Scope

- snapshot: проверка спеки QUIC relay — добавление QUIC listener (UDP) параллельно с WS (TCP/TLS) в relay mode, transparent bridge, client fallback
- artifacts:
  - CONSTITUTION.md
  - specs/active/quic-relay-mode/spec.md

## Verdict

- status: pass
- резюме: спека полна, AC-* корректны, scope изолирован, конституция не нарушена

## Errors

- none

## Warnings

- **W1 — RQ-002 vs resolved question**: RQ-002 упоминает `transport: quic` как альтернативу блоку `relay.quic`, но открытый вопрос №1 resolved в пользу только `relay.quic`. Рекомендуется убрать `transport: quic` из RQ-002 на фазе plan, чтобы избежать неоднозначности конфига.

## Questions

- none

## Suggestions

- **S1 — AC-003 evidence**: `ss -tlnp | grep 443` + `ss -ulnp | grep 443` — в Docker-контейнере без root `ss` может не показывать процессы. Рекомендуется добавить альтернативное evidence (лог relay `quic listening on`).
- **S2 — разбить RQ-006**: RQ-006 покрывает 4 действия (read hello → dial → forward → bridge). На фазе tasks стоит разбить на отдельные подзадачи для лучшей traceability.

## Traceability

- 5 AC покрывают 11 RQ
- AC-001 -> RQ-001, RQ-002, RQ-003, RQ-005 (оба транспорта + конкурентность)
- AC-002 -> RQ-007, RQ-008 (data bridge + disconnect)
- AC-003 -> RQ-001, RQ-002, RQ-009, RQ-010 (конфиг + listener)
- AC-004 -> RQ-007 (opaque bridge)
- AC-005 -> RQ-006 (reject on upstream failure)
- RQ-004 (WS path allowlist) и RQ-011 (запрет нерелевантных подсистем) — AC не привязаны явно, но являются частью общего контракта

## Next Step

- safe to continue to plan

Готово к: /speckeep.plan quic-relay-mode
