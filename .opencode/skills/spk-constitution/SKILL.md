---
name: spk-constitution
description: SpecKeep-фаза «constitution» — Create or update the project constitution.
---

# /spk-constitution

Вы действуете как **principal architect**. Превратите реальность проекта в минимальный набор проверяемых правил, которые держат всех агентов и людей в одном контексте.

Вы создаёте или обновляете конституцию проекта.

## Phase Contract

Inputs: запрос пользователя, минимальный контекст репозитория (только то, что нужно для ограничений/архитектуры).
Outputs: `project.constitution_file` (по умолчанию `CONSTITUTION.md`).
Stop if: правила остаются `TBD`/placeholder или конфликтуют с текущим repo reality без явного решения.

## Правила

- Конституция — верхний приоритет: короткие, проверяемые правила; без «философии».
- Укажите: Purpose, принципы, ограничения, tech stack, архитектуру, language policy, workflow.
- Всегда используйте шаблон `.speckeep/templates/constitution.md` как каркас и формат результата. Не ищите «примеры» в чужих конституциях/проектах ради формы: это лишний токен‑расход и дрейф.
- Constitution summary: фаза автоматически загружает `.speckeep/constitution.summary.md` (см. AGENTS.md).
- Запустите readiness script фазы (см. AGENTS.md: Скрипты).

## Output expectations

- Запишите/patch конституцию.
- Сгенерируйте `.speckeep/constitution.summary.md` в строгом компактном формате (только правила, без абзацев рассуждений):
  - `Purpose:` одна строка
  - `Non-negotiables:` 3-6 bullets (`MUST` / `MUST NOT`)
  - `Stack/Architecture:` 2-5 bullets
  - `Workflow/DoD:` 4-7 bullets (traceability, proof-требования, scope discipline и repo map policy)
  - `Branching:` 1-2 bullets (соглашение о нейминге веток, какая фаза создаёт ветки)
  - `Languages:` одна строка (`docs=...`, `agent=...`, `comments=...`)
  - жесткий лимит: ≤200 слов суммарно
- Коротко перечислите ключевые правила и что изменилось.
- Финальная строка: `Готово к: /spk.spec <slug>`

---

Напоминания:

- readiness: ./.speckeep/scripts/check-ready.sh constitution [<slug>] (запусти, доверяй exit-коду).
- Создавай/правь только артефакты, которые называет промпт выше; контекст — текущий slug и surfaces из Touches:.
- Не расширяй scope, не перепланируй, не коммить без явной просьбы.
- Заверши фазу end block и сохрани точную финальную строку промпта.
- Гейт: speckeep check <slug> → исправь находки или сообщи blocker.
- Канонический источник (синхронизируется автоматически): .speckeep/templates/prompts/constitution.md

Доказанность: каждая закрытая задача в `tasks.md` обязана иметь строку `Proof:` (формат `Proof: kind path anchor`, например `Proof: test src/tests/export_test.go TestRunExport`). Задача без `Proof` считается незавершённой; `speckeep trace` и архивные проверки читают именно эти записи.
