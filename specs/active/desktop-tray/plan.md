# Desktop-tray План

## Phase Contract

Inputs: spec, inspect, repo-surface `src/cmd/desktop/`.
Outputs: plan, data-model stub.
Stop if: нет — spec чёткая, inspect pass.

## Цель

Добавить в `kvn-desktop` три независимые фичи: (1) сворачивание в системный трей при закрытии окна с контекстным меню Show/Hide/Quit; (2) авто-регистрация `.desktop`/`.lnk` при первом запуске; (3) single-instance guard. Все три реализуются новыми платформенными файлами внутри `src/cmd/desktop/` без внешних зависимостей (кроме `golang.org/x/sys/windows`, уже в go.mod).

## MVP Slice

Трей-иконка + сворачивание в трей при закрытии окна + Show/Hide/Quit.
- AC-001, AC-002, AC-003, AC-007, AC-008.

## First Validation Path

1. `go build -o /tmp/kvn-desktop ./src/cmd/desktop && /tmp/kvn-desktop`
2. Открывается окно 900×600.
3. Закрыть окно → иконка в трее (панель задач / status bar).
4. "Show" → окно восстанавливается.
5. "Quit" → процесс завершается, `ps aux | grep kvn-desktop` пуст.
6. `--no-tray` → close=exit (как сейчас).

## Scope

- Новые файлы в `src/cmd/desktop/`: `tray_linux.go`, `tray_darwin.go`, `tray_windows.go`, `tray.go` (shared interface), иконки `icons/` embedded через `//go:embed`.
- Модификация `app_*.go`: интеграция tray-колбэков, замена `w.Run()` на событийную модель.
- Модификация `main.go`: флаг `--no-tray`.
- Новые `shortcut_unix.go`, `shortcut_windows.go` (P2).
- Новый `single.go`, `single_unix.go`, `single_windows.go` (P3).
- `install-web.sh` / `install-web.ps1` — не меняются (фича работает автономно).

## Performance Budget

- SC-001–SC-004 из spec: tray <500ms, прирост ≤100KB, quit <2s, single <200ms.
- `none` критических ограничений — трей не на горячем пути.

## Implementation Surfaces

| Surface | Статус | Роль |
|---------|--------|------|
| `src/cmd/desktop/main.go` | модификация | флаг `--no-tray`, инициализация tray, single-instance check |
| `src/cmd/desktop/app_linux.go` | модификация | интеграция GtkStatusIcon + close→tray |
| `src/cmd/desktop/app_darwin.go` | модификация | интеграция NSStatusBar + close→tray |
| `src/cmd/desktop/app_windows.go` | модификация | интеграция Shell_NotifyIcon + close→tray |
| `src/cmd/desktop/tray.go` | новая | interface `TrayManager` + shared state |
| `src/cmd/desktop/tray_linux.go` | новая | GtkStatusIcon impl |
| `src/cmd/desktop/tray_darwin.go` | новая | CGo NSStatusBar impl (`.mm` companion) |
| `src/cmd/desktop/tray_windows.go` | новая | Shell_NotifyIconW impl |
| `src/cmd/desktop/icons/` | новая | embedded PNG/ICO через `//go:embed` |
| `src/cmd/desktop/shortcut_unix.go` | новая (P2) | .desktop file write |
| `src/cmd/desktop/shortcut_windows.go` | новая (P2) | COM ShellLink |
| `src/cmd/desktop/single_unix.go` | новая (P3) | pidfile + LOCK_EX |
| `src/cmd/desktop/single_windows.go` | новая (P3) | CreateMutexW |

## Bootstrapping Surfaces

- `src/cmd/desktop/` уже существует с 11 файлами. Нужно создать поддиректорию `icons/`.
- Для macOS: `tray_darwin.mm` — Objective-C++ файл рядом с `.go` в том же пакете (CGo).

## Влияние на архитектуру

