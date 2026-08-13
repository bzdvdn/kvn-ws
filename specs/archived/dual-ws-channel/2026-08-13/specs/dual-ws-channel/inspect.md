---
report_type: inspect
slug: dual-ws-channel
status: concerns
docs_language: ru
generated_at: 2026-08-13
---

# Inspect Report: dual-ws-channel

## Scope

- snapshot: проверка spec на соответствие конституции, полноту/проверяемость AC, scope дисциплину и обратную совместимость протокола.
- artifacts:
  - CONSTITUTION.md (summary)
  - specs/active/dual-ws-channel/spec.md

## Verdict

- status: pass — warnings W-001 и W-002 устранены точечными правками spec (RQ-002, RQ-007/RQ-008, AC-001, AC-006).

## Errors

- none

## Warnings

- none
- (resolved) W-001 app-layer crypto на вторичном канале — зафиксирован в RQ-008 + AC-006.
- (resolved) W-002 связка session_id↔токен — зафиксирована в RQ-002 + AC-001.

## Questions

- none

## Suggestions

- S-001: edge case «вторичный канал упал посреди сессии» — спека описывает фоллбэк на этапе установки (AC-004) и переподключение (Краевые случаи), но лучше явно зафиксировать поведение Session при падении вторичного канала: сессия продолжает жить на первичном, весь трафик (включая UDP) идёт по первичному до реконнекта вторичного. Добавить строку в AC-004 Then.
- S-002: вероятность того, что вторичный канал приходит раньше первичного, низка, а буфер «порядка сотен мс» (RQ-006) не задан числом. Достаточно оставить порядок величины в spec, точное число — в plan. Принято, не блокирует.
- S-003: середина сильная по репо: `Domain mapping` в REPOSITORY_MAP для session/transport уже актуальна; новых top-level директорий фича не создаёт — repo-map не требуется обновлять.

## Traceability

- 8 AC покрывают 10 RQ:
  - AC-001 ↔ RQ-002; AC-002 ↔ RQ-003; AC-003 ↔ RQ-004; AC-004 ↔ RQ-005,
  - AC-005 ↔ RQ-006; AC-006 ↔ RQ-007/008; AC-007 ↔ RQ-001/009; AC-008 ↔ RQ-010.
- Логика cost-факторов подтверждена кодом: единый `session_id` и `Get` в `session/session.go:395`, tag-based декодирование ClientHello, пропускающее неизвестные теги (`handshake.go:83-93`), `parseDestIP` в `demux.go:100-118` — протокольная классификация реализуема без новых фрейм-типов.

## Next Step

- safe to continue to plan — apply W-001 и W-002 (точечные правки RQ/AC) либо в spec до плана, либо принять как входные ограничения в plan.