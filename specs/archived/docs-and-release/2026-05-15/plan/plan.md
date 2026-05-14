# Docs & Release — План

## Phase Contract

Inputs: `specs/active/docs-and-release/spec.md`, `inspect.md` (pass).
Outputs: plan, data model (no-change).
Stop if: нет — spec стабильна, inspect pass.

## Цель

Создать и опубликовать полный комплект документации (EN+RU), примеров, CHANGELOG, README и GitHub Release v1.0.0. Никаких изменений кода — только документация и конфигурационные файлы.

## MVP Slice

AC-001 (quickstart) + AC-005 (examples) + AC-006 (README). После этого пользователь может клонировать, запустить `docker compose up` и получить работающее соединение.

## First Validation Path

```bash
git clone <repo> && cd kvn-ws
cp -r examples/* .
bash examples/run.sh
docker compose logs client | grep connected
```

Если в логах клиента появилось `connected, ip=10.10.0.X` — MVP готов.

## Scope

- Создание файлов документации в `docs/en/` и `docs/ru/`
- Создание файлов примеров в `examples/`
- Создание CHANGELOG.md, README.md
- GitHub Release v1.0.0 (tag + release notes)
- Все изменения строго в перечисленных поверхностях — ни строчки Go-кода

## Implementation Surfaces

