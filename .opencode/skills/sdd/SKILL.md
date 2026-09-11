---
name: sdd
description: SpecKeep — spec-driven development. Use when the user asks to propose, spec, inspect, plan, decompose, implement, converge, or verify a feature.
---

# SpecKeep / SDD

Цепочка: constitution → spec → [inspect, опционально] → plan → tasks → implement → archive. Verify — опциональный аудит по требованию; propose — one-shot быстрая полоса; converge — быстрый цикл закрытия.

Каждая фаза — независимый skill, вызывается напрямую слэш-командой: `/spk-<phase>`

## Как выполнять фазу

1. Если фаза уже известна — вызови её напрямую: /spk-<фаза> (каждый такой skill самодостаточен, полные инструкции внутри). Этот sdd-skill нужен только когда фаза не очевидна из запроса.
2. Сначала прочитай .speckeep/constitution.summary.md (fallback: CONSTITUTION.md).
3. Branch-first: работай с feature/<slug> (ветку создаёт/переключает только spec/propose).
4. Держи контекст узким: текущий slug + surfaces из Touches:.
5. Запускай readiness-скрипт: ./.speckeep/scripts/check-ready.sh <phase> <slug>, доверяй exit-коду.
6. Каждую фазу завершай end block (Slug / Status / Artifacts / Blockers / Готово к) и сохраняй точную финальную строку промпта.

## Гейты (не пропускать)

- speckeep check <slug> перед завершением фазы.
- speckeep converge <slug> (быстрый цикл) или speckeep guard . (CI) перед закрытием.
- Задача выполнена только со строкой Proof: под её [x] в tasks.md.

## Фазы (каждая — отдельный вызываемый skill)

- `/spk-constitution` — Create or update the project constitution
- `/spk-spec` — Create or update one feature spec
- `/spk-propose` — One-shot: turn an idea into spec + tasks (plan optional) and go straight to implement
- `/spk-inspect` — Inspect one feature for consistency and quality
- `/spk-plan` — Create or update plan artifacts for one feature
- `/spk-tasks` — Create or update tasks for one feature
- `/spk-implement` — Implement one feature from tasks
- `/spk-verify` — Verify one implemented feature package
- `/spk-converge` — Close a feature fast: re-check tasks/proofs, append follow-up tasks, repeat until converged
- `/spk-handoff` — Generate a session handoff document for one feature
- `/spk-challenge` — Adversarial review of a feature spec or plan
- `/spk-scope` — Quick scope boundary check for a feature
- `/spk-glossary` — Create or update the shared domain-language glossary
- `/spk-recap` — Project-level overview of all active features and their current phase
- `/spk-hotfix` — Create emergency fix outside the standard phase chain
- `/spk-repo-map` — Update REPOSITORY_MAP.md navigation index
- `/spk-rollback` — Roll back completed tasks for a feature, returning them to unfinished state

## Ограничения

Запрещено:
- пропускать readiness scripts
- расширять scope / перепланировать во время implement
- отмечать done без observable proof
- делать git commit/push/tag или PR без явной просьбы
- читать весь репозиторий вместо минимального среза
