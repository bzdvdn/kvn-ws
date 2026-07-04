# Desktop-tray Задачи

## Phase Contract

Inputs: plan.md + surfaces из `src/cmd/desktop/`.
Outputs: исполнимые задачи с покрытием AC.
Stop if: нет — plan конкретный, поверхности известны.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/cmd/desktop/tray.go` | T1.1 |
| `src/cmd/desktop/icons/` (new dir) | T1.2 |
| `src/cmd/desktop/tray_windows.go` | T2.1 |
| `src/cmd/desktop/tray_linux.go` | T2.2 |
| ~~`src/cmd/desktop/tray_darwin.go` + `tray_darwin.mm`~~ | ~~T2.3~~ отложено |
| `src/cmd/desktop/app_linux.go` | T2.4 |
| `src/cmd/desktop/app_darwin.go` | T2.4 (legacy без трея) |
| `src/cmd/desktop/app_windows.go` | T2.4 |
| `src/cmd/desktop/main.go` | T1.3, T2.5, T3.3, T4.3 |
| `src/cmd/desktop/shortcut_unix.go` | T3.1 |
| `src/cmd/desktop/shortcut_windows.go` | T3.2 |
| `src/cmd/desktop/single_unix.go` | T4.1 |
| `src/cmd/desktop/single_windows.go` | T4.2 |

## Implementation Context

- **Цель MVP:** трей-иконка с контекстным меню Show/Hide/Quit + сворачивание в трей при закрытии окна.
- **Инварианты:** закрытие окна ≠ exit (кроме `--no-tray`); Quit = полный cleanup; tray-loop живёт пока не выбран Quit.
- **Lifecycle:** `w.Run()` не используется напрямую — вешаем колбэк на закрытие (GTK `delete-event`, Cocoa `windowShouldClose`, Win32 `WM_CLOSE`). При закрытии → hide window, показываем иконку трея. При "Quit" → destroy tray → destroy window → exit.
- **Cleanup (Windows):** `disconnectClient()` и `proxyState.Restore()` вызываются только при "Quit", а не при закрытии окна.
- **Иконки:** embedded через `//go:embed` → PNG (Linux→GdkPixbuf, macOS→NSImage), ICO (Windows→HICON через `LoadImage`).
- **GtkStatusIcon:** deprecated в GTK 3.24+ — может не работать на Wayland. Fallback: `--no-tray`.
- **macOS CGo:** ~~отдельный `tray_darwin.mm` + build tag `darwin && cgo`. Заглушка `tray_stub.go` для `!cgo`.~~ **отложено на отдельную спеку** — CGo + `extern "C" {}` не собирается на CI без macOS runner
- **DEC:** DEC-001 (платформенный tray), DEC-002 (close interceptor), DEC-003 (lifecycle), DEC-004 (go:embed icons), DEC-005 (pidfile/mutex).
- **Границы scope:** не меняем SPA, install-скрипты, webui.Server, systemproxy; Android не трогаем.
- **Proof signals:** `go build ./src/cmd/desktop` проходит на 3 платформах; закрытие окна → иконка в трее; Show → окно восстановлено; Quit → процесс завершён.

## Фаза 1: Инфраструктура

Цель: подготовить интерфейс трея, иконки и флаг `--no-tray`, на которые будут опираться платформенные реализации.

- [x] T1.1 Создать `tray.go` — interface `TrayManager` с методами `Show()`, `Hide()`, `SetStatus(connected bool)`, `Run()`, `Stop()`, канал `ActionCh chan TrayAction` (Show, Hide, Quit). Touches: `src/cmd/desktop/tray.go`
- [x] T1.2 Создать `icons/` с embedded PNG (connected/disconnected) и ICO (Windows). Файлы: `icons/connected.png`, `icons/disconnected.png`, `icons/kvn.ico`. Использовать `//go:embed`. Touches: `src/cmd/desktop/icons/`
- [x] T1.3 Добавить флаг `--no-tray` (bool, default false) в `main.go`. При `--no-tray` — старый lifecycle (close=exit, без инициализации трея). Touches: `src/cmd/desktop/main.go`

## Фаза 2: MVP — Трей (P1)

Цель: трей-иконка работает на всех трёх платформах, закрытие окна → сворачивание, контекстное меню Show/Hide/Quit.

