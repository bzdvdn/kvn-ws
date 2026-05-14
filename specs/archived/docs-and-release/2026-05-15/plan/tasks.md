# Docs & Release — Задачи

## Phase Contract

Inputs: plan.md, plan.digest.md, spec.md.
Outputs: исполнимые задачи с Touches: и покрытием AC.
Stop if: нет — plan полный, spec стабильна.

## Implementation Context

- Цель MVP: пользователь клонирует → `cp -r examples/* . && bash examples/run.sh` → видит `connected` в логах за ≤5 мин
- DEC-004: examples/ — standalone копии (не symlinks), можно скопировать и запустить
- DEC-003: статические shields.io бейджи в README
- DEC-001: Mermaid для диаграмм в architecture.md
- Границы scope: никаких изменений Go-кода (src/), никаких CI/CD пайплайнов
- Proof signals: `docker compose logs client | grep connected`, `git tag -l v1.0.0`, `gh release view v1.0.0`
- Source of truth для config reference: configs/server.yaml, configs/client.yaml

## Surface Map

| Surface | Tasks |
|---------|-------|
| README.md | T1.1, T4.1 |
| CHANGELOG.md | T1.2 |
| examples/docker-compose.yml | T1.3 |
| examples/client.yaml | T1.3 |
| examples/server.yaml | T1.3 |
| examples/run.sh | T1.3 |
| docs/en/quickstart.md | T2.1 |
| docs/en/config.md | T3.1 |
| docs/en/architecture.md | T3.2 |
| docs/ru/quickstart.md | T4.2 |
| docs/ru/config.md | T4.2 |
| docs/ru/architecture.md | T4.2 |
| git tag v1.0.0 | T5.1 |
| GitHub Release v1.0.0 | T5.1 |

## Фаза 1: Bootstrap — README, CHANGELOG, examples/

- [x] **T1.1** Создать README.md с бейджами (Go version, License, Release), описанием проекта (3 строки), 30-сек quick start (3 команды), ссылками на docs/. Touches: README.md
- [x] **T1.2** Создать CHANGELOG.md по формату Keep a Changelog с секцией v1.0.0 и подсекциями Added/Changed/Fixed. Touches: CHANGELOG.md
- [x] **T1.3** Создать examples/ с docker-compose.yml (упрощённая копия корневого), client.yaml, server.yaml, run.sh (openssl gen + docker compose up). Touches: examples/docker-compose.yml, examples/client.yaml, examples/server.yaml, examples/run.sh

## Фаза 2: MVP — quickstart

- [x] **T2.1** Создать docs/en/quickstart.md: prerequisites (Docker, openssl, git), пошаговая инструкция (клон → cp examples → bash run.sh → verify), troubleshooting. Учесть AC-001: verify через `docker compose logs client | grep connected`. Touches: docs/en/quickstart.md

## Фаза 3: Основная EN документация

- [x] **T3.1** Создать docs/en/config.md: документировать каждый ключ из configs/server.yaml и configs/client.yaml (тип, default, описание, пример). Включить таблицу config keys. Touches: docs/en/config.md, configs/server.yaml, configs/client.yaml
- [x] **T3.2** Создать docs/en/architecture.md: Mermaid-диаграмма компонентов, описание data flow (handshake, data transfer), перечень src/internal/* пакетов с ролями. Touches: docs/en/architecture.md

## Фаза 4: Локализация и финализация

- [x] **T4.1** Обновить README.md: добавить полные ссылки на docs/en/*, docs/ru/*, examples/, CHANGELOG.md. Touches: README.md
- [x] **T4.2** Перевести docs/en/quickstart.md, docs/en/config.md, docs/en/architecture.md на русский в docs/ru/. Сохранить структуру разделов и Mermaid-диаграммы. Touches: docs/ru/quickstart.md, docs/ru/config.md, docs/ru/architecture.md

## Фаза 5: Релиз

- [x] **T5.1** Создать git tag v1.0.0 (на текущем HEAD) и GitHub Release с release notes из CHANGELOG.md. Проверить: `git tag -l v1.0.0`, `gh release view v1.0.0`. Touches: git tag, GitHub Release

## Покрытие критериев приемки

- AC-001 (quickstart ≤5 min) → T1.3, T2.1
- AC-002 (config reference) → T3.1
- AC-003 (architecture docs) → T3.2
- AC-004 (ru translation) → T4.2
- AC-005 (examples/) → T1.3
- AC-006 (root README) → T1.1, T4.1
- AC-007 (CHANGELOG) → T1.2
- AC-008 (GitHub Release) → T5.1

## Заметки

- Фазы строго последовательные: T1 → T2 → T3 → T4 → T5
- Внутри Фазы 1 задачи T1.1, T1.2, T1.3 можно выполнять параллельно
- Внутри Фазы 3 задачи T3.1 и T3.2 можно выполнять параллельно
- Ни одна задача не требует изменений Go-кода
- После T2.1 выполнить ручную проверку MVP: `cp -r examples/* . && bash examples/run.sh`
