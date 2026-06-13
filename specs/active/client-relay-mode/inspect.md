---
report_type: inspect
slug: client-relay-mode
status: pass
docs_language: ru
generated_at: 2026-06-13
---

# Inspect Report: client-relay-mode

## Scope

- snapshot: проверка спеки client-relay-mode — добавление `mode: "relay"` в Go-клиент как прозрачного KVN-bridge
- artifacts:
  - CONSTITUTION.md
  - specs/active/client-relay-mode/spec.md

## Verdict

- status: pass
- резюме: спека полна, AC покрыты, конституция не нарушена. Два minor-warning, не блокирующих планирование.

## Errors

- none

## Warnings

- (none — оба warning из первоначального inspect исправлены в spec: RQ-008 уточнён, основной сценарий помечен P1-only)

## Questions

- none

## Suggestions

- **S1 — добавить AC для relay с автогенерированным self-signed cert**: покрыть кейс, когда `relay.tls` не указан, relay стартует с self-signed и логирует WARNING. Сейчас AC-003 предполагает явный cert, а RQ-008 не имеет AC. Можно добавить AC-006 или расширить AC-003 вторым sub-вариантом.
- **S2 — уточнить поведение при ClientHello timeout**: крайний случай «Клиент не шлёт ClientHello» есть, но timeout (30s) не привязан к конкретному RQ или AC. На `plan` стоит добавить константу в data-model.

## Traceability

- tasks ещё не созданы. AC-001..AC-05 покрывают все RQ (кроме RQ-008, см. W1). Каждый AC имеет Given/When/Then + Evidence.

## Next Step

- safe to continue to plan
