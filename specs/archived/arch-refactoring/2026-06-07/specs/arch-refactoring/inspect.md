---
report_type: inspect
slug: arch-refactoring
status: pass
docs_language: ru
generated_at: 2026-06-07
---

# Inspect Report: arch-refactoring

## Scope

- snapshot: проверка spec на соответствие конституции, полноту AC, отсутствие неоднозначностей/placeholder
- artifacts:
  - CONSTITUTION.md
  - specs/active/arch-refactoring/spec.md

## Verdict

- status: pass

## Errors

- none

## Warnings

- RQ-001: "без выделения более 64KB буфера" — требование на impl detail (размер промежуточного буфера). Не блокирует, но на реализации это может быть сложно гарантировать для всех путей (obfuscated vs plain QUIC). Рекомендуется уточнить при планировании.

## Questions

- все открытые вопросы spec явно обозначены; [RESOLVED] по MaxMessageSize закрыт.

## Suggestions

- AC-005: "нет switch по FrameType внутри wsToTun" — возможно, слишком жёсткий критерий. Если диспатчер останется как короткий switch с выносом логики в методы, это тоже acceptable. Рекомендуется смягчить на уровне задач.

## Traceability

- spec не ссылается на существующие tasks/plan (их нет). AC (1-7) покрывают все RQ. Изменения затрагивают 5 независимых областей (QUIC OOM, StreamConn, dialStream, wsToTun, netlink, config/UI).
- При реализации потребуется 6-8 задач для полного покрытия AC.

## Next Step

- safe to continue to plan