- [x] T2.1 Реализовать `tray_windows.go` — Shell_NotifyIconW через `golang.org/x/sys/windows`. Создать NOTIFYICONDATAW с HWND окна, контекстное меню через TrackPopupMenu. Иконка connected/disconnected через LoadImage + NIM_MODIFY. Действия: Show (SetForegroundWindow), Hide (ShowWindow(SW_HIDE)), Quit (пост сигнала). Touches: `src/cmd/desktop/tray_windows.go`
- [x] T2.2 Реализовать `tray_linux.go` — GtkStatusIcon из GTK3 (уже есть как транзитивная зависимость). Создать status icon с GdkPixbuf из embedded PNG, контекстное меню через GtkMenu. Show (gtk_window_present), Hide (gtk_widget_hide_on_delete). Touches: `src/cmd/desktop/tray_linux.go`
- [-] T2.3 ~~Реализовать `tray_darwin.go` + `tray_darwin.mm` — NSStatusBar через CGo. Создать NSStatusItem с NSImage (из embedded PNG), NSMenu с пунктами Show/Hide/Quit. Show (NSApplication activateIgnoringOtherApps), Hide (NSWindow orderOut). Touches: `src/cmd/desktop/tray_darwin.go`, `src/cmd/desktop/tray_darwin.mm`~~ **ОТЛОЖЕНО** — macOS tray на отдельную спеку (CGo + `extern "C" {}` требует CI-верификации)
- [x] T2.4 Модифицировать `app_linux.go`, ~~`app_darwin.go`,~~ `app_windows.go` — заменить `w.Run()` + `defer w.Destroy()` на lifecycle: close → hide window + show tray; "Show" → show window; "Quit" → destroy window + stop tray + exit. На Windows: cleanup (disconnect + proxy restore) перенести из post-Run в "Quit"-обработчик. `app_darwin.go` — только legacy (без трея, отложено). Touches: `src/cmd/desktop/app_linux.go`, `src/cmd/desktop/app_darwin.go`, `src/cmd/desktop/app_windows.go`
- [x] T2.5 Интегрировать tray в `main.go`: после `--no-tray` guard, создать TrayManager, запустить через goroutine, передать колбэки show/hide/quit в `platformRun()`. Touches: `src/cmd/desktop/main.go`

## Фаза 3: Shortcuts (P2)

Цель: при первом запуске приложение само регистрирует ярлык в системе.

- [x] T3.1 Реализовать `shortcut_unix.go` — функция `maybeRegisterShortcut()`. Проверить существование `~/.local/share/applications/kvn-desktop.desktop`. Если нет — создать с Exec=`os.Executable()`. Touches: `src/cmd/desktop/shortcut_unix.go`
- [x] T3.2 Реализовать `shortcut_windows.go` — функция `maybeRegisterShortcut()`. Через COM CoCreateInstance(CLSID_ShellLink) + IShellLinkW создать .lnk в `[Environment]::GetFolderPath('StartMenu')` и `[Environment]::GetFolderPath('DesktopDirectory')`. Touches: `src/cmd/desktop/shortcut_windows.go`
- [x] T3.3 Вызвать `maybeRegisterShortcut()` в `main.go` после single-instance guard, перед инициализацией трея. Touches: `src/cmd/desktop/main.go`

## Фаза 4: Single-instance guard (P3)

Цель: предотвратить запуск второго экземпляра kvn-desktop; фокусировать существующее окно.

- [x] T4.1 Реализовать `single_unix.go` — pidfile `/tmp/kvn-desktop.pid` + fcntl LOCK_EX. stale check: если pidfile существует, читать PID, проверить `/proc/<pid>/status`, если мёртв — удалить и занять. Touches: `src/cmd/desktop/single_unix.go`
- [x] T4.2 Реализовать `single_windows.go` — CreateMutexW("Global\\KVN-Desktop-{port}"). Если мьютекс уже есть — найти окно через FindWindow/EnumWindows по заголовку "KVN Desktop" и BringWindowToTop. Touches: `src/cmd/desktop/single_windows.go`
- [x] T4.3 Вызвать `guardSingleInstance()` в `main.go` перед всем остальным. Если второй экземпляр — focus + os.Exit(0). Touches: `src/cmd/desktop/main.go`

## Фаза 5: Проверка

Цель: доказать, что все AC закрыты, сборка проходит на всех платформах.

- [-] T5.1 Проверить сборку на поддерживаемых платформах: `GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build ./src/cmd/desktop` (✅ pass), `GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build ./src/cmd/desktop` (❌ macOS tray отложен — используется legacy-путь webview без трея), `GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build ./src/cmd/desktop` (❌ pre-existing MinGW -mthreads cross-compiler issue). Touches: CI matrix
- [x] T5.2 Ручная валидация MVP: запуск → окно → close → икона в трее → Show → окно восстановлено → Quit → процесс завершён. `--no-tray`: close = exit. Touches: manual
- [x] T5.3 Добавить trace-маркеры `@sk-task` над новыми функциями/методами во всех новых файлах. Touches: все новые файлы
- [x] T5.4 Проверить CHANGELOG.md на наличие записи о desktop-tray фиче. Touches: `CHANGELOG.md`

## Покрытие критериев приемки

- AC-001 -> T2.1, T2.2, T2.3, T2.4, T5.2
- AC-002 -> T2.1, T2.2, T2.3, T2.4, T5.2
- AC-003 -> T2.1, T2.2, T2.3, T2.4, T5.2
- AC-004 -> T3.1, T3.3, T5.2
- AC-005 -> T3.2, T3.3, T5.2
- AC-006 -> T4.1, T4.2, T4.3, T5.2
- AC-007 -> T1.3, T5.2
- AC-008 -> T5.2


## Заметки

- Порядок важен: Фаза 1 → Фаза 2 → Фаза 3 + 4 параллельно → Фаза 5.
- T2.1, T2.2, T2.3 можно реализовывать параллельно (разные платформенные файлы).
- T3.x и T4.x не зависят от T2.x (кроме `main.go`-интеграции T2.5/T3.3/T4.3).
- На implement: начинать с T1.1→T1.2→T1.3, затем T2.1 и/или T2.2, затем T2.4/T2.5.
- Не блокироваться на macOS — заглушка `tray_stub.go` для `!cgo`.
