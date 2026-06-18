---
report_type: inspect
slug: geoip-geosite-integration
status: concerns
docs_language: ru
generated_at: 2026-06-18
---

# Inspect Report: geoip-geosite-integration

## Scope

- snapshot: проверка спеки на добавление GeoIP/GeoSite/external URL источников правил роутинга с резолвом на старте и refresh-кнопкой
- artifacts:
  - CONSTITUTION.md
  - .speckeep/constitution.summary.md
  - specs/active/geoip-geosite-integration/spec.md

## Verdict

- status: concerns

## Errors

- none

## Warnings

1. **«Вне scope» не синхронизирован с refinement.** Спека говорит "Hot-reload источников без перезапуска клиента" и "UI для управления источниками в Web UI или Android" — вне scope. Но refinement (раздел «Принятые решения») и AC-011/RQ-013/RQ-015 прямо описывают refresh-кнопку и структурированный UI. Это не ошибка — refinement имеет приоритет, — но раздел «Вне scope» нужно поправить перед plan, чтобы не вводить в заблуждение.

2. **AC-011 расплывчат.** "обновляет маршрутизацию в рантайме" и "старые соединения не рвутся" — нет specification, КАК именно обновляется роутинг. Пересоздаётся `RuleSet`? Подменяется ссылка в `TunRouter`? Без этого implement-фаза будет гадать. Требуется уточнение в плане или в AC.

3. **SC-002 не измерим.** "не более +30s" — это не success criterion, это грубая оценка. Зависит от скорости сети и размера базы. Либо убрать, либо заменить на что-то контролируемое (напр. "таймаут скачивания не более 30s").

## Questions

- `geoip: "private"` всё ещё открыт. Built-in alias для частных диапазонов — тривиальное дополнение, стоит принять решение до плана, чтобы не переделывать SourceRule.

## Suggestions

- Из «Вне scope» убрать/смягчить про hot-reload и UI, заменив на то, что реально остаётся вне scope: runtime geoip-матчинг, автообновление в фоне, drag-n-drop редактор.
- Добавить в AC-011 уточнение механизма обновления (например: "клиент вызывает `resolver.Refresh()`, который пересоздаёт `RuleSet` и атомарно подменяет ссылку в `TunRouter` / `proxy.Session`").
- SC-002: заменить на "HTTP-клиент для скачивания баз имеет таймаут 30s".

## Traceability

- 15 RQ, 11 AC — все RQ покрыты хотя бы одним AC. Геосайт (RQ-012) покрыт AC-008. Refresh (RQ-015) покрыт AC-011.
- Нет задач — plan ещё не создан.

## Next Step

- safe to continue to plan after minor spec cleanup (Warnings 1-3)
