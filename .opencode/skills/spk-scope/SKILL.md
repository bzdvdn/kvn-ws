---
name: spk-scope
description: SpecKeep-фаза «scope» — Quick scope boundary check for a feature.
---

# /spk-scope

Вы действуете как **senior engineer на проверке границ**. Будьте максимально конкретны насчёт того, что входит и не входит — неоднозначность здесь оборачивается дрейфом позже.

Быстрая проверка границ: что входит/не входит, где риск scope creep.

**Границы роли:** только инвентаризация границ — перечислите, что входит/не входит и где риск, но **не** пишите правки и не выносите вердикт `pass|concerns|blocked` (это `/spk.inspect`), и не делайте адверсариальный поиск (это `/spk.challenge`).

## Phase Contract

Inputs: `<specs_dir>/<slug>/spec.md` и/или `<specs_dir>/<slug>/plan.md`.
Outputs: отчёт о границах scope.
Stop if: не существует ни spec.md, ни plan.md.

## Разрешение путей

- Определите `<specs_dir>` из `.speckeep/speckeep.yaml` (читать ≤ 1 раза за сессию). Если конфиг отсутствует — используйте `specs/active`.

## Output expectations

- `In scope` (3–7 bullets), `Out of scope` (3–7), `Risks`, `Clarify questions` (≤ 3).
- Добавьте короткий summary block: `Slug`, `Status`, `Artifacts`, `Blockers`, `Готово к` (следующая рекомендованная фаза). Не добавляйте полный end block — это проверка границ, а не фазовый артефакт.

---

Напоминания:

- readiness: ./.speckeep/scripts/check-ready.sh scope [<slug>] (запусти, доверяй exit-коду).
- Создавай/правь только артефакты, которые называет промпт выше; контекст — текущий slug и surfaces из Touches:.
- Не расширяй scope, не перепланируй, не коммить без явной просьбы.
- Заверши фазу end block и сохрани точную финальную строку промпта.
- Гейт: speckeep check <slug> → исправь находки или сообщи blocker.
- Канонический источник (синхронизируется автоматически): .speckeep/templates/prompts/scope.md

Доказанность: каждая закрытая задача в `tasks.md` обязана иметь строку `Proof:` (формат `Proof: kind path anchor`, например `Proof: test src/tests/export_test.go TestRunExport`). Задача без `Proof` считается незавершённой; `speckeep trace` и архивные проверки читают именно эти записи.
