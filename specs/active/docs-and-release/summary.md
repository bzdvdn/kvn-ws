## Goal

Создать полную двуязычную документацию (EN/RU), рабочие примеры для docker compose, CHANGELOG, корневой README и опубликовать GitHub Release v1.0.0. Gate — пользователь за 5 минут поднимает сервер через `docker compose up` и подключает клиента следуя `docs/en/quickstart.md`.

## Acceptance Criteria

| ID    | Description | Verification |
|-------|-------------|--------------|
| AC-001 | English quickstart — docker compose up in ≤5 min | Smoke-test: fresh clone → `docker compose up` → client connected |
| AC-002 | English config reference — all keys documented | Every key in configs/*.yamldocumented with type, default, description |
| AC-003 | English architecture docs — system design documented | Diagrams + data flow + component interaction in docs/en/architecture.md |
| AC-004 | Russian translation — docs/ru/ mirrors docs/en/ | Same structure, same sections, Russian language |
| AC-005 | Runnable examples — examples/ with compose + configs + scripts | `examples/` contains independent copies of compose, configs, run.sh |
| AC-006 | Root README — badges, quick start, doc links | README.md at repo root with GitHub badges, 30s quick start, all doc links |
| AC-007 | CHANGELOG — v1.0.0 release entry | CHANGELOG.md with v1.0.0 section: Added/Changed/Fixed |
| AC-008 | GitHub Release — git tag v1.0.0 + release notes | `git tag v1.0.0` + GitHub Release with notes from CHANGELOG |

## Inspect Verdict

- **status:** pass
- **errors:** 0
- **warnings:** 1 (AC-001: уточнить команду верификации для smoke-теста)
- **suggestions:** 3 (уточнить 3 команды в AC-006, формат release notes в AC-008, troubleshooting follow-up)

## Tasks

| Фаза | Задачи | AC |
|------|--------|----|
| 1: Bootstrap | T1.1 README, T1.2 CHANGELOG, T1.3 examples/ | AC-005, AC-006, AC-007 |
| 2: MVP | T2.1 quickstart.md | AC-001 |
| 3: EN docs | T3.1 config.md, T3.2 architecture.md | AC-002, AC-003 |
| 4: RU + финал | T4.1 README update, T4.2 docs/ru/* | AC-004, AC-006 |
| 5: Release | T5.1 tag + GitHub Release | AC-008 |

## Out of Scope

- Любые изменения Go-кода (исходников в src/)
- Исправление багов или добавление feature
- CI/CD пайплайны (кроме самого релиза)
- Документация для сторонних интеграций
