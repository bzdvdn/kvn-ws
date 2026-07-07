---
report_type: inspect
slug: android-fakedns-routing
status: concerns
docs_language: ru
generated_at: 2026-07-07
---

# Inspect Report: android-fakedns-routing

## Scope

- snapshot: проверка spec фазы fakeDNS domain-based routing для Android-клиента — архитектура, acceptance criteria, scope, консистентность с конституцией.
- artifacts:
  - CONSTITUTION.md
  - specs/active/android-fakedns-routing/spec.md

## Verdict

- status: pass

## Errors

- none

## Warnings

- none

## Questions

- none

## Suggestions

1. **AC-001 evidence — «IP принадлежит 2ip.ru»**: формулировка полагается на внешний DNS и знание конкретных IP. Для автоматизации теста потребуется mock-сервер или wire-level assertion на то, что пакет не ушёл в WebSocket tunnel; consider переформулировать evidence как «пакет с dst IP = реальный IP не отправлен через WS DATA frame, а доставлен через system socket».

## Traceability

- spec содержит 11 RQ (RQ-001 — RQ-011) и 9 AC (AC-001 — AC-009). Все AC имеют Given/When/Then структуру. Placeholders отсутствуют. Scope, Допущения, Открытые вопросы, Вне scope — присутствуют и заполнены.
- plan.md / tasks.md не созданы — проверка plan→tasks не применима.

## Next Step

- safe to continue to plan

Готово к: /speckeep.plan android-fakedns-routing
