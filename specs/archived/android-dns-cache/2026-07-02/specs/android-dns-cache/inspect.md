---
report_type: inspect
slug: android-dns-cache
status: pass
docs_language: ru
generated_at: 2026-07-01
---

# Inspect Report: android-dns-cache

## Scope

- snapshot: Inspect DNS Cache + Exclude Domains + WS: EOF Reconnect spec for Android client
- artifacts:
  - CONSTITUTION.md
  - specs/active/android-dns-cache/spec.md

## Verdict

- status: pass — все ошибки (E1–E4) и предупреждения (W1–W3) исправлены в spec.md.

## Errors

- ~~E1~~ [FIXED] — дублирующиеся секции удалены, AC-008/009/009 объединены в единый блок.
- ~~E2~~ [FIXED] — AC-005 Evidence заменён на проверяемые сигналы (возврат `send()`, отсутствие исключений).
- ~~E3~~ [FIXED] — AC-008 (data-packet routing) удалён как дублирующий AC-006.
- ~~E4~~ [FIXED] — AC-009 (бывший AC-010) Evidence заменён на прямой сигнал (`ConnectionConfig.deserialize()`).

## Warnings

- ~~W1~~ [FIXED] — AC-003: `closeTun()` заменён на поведенческое описание.
- ~~W2~~ [FIXED] — Основной сценарий п.8 исправлен: route exclusion для всех протоколов, data-plane routing убран.
- ~~W3~~ [FIXED] — RQ-013 добавлен для явного маппинга snake_case ↔ camelCase.

## Questions

- Q1 [RESOLVED] — Основной сценарий п.8 исправлен: route exclusion обрабатывает все протоколы, data-plane routing не нужен.

## Suggestions

- S1 [ACCEPTED] — MVP Slice остаётся «AC-003 + AC-006» как минимальный срез; AC-008 (toggle) и AC-009 (config propagation) добавлены в Scope.
- S2 [DISMISSED] — route exclusion на уровне ядра покрывает все протоколы, отдельный data-plane routing не нужен.

## Changes applied to spec.md

- Удалены дублирующиеся блоки «Допущения» и «Критерии успеха»
- Удалён AC-008 (data-packet, дубль AC-006)
- AC-009 → AC-008 (toggle), AC-010 → AC-009 (config propagation)
- AC-005 evidence заменён на `send()` return value + exception check
- AC-009 evidence заменён на `ConnectionConfig.deserialize()`
- AC-003: `closeTun()` → поведенческое описание
- П.8 сценария: route exclusion для всех протоколов
- Добавлен RQ-013 (config mapping snake_case↔camelCase)
- Добавлен RQ-014 (UI toggle) + Scope
- Open Questions обновлены (toggle решён, logging решён)

## Traceability

- spec.md содержит 9 AC (AC-001 — AC-009), 14 RQ (RQ-001 — RQ-014).
- AC-003, AC-005 → WS: EOF reconnect (RQ-007, RQ-006)
- AC-001, AC-002, AC-004 → DNS cache (RQ-008, RQ-009, RQ-010)
- AC-006, AC-007 → exclude domains (RQ-003, RQ-004, RQ-005)
- AC-008 → toggle (RQ-001, RQ-002, RQ-014)
- AC-009 → config propagation (RQ-011, RQ-012, RQ-013)
- Покрытие полное, дублирующие AC удалены.

## Next Step

- Все ошибки и предупреждения исправлены.
- Spec готова к планированию.
