---
report_type: inspect
slug: kvn-desktop
status: pass
docs_language: ru
generated_at: 2026-07-04
---

# Inspect Report: kvn-desktop

## Scope

- snapshot: Adversarial review of kvn-desktop spec — desktop WebView wrapper for kvn-web
- artifacts:
  - CONSTITUTION.md (via .speckeep/constitution.summary.md)
  - specs/active/kvn-desktop/spec.md

## Verdict

- status: pass

## Errors

- none

## Warnings

- **AC-006**: "понятное сообщение" — заменено на конкретный текст "Служба kvn-web не запущена" + кнопка запуска. ✅ resolved.
- **Вопрос на inspect**: механизм детекции ошибки — решено: WebView ловит onLoadFailed, Go-сторона инжектит HTML с кнопкой через SetHtml().

## Questions

- **Resolved**: все 4 открытых вопроса из spec закрыты (нейминг `kvn-desktop`, UAC embedded manifest, macOS без подписи, закрытие = exit).

- **Вопрос на inspect**: На Linux/macOS — что если kvn-web не доступен на localhost:2311? В AC-006 есть экран ошибки, но не указан механизм: это делает Go-код (проверка `http.Get` перед webview) или сам webview ловит `onLoadFailed`? Желательно уточнить на фазе plan.

## Suggestions

- **Инсталлятор**: стоит рассмотреть флаг `--with-desktop` вместо `--desktop` в install-web.sh, чтобы сохранить обратную совместимость с текущими флагами (на данный момент `--open-browser`, `--no-browser` — разных стилей).
- **Windows reuse**: если kvn-сервер уже занял порт 2311 (другой процесс), второй экземпляр kvn-desktop мог бы просто открыть WebView на него, а не падать. Стоит заложить в реализацию как fallback.
- **Restart button**: архитектурно проще сделать кнопку в тулбаре WebView-окна (Go-сторона), чем модифицировать SPA. Принято на spec-уточнении.

## Traceability

- 12 AC (AC-001 — AC-012) полностью покрывают scope spec
- Все AC тестируемы (Given/When/Then/Evidence)
- @sk-task маркеры будут проставлены на фазе implement (spec эту фазу не требует)

## Next Step

- safe to continue to plan
