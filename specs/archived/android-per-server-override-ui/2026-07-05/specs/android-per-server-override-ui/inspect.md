---
report_type: inspect
slug: android-per-server-override-ui
status: pass
docs_language: ru
generated_at: 2026-07-05
---

# Inspect Report: android-per-server-override-ui

## Scope

- snapshot: per-server app settings overrides (DNS + per-app filtering) with global defaults, duplicate-to-server, bottom navigation (Connect/Settings/Traffic), server card preview, mini traffic panel, full traffic graph
- artifacts:
  - CONSTITUTION.md
  - specs/active/android-per-server-override-ui/spec.md

## Verdict

- status: pass — spec готова к планированию, неоднозначности в пределах нормы

## Errors

- none

## Warnings

1. **AC-007: пропущен `When` шаг** — критерий написан как `Given ... Then` без `When`. По контексту очевидно, что `When` = "открывает Connect tab", но формально структура нарушена. Рекомендуется добавить `When` перед `Then`.
2. **AC-003: evidence размыт** — "визуальный badge на экране" — наблюдаемо для человека, но не автоматизируемо в UI-тесте. Для MVP допустимо.
3. **"быстр" в AC-004, AC-007** — subjective wording в секции "Почему важно". Не влияет на проверяемость AC, т.к. сами Given/When/Then конкретны.

## Questions

- В spec есть "Need?" question в Открытые вопросы про "Clear override" — решение принято (да, добавить). Стоит перенести в основное тело spec.
- Mini traffic panel (AC-006) не привязана к отдельному RQ — покрыта через RQ-006 (Connect tab). OK.

## Suggestions

- Разделить AC-007 на два под-кейса: (a) нет серверов → empty state, (b) есть серверы → карточки. Сейчас описан только case (b), хотя empty state есть в Краевые случаи.
- Для AC-010 (migration) добавить кейс: активный сервер уже имеет override → не перезаписывать.

## Traceability

- RQ-001 → AC-001 | RQ-002 → AC-002 | RQ-003 → AC-003 | RQ-004 → AC-004
- RQ-005 → AC-005 | RQ-006 → AC-006, AC-007 | RQ-007 → AC-008
- RQ-008 → AC-009 | RQ-009 → AC-010
- Все 9 RQ покрыты 10 AC. Tasks пока нет.
- Связь с файлами кода: `AppConfig.kt`, `MainViewModel.kt`, `ConnectScreen.kt`, новый `SettingsScreen.kt`, новый `TrafficScreen.kt`, `MainActivity.kt`
- Все изменения в Android-модуле (Kotlin/Compose) — Go-ядро не затрагивается

## Next Step

- safe to continue to plan
