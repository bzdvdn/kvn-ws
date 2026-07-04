---
report_type: inspect
slug: desktop-tray
status: pass
docs_language: ru
generated_at: 2026-07-04
---

# Inspect Report: desktop-tray

## Scope

- snapshot: системный трей + авто-регистрация ярлыков + single-instance guard для kvn-desktop
- artifacts:
  - CONSTITUTION.md
  - specs/active/desktop-tray/spec.md

## Verdict

- status: **pass**

## Errors

- none

## Warnings

- **Технологии в spec:** упомянуты GtkStatusIcon, NSStatusBar, Shell_NotifyIconW, `golang.org/x/sys/windows`, `github.com/adrg/xdg`. Это не нарушение — все библиотеки либо уже в go.mod, либо транзитивные зависимости webview_go. xdg — опционально. Оставляем как есть, т.к. они отражают repo constraints.
- **AC-005 (Windows .lnk):** evidence "ярлыки видны в меню Пуск и на рабочем столе" не специфицирует, **с какими правами** создаются ярлыки (Shell.ADMIN vs текущий пользователь). На Win11 без админ-прав COM ShellLink может не создать ярлык в Public Desktop. Уточнить на плане — возможно, `Shell: 0x07` (ALL_USERS) или замена на `Environment.SpecialFolder.DesktopDirectory`.

## Questions

- none

## Suggestions

- AC-005 стоит дополнить: "Given ... с правами администратора" уже есть, но хорошо бы явно указать на плане DCOM-инициализацию `CoCreateInstance(CLSID_ShellLink)` + `IShellLinkW`.

## Traceability

- AC-001–AC-007 покрывают все заявленные сценарии. Жёстких замечаний к формулировкам нет.
- MVP (P1 = AC-001, AC-002, AC-003, AC-007) выделен явно.
- Single-instance guard (AC-006) отложен в P3 — корректно.

## Next Step

- safe to continue to plan
