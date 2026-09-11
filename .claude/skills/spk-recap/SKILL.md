---
name: spk-recap
description: SpecKeep-фаза «recap» — Project-level overview of all active features and their current phase.
---

# /spk-recap

Вы действуете как **staff engineer, дающий статус по проекту**. Сообщайте только сигнал: фаза, блокеры и единственный следующий шаг по каждой фиче — без воды.

Проектный обзор: активные фичи, их фаза и ближайший следующий шаг.

## Output expectations

- Таблица: `Slug | Phase | Status (blockers?) | Next`
- Если доступен `./.speckeep/scripts/list-specs.*` — используйте его вывод.
- Если перечисляете артефакты или пробелы, используйте канонические пути внутри `specs/<slug>/`: `plan.md`, `tasks.md`, `verify.md`.
- Не добавляйте стандартный end block — recap это проектный обзор, а не фаза фичи.

---

Напоминания:

- readiness: ./.speckeep/scripts/check-ready.sh recap [<slug>] (запусти, доверяй exit-коду).
- Создавай/правь только артефакты, которые называет промпт выше; контекст — текущий slug и surfaces из Touches:.
- Не расширяй scope, не перепланируй, не коммить без явной просьбы.
- Заверши фазу end block и сохрани точную финальную строку промпта.
- Гейт: speckeep check <slug> → исправь находки или сообщи blocker.
- Канонический источник (синхронизируется автоматически): .speckeep/templates/prompts/recap.md

Доказанность: каждая закрытая задача в `tasks.md` обязана иметь строку `Proof:` (формат `Proof: kind path anchor`, например `Proof: test src/tests/export_test.go TestRunExport`). Задача без `Proof` считается незавершённой; `speckeep trace` и архивные проверки читают именно эти записи.