- Изменение жизненного цикла: `w.Run()` (блокирующий) → событийная модель (window + tray-loop параллельно).
- На Windows: `disconnectClient()` и `proxyState.Restore()` сейчас вызываются после `w.Run()`. С tray они должны вызываться только при "Quit", а не при закрытии окна.
- Никакого влияния на `webui.Server`, `systemproxy`, SPA или другие модули.

## Acceptance Approach

| AC | Подход | Surfaces |
|----|--------|----------|
| AC-001 close→tray | Закрытие окна → hide window + show tray icon. Программа не завершается | `app_*.go`, `tray*.go` |
| AC-002 restore | "Show" в трее → show window. Double-click тоже | `tray*.go` → sync with window |
| AC-003 quit cleanup | "Quit" → disconnect + proxy restore (win) + pidfile remove + exit(0) | `tray*.go`, `app_windows.go` |
| AC-004 .desktop (linux) | firstRun(): write `~/.local/share/applications/kvn-desktop.desktop` | `shortcut_unix.go` |
| AC-005 .lnk (windows) | firstRun(): COM IShellLink → Start Menu + Desktop | `shortcut_windows.go` |
| AC-006 single-instance | second instance: detect pidfile/mutex, focus first, exit | `single*.go`, `main.go` |
| AC-007 --no-tray | flag → old behavior (close=exit, no tray init) | `main.go` |
| AC-008 no-browser | встроенный сервер не вызывает браузер; системный не стартует `--open-browser` | не требуется — уже так |

## Данные и контракты

**Data model**: не меняется. Нет новых структур данных, сериализации, API или событий. `data-model.md` — stub: `status: no-change`.

**Контракты**: не меняются. `kvn-desktop` не имеет внешнего API.

## Стратегия реализации

### DEC-001 Платформенный tray vs единая библиотека

Why: `webview_go` не имеет tray API. Единая кроссплатформенная библиотека (напр., `github.com/getlantern/systray`) добавит ~2MB к бинарнику и внешнюю зависимость с CGo на всех платформах.
Tradeoff: платформенные файлы = больше кода, но 0 новых зависимостей, полный контроль, меньший бинарник.
Affects: `tray_linux.go`, `tray_darwin.go`, `tray_windows.go`, `tray_darwin.mm`.
Validation: бинарник `kvn-desktop` не содержит новых внешних CGo-зависимостей (кроме `-framework Cocoa` на macOS).

### DEC-002 Intercept закрытия окна

Why: webview_go `Run()` блокирует до закрытия окна и затем разрушает window handle. Нужно перехватить событие закрытия до вызова `Destroy()`.
Tradeoff: самый надёжный способ — `SetWindowCloseCallback` (если есть в webview_go API) или hook нативного сигнала `delete-event` (GTK) / `windowShouldClose` (Cocoa) / `WM_CLOSE` (Win32). Если колбэка нет — оборачиваем нативный window handle.
Affects: `app_*.go`, `tray*.go`.
Validation: закрытие окна не завершает процесс — иконка появляется в трее.

### DEC-003 Платформенный lifecycle: tray loop после Run()

Why: после `w.Run()` (возврат) мы не можем показать окно заново (handle разрушен). Решение: вешаем колбэк на закрытие окна, который скрывает окно (не уничтожает), а tray-loop ждёт "Quit" или "Show".
Tradeoff: окно физически существует в памяти, но не отображается. Это нормально для tray-режима.
Affects: все `app_*.go`.
Validation: ps/task manager показывает 1 процесс, окно не отображается, иконка в трее есть.

### DEC-004 Иконки через go:embed

Why: иконки должны быть встроены в бинарник (не внешние файлы). `//go:embed` — стандартный Go-механизм.
Tradeoff: PNG для Linux/macOS конвертировать в GdkPixbuf/NSImage в runtime. ICO для Windows — в HICON через `LoadImage`. Это немного кода конвертации.
Affects: `tray_linux.go` (GdkPixbuf), `tray_darwin.go` (NSImage), `tray_windows.go` (LoadImage).
Validation: иконка отображается в трее.