| Surface | Тип | Почему |
|---------|-----|--------|
| `README.md` | новая | Корневой entrypoint репозитория на GitHub |
| `docs/en/quickstart.md` | новая | Onboarding: клон → конфиг → запуск за 5 мин |
| `docs/en/config.md` | новая | Config reference — source of truth из configs/*.yaml |
| `docs/en/architecture.md` | новая | Архитектура, компоненты, data flow |
| `docs/ru/quickstart.md` | новая | Перевод docs/en/quickstart.md |
| `docs/ru/config.md` | новая | Перевод docs/en/config.md |
| `docs/ru/architecture.md` | новая | Перевод docs/en/architecture.md |
| `examples/docker-compose.yml` | новая | Standalone docker compose для демо |
| `examples/client.yaml` | новая | Client config для демо |
| `examples/server.yaml` | новая | Server config для демо |
| `examples/run.sh` | новая | Генерация TLS + docker compose up |
| `CHANGELOG.md` | новая | Keep a Changelog формат |

## Bootstrapping Surfaces

`docs/en/` и `docs/ru/` существуют (пусты). `examples/` не существует — создать.

## Влияние на архитектуру

Нет влияния на архитектуру — работа только с файлами документации и конфигурации.

## Acceptance Approach

| AC | Подход | Поверхности | Наблюдение |
|----|--------|-------------|------------|
| AC-001 | Написать quickstart, проверить ручным прогоном `docker compose up` | docs/en/quickstart.md, examples/* | Логи client connected |
| AC-002 | Сопоставить каждый ключ из configs/*.yaml с docs/en/config.md | docs/en/config.md, configs/*.yaml | Чеклист покрытия |
| AC-003 | Написать architecture.md с mermaid-диаграммой | docs/en/architecture.md | Визуальный review |
| AC-004 | Перевести docs/en/ → docs/ru/, сохраняя структуру | docs/ru/* | Сравнение структуры директорий |
| AC-005 | Создать examples/ с независимыми копиями файлов | examples/* | ls examples/ показывает 4+ файла |
| AC-006 | Написать README с бейджами + quickstart + ссылками | README.md | Визуальный review на GitHub |
| AC-007 | Создать CHANGELOG по Keep a Changelog | CHANGELOG.md | Парсинг структуры |
| AC-008 | git tag + GitHub Release с notes из CHANGELOG | git tag, GitHub Release | git tag -l, gh release view |

## Данные и контракты

- Data model не меняется — ни один Go-файл не затрагивается
- data-model.md: no-change stub

## Стратегия реализации

### DEC-001 Mermaid для архитектурных диаграмм
- Why: Нативный рендеринг на GitHub, text-based, version control friendly
- Tradeoff: Ограниченная кастомизация по сравнению с draw.io
- Affects: docs/en/architecture.md, docs/ru/architecture.md
- Validation: mermaid-блок корректно отображается на GitHub

### DEC-002 docs/en/ как primary, docs/ru/ как полный перевод
- Why: Английский — язык кода и максимальная аудитория; русский — для основной аудитории проекта
- Tradeoff: Двойная работа при обновлениях
- Affects: docs/en/*, docs/ru/*
- Validation: Одинаковое количество файлов и структура разделов

### DEC-003 Статические shields.io бейджи
- Why: Не требуют CI/CD пайплайна для работы; можно обновить позже
- Tradeoff: Не отражают реальное состояние (build, coverage)
- Affects: README.md
- Validation: Бейджи отображаются на GitHub

### DEC-004 examples/ — standalone копии, не symlinks
- Why: Пользователь копирует examples/ и запускает без зависимостей от корня репозитория
- Tradeoff: Дублирование с configs/ и корневым docker-compose.yml — нужно синхронизировать
- Affects: examples/*
- Validation: `cp -r examples/* . && docker compose up` работает

### DEC-005 run.sh генерирует самоподписанный TLS-сертификат
- Why: Единственный способ сделать examples самодостаточными без внешних зависимостей
- Tradeoff: openssl как prerequisite; self-signed cert не подходит для production
- Affects: examples/run.sh, examples/server.yaml
- Validation: `bash run.sh` генерирует cert.pem + key.pem и запускает compose

## Incremental Delivery

### MVP (Первая ценность)

1. README.md (базовый) + CHANGELOG.md
2. examples/ (все 4 файла)
3. docs/en/quickstart.md
4. Проверка: ручной прогон `cp -r examples/* . && bash examples/run.sh`
5. AC covered: AC-001, AC-005, AC-006, AC-007

### Итеративное расширение

1. docs/en/config.md — после MVP, AC-002
2. docs/en/architecture.md — после config.md, AC-003
3. docs/ru/* — после стабилизации всех EN-документов, AC-004
4. README.md (финальная версия с полными ссылками) — после всех docs
5. GitHub Release v1.0.0 — последним шагом, AC-008

## Порядок реализации

1. **Шаг 1:** README.md (скелет) + CHANGELOG.md + examples/
2. **Шаг 2:** docs/en/quickstart.md
3. **Шаг 3:** docs/en/config.md
4. **Шаг 4:** docs/en/architecture.md
5. **Шаг 5:** docs/ru/* (все три файла)
6. **Шаг 6:** README.md (финал с полными ссылками)
7. **Шаг 7:** git tag v1.0.0 + GitHub Release

Параллельно: README.md (шаг 1) и CHANGELOG.md можно писать одновременно.
docs/ru/* (шаг 5) можно писать параллельно с EN-документацией, если переводчик/автор один — последовательно.

## Риски

- **Риск 1: Config reference неполный**
  Mitigation: Чеклист всех ключей из configs/*.yaml перед финализацией docs/en/config.md
- **Риск 2: run.sh не работает на macOS (отличия openssl)**
  Mitigation: Протестировать на Linux (таргет-платформа); добавить примечание в quickstart
- **Риск 3: mermaid-диаграмма не соответствует архитектуре (расходится с кодом)**
  Mitigation: Базировать на существующих пакетах src/internal/*, проверить по REPOSITORY_MAP.md
- **Риск 4: Забыли сделать git push после tag**
  Mitigation: Чеклист: tag → push → release → verify

## Rollout и compatibility

Специальных rollout-действий не требуется. Все артефакты — новые файлы, обратная совместимость не нарушается.

## Проверка

- Manual: прогон `cp -r examples/* . && bash examples/run.sh` на чистом Linux
- Manual: проверка отображения README.md и docs на GitHub (mermaid, бейджи, ссылки)
- Manual: чеклист config keys → docs/en/config.md покрытие
- Automated: `git tag -l v1.0.0` и `gh release view v1.0.0` для AC-008

## Соответствие конституции

Нет конфликтов. Конституция требует разделение docs/ru/ и docs/en/ (удовлетворено), Docker multi-stage build (удовлетворено — документировано), traceability (CHANGELOG + Release Notes покрывают).
