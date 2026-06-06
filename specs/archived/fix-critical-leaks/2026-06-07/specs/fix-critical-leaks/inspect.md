---
report_type: inspect
slug: fix-critical-leaks
status: concerns
docs_language: ru
generated_at: 2026-06-06
---

# Inspect Report: fix-critical-leaks

## Scope

- snapshot: deep inspection of spec for fixing goroutine leaks, context leaks, deadlock risk, BoltDB timeout, swallowed errors, type assertions, duplicate code, and missing sync.Pool
- artifacts:
  - CONSTITUTION.md
  - specs/active/fix-critical-leaks/spec.md

## Verdict

- status: concerns

## Errors

- none

## Warnings

1. **AC-003 Evidence imprecision** — `pprof.Lookup("goroutine")` показывает goroutine stack frames (function names), а не имена файлов. "0 goroutines из proxy.go" нельзя проверить через pprof напрямую. Используйте `runtime.Stack(buf, true)` + `strings.Contains(buf, "proxy.go")`.
2. **AC-008 Evidence over-engineered** — "собран GC" избыточно для доказательства; достаточно `timer.Stop() == true` + отсутствие timer-горутин в `runtime.NumGoroutine`. Упростите evidence.
3. **Плотность scope** — spec охватывает ~20 source-файлов и 10 разных категорий правок. Это не ошибка (все категории — resource safety), но на implement/plan нужно строго следить, чтобы не размылись границы.

## Questions

- none beyond the Open Questions already listed in spec (3 items)

## Suggestions

1. AC-003: заменить evidence на `runtime.Stack` для точности.
2. AC-008: убрать "GC trace", достаточно `timer.Stop()`.
3. AC-009: рассмотреть разделение на два AC (json encode + frame encode) для лучшей traceability, но текущий вариант допускается.
4. Open Question #2 (WebUI broadcast goroutines) — уточнить на plan фазе, это не блокирует spec.

## Traceability

spec содержит 13 AC (AC-001..AC-013) и 13 RQ (RQ-001..RQ-013), все имеют Given/When/Then с observable evidence. AC перекрывают RQ. Plan/Tasks пока не созданы.

## Next Step

- safe to continue to plan, address Warning #1 (AC-003 evidence) during tasks phase