### DEC-005 PID-based single-instance (Unix) vs Named Mutex (Windows)

Why: pidfile + flock — стандартный Unix-подход. На Windows `CreateMutexW` — нативный механизм, не требует файловой системы.
Tradeoff: pidfile может остаться после краша — требуется cleanup на старте (проверка `/proc/<pid>/status`).
Affects: `single_unix.go`, `single_windows.go`, `main.go`.
Validation: второй экземпляр не стартует, фокусирует первое окно.

## Incremental Delivery

### P1 — MVP: Трей
- AC-001, AC-002, AC-003, AC-007, AC-008
- Файлы: `tray.go`, `tray_linux.go`, `tray_darwin.go`, `tray_darwin.mm`, `tray_windows.go`, `icons/`, модификация `app_*.go`, `main.go`
- Валидация: закрытие окна → икона в трее, Show/Hide/Quit работают, `--no-tray` сохраняет поведение

### P2 — Авто-регистрация shortcut
- AC-004, AC-005
- Файлы: `shortcut_unix.go`, `shortcut_windows.go`, вызов `maybeRegisterShortcut()` на старте в `main.go`
- Валидация: первый запуск на чистой системе → ярлык появляется

### P3 — Single-instance guard
- AC-006
- Файлы: `single_unix.go`, `single_windows.go`, вызов `guardSingleInstance()` на старте в `main.go`
- Валидация: второй экземпляр фокусирует первое окно и завершается

## Порядок реализации

1. **P1 (MVP) — трей:** сначала `tray.go` (interface), затем `tray_windows.go` (самый простой WinAPI), затем `tray_linux.go`, затем `tray_darwin.go`. Модификация `app_*.go` после каждой платформы.
2. **P2 — shortcuts:** независимо от P1, можно параллельно.
3. **P3 — single-instance:** зависит от P1 только в части фокусировки окна (AC-006 требует HWND/window handle). Логика mutex/pidfile независима.

## Риски

- **GtkStatusIcon deprecated** (GTK 3.24+) → может не работать на новых GNOME (Wayland). Mitigation: `--no-tray` fallback, warning в лог, accept that users on pure Wayland/GNOME 45+ отключат трей через флаг.
- **macOS CGo + .mm файл** — сложность сборки, может не скомпилироваться на CI без Xcode CLT. Mitigation: `//go:build darwin && cgo`, заглушка `tray_stub.go` для `!cgo`.
- **webview_go window handle** — может быть недоступен для hide/show. Mitigation: если HWND не извлекается через webview_go API, используем `SetParent` или минимизацию через нативные вызовы к parent window.
- **Shell_NotifyIcon требует HWND** — webview на Windows имеет HWND. Mitigation: `webview.GetWindowHandle()` или поиск через `EnumWindows` по имени класса/заголовку.

## Rollout и compatibility

- Флаг `--no-tray` — поведение по умолчанию не меняется без флага.
- На существующих установках `kvn-desktop` после обновления бинарника трей появляется автоматически.
- Single-instance guard не ломает существующий workflow — только предотвращает дубликаты.
- `install-web.sh/ps1` не меняются — P2 регистрирует shortcut сам на первом запуске.

## Проверка

- Go build + платформенная сборка на CI (уже есть в matrix).
- Ручная проверка на каждой платформе: закрыть окно → трей → Show → Quit.
- `--no-tray`: сборка, закрытие → exit без трея.
- P2: `rm ~/.local/share/applications/kvn-desktop.desktop` → запуск → файл создан.
- P3: `./kvn-desktop & ./kvn-desktop` → первое окно в фокусе, второй exit code 0.

## Соответствие конституции

- нет конфликтов. Все новые файлы в `src/cmd/desktop/`, Go 1.22+, trace-маркеры `@sk-task` над функциями, никакого глобального состояния.
