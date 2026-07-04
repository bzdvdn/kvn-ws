# Desktop tray + desktop-file auto-registration

## Scope Snapshot

- In scope: системный трей-иконка для kvn-desktop (сворачивание, контекстное меню, фоновый режим) и авто-регистрация `.desktop`/`.lnk` при первом запуске.
- Out of scope: native notifications, autostart при входе в систему, модификация SPA, Android desktop mode, замена webview-библиотеки.

## Цель

Пользователи kvn-desktop получают возможность сворачивать приложение в системный трей вместо полного закрытия. При первом запуске приложение само регистрирует ярлык в меню приложений/пуске — так что даже ручное копирование бинарника даёт полноценную интеграцию в систему. На Windows это позволяет держать kvn-desktop в фоне с быстрым доступом из трея.

## Основной сценарий

1. Пользователь устанавливает kvn-desktop (через install-скрипт, менеджер пакетов или просто копирует бинарник).
2. При первом запуске приложение:
   - Linux: создаёт `~/.local/share/applications/kvn-desktop.desktop`
   - Windows: создаёт ярлыки в Start Menu и на рабочем столе
    - ~~macOS: проверяет наличие `/Applications/KVN Desktop.app` (если нет — только warn, .app требует установки)~~ macOS tray отложен (см. отдельную спеку)
3. После загрузки UI — стандартное окно 900×600.
4. **При закрытии окна** (крестик) приложение не завершается, а сворачивается в системный трей.
5. В трее — иконка KVN, контекстное меню: "Show", "Hide", "Quit".
6. **Повторный запуск** — single-instance guard: второй процесс находит первый, фокусирует его окно и завершается.
7. Полный выход — только через "Quit" в трее (disconnect + cleanup + exit).

## User Stories

- P1 (MVP): Трей-иконка с контекстным меню Show/Hide/Quit + сворачивание в трей при закрытии.
- P2: Авто-регистрация .desktop/.lnk при первом запуске.
- P3: Single-instance guard (pidfile/mutex).

## MVP Slice

Трей-иконка + сворачивание в трей при закрытии окна + пункты Show/Hide/Quit. Без авто-регистрации (P2) и single-instance (P3).

## First Deployable Outcome

После сборки:
- Запуск → окно
- Закрытие → иконка в трее
- "Show" → окно восстанавливается
- "Quit" → clean exit
- Иконка статуса (connected/disconnected)

## Scope

- Платформенные tray-файлы: `tray_linux.go` (GtkStatusIcon), `tray_windows.go` (Shell_NotifyIcon)
- macOS tray — отложено на отдельную спеку (CGo + Objective-C сборка требует CI с macOS)
- Модификация `app_*.go`: интеграция трея с webview
- Модификация `main.go`: флаг `--no-tray`, инициализация трея
- Хранение tray-иконок: embedded PNG (Linux/macOS) + ICO (Windows) через `//go:embed`
- `shortcut_unix.go`: создание `.desktop`-файла (Linux)
- `shortcut_windows.go`: создание `.lnk` через COM (Windows)
- Single-instance guard: pidfile + `FLOCK` (Unix), именованный мьютекс (Windows)
- Сохранение текущего поведения `close=exit` при флаге `--no-tray`

## Контекст

- `webview_go` не имеет API для трея — пишем платформенные обёртки
- На Linux трей: GtkStatusIcon (идёт через уже установленный GTK3, без новых зависимостей)
- На Windows: Shell_NotifyIconW через `golang.org/x/sys/windows`
- macOS: трей отложен — NSStatusBar через CGo `extern "C" {}` требует отдельной спеки и CI-доступа к macOS runner
- Install-web.sh/ps1 уже создаёт ярлыки, но только при явном `--desktop`. Фича делает это на первом запуске — покрывает случай ручного копирования
- Single-instance: pidfile `/tmp/kvn-desktop.pid` + fcntl `LOCK_EX` (Unix), `CreateMutexW` (Windows)
- Текущее поведение: закрытие окна = `w.Run()` возвращает → `defer w.Destroy()` → exit. Нужно заменить на скрытие окна + отложенный cleanup до "Quit"
- `kvn-web` имеет флаг `--no-browser` (по умолчанию `--open-browser=true`). Systemd/launchd использует `--no-browser`. kvn-desktop (Windows) запускает `webui.Server` программно — браузер не открывается. RQ-008 фиксирует это явно для tray-mode

## Зависимости

- Linux: GtkStatusIcon (из `webview_go` транзитивно через `github.com/webview/webview_go` → GTK3)
- Windows: `golang.org/x/sys/windows` (уже есть в go.mod)
- macOS: ~~CGo + `#import <Cocoa/Cocoa.h>`~~ отложено на отдельную спеку
- `github.com/adrg/xdg` (для `xdg.DataDir` на Linux) — опционально

## Требования

- RQ-001 При закрытии окна kvn-desktop ДОЛЖЕН сворачиваться в системный трей, а не завершаться (кроме `--no-tray`).
- RQ-002 Трей-иконка ДОЛЖНА менять состояние при connect/disconnect (иконка connected / disconnected).
- RQ-003 Контекстное меню трея: "Show" (восстановить окно), "Hide" (свернуть), "Quit" (cleanup + exit).
- RQ-004 "Quit" ДОЛЖЕН выполнять: disconnect, cleanup system proxy (Windows), останов сервера (Windows), удаление pidfile.
- RQ-005 При первом запуске (отсутствие `~/.local/share/applications/kvn-desktop.desktop` или lnk) — создание ярлыка.
- RQ-006 Single-instance guard: повторный запуск фокусирует существующее окно и завершается.
- RQ-007 Флаг `--no-tray` восстанавливает поведение `close=exit` (нужен для Wayland без StatusNotifier или для CI-запусков).
- RQ-008 kvn-desktop НЕ ДОЛЖЕН открывать браузер при запуске встроенного сервера (Windows) или подключении к существующему (Linux/macOS). UI отображается только в нативном WebView-окне.

## Вне scope

- Native notifications / toast.
- Autostart при входе в систему (Startup folder / `~/.config/autostart/`).
- Кастомизация трея через SPA (вся настройка — только контекстное меню).
- Смена webview-библиотеки (остаёмся на `webview_go`).
- Android desktop mode.

## Критерии приемки

### AC-001 Закрытие окна → трей

- **Given** kvn-desktop запущен, UI загружен
- **When** пользователь закрывает окно (ALT+F4 / крестик)
- **Then** окно скрывается, в трее появляется иконка KVN
- Evidence: окно исчезает, иконка видна в системном трее

### AC-002 Восстановление из трея

- **Given** kvn-desktop свёрнут в трей
- **When** пользователь выбирает "Show" в контекстном меню (или двойной клик)
- **Then** окно восстанавливается на прежнюю позицию и размер
- Evidence: окно с UI отображается, приложение активно

### AC-003 Quit с полным cleanup

- **Given** kvn-desktop запущен, клиент подключён
- **When** пользователь выбирает "Quit" в трее
- **Then** происходит disconnect, cleanup system proxy (если был установлен), остановка встроенного сервера (Windows), удаление pidfile, завершение процесса
- Evidence: иконка трея исчезает, процесс не висит, реестр Windows чист от proxy, pidfile удалён

### AC-004 Авто-регистрация .desktop (Linux)

- **Given** kvn-desktop запущен на Linux, `~/.local/share/applications/kvn-desktop.desktop` отсутствует
- **When** first-run проверка
- **Then** создаётся файл `~/.local/share/applications/kvn-desktop.desktop` с правильным Exec-путём
- Evidence: файл существует, `desktop-file-validate` не выдаёт ошибок

### AC-005 Авто-регистрация .lnk (Windows)

- **Given** kvn-desktop запущен на Windows с правами администратора
- **When** first-run проверка
- **Then** создаются ярлыки: Start Menu + Desktop
- Evidence: ярлыки видны в меню Пуск и на рабочем столе

### AC-006 Single-instance guard

- **Given** kvn-desktop уже запущен (окно скрыто или видимо)
- **When** пользователь запускает kvn-desktop снова
- **Then** второй процесс обнаруживает первый, фокусирует его окно (BringWindowToTop / raise), завершается
- Evidence: новое окно не появляется, первое окно выходит на передний план, второй процесс exit code 0

### AC-007 Флаг --no-tray (fallback)

- **Given** kvn-desktop запущен с `--no-tray`
- **When** пользователь закрывает окно
- **Then** приложение завершается полностью (как сейчас)
- Evidence: иконка трея не появляется, процесс завершается

### AC-008 Без браузера при старте

- Почему это важно: UI отображается только в нативном окне, браузер не должен открываться
- **Given** kvn-desktop запущен (на любой платформе), встроенный сервер стартовал (Windows) или подключение к systemd/launchd (Linux/macOS)
- **When** приложение загружает UI
- **Then** браузер по умолчанию НЕ открывается, UI виден только в WebView-окне
- Evidence: браузер не появляется, окно kvn-desktop активно и отображает UI

## Допущения

- На Linux GTK3 уже установлен (требуется `webview_go`)
- GtkStatusIcon работает на X11; на Wayland — fallback на StatusNotifierItem или `--no-tray`
- ~~macOS NSStatusBar доступен всегда (без доп. разрешений)~~ отложено
- На Windows WebView2 Runtime установлен
- Пользователь имеет права на запись в `~/.local/share/applications/` (Linux) или запущен как администратор (Windows для создания lnk)
- Single-instance pidfile в `/tmp/kvn-desktop.pid` (стандартный tmpfs, чистится при reboot)

## Критерии успеха

- SC-001 Иконка трея появляется < 500ms после закрытия окна
- SC-002 Прирост бинарника от tray-кода ≤ 100 KB
- SC-003 "Quit" завершается за < 2s на всех платформах
- SC-004 Single-instance второй экземпляр завершается за < 200ms

## Краевые случаи

- **Нет трея (Wayland без StatusNotifierItem):** fallback на `--no-tray` (close=exit), warning в лог
- **pidfile остался после краша:** при запуске проверяем `/proc/<pid>/cmdline`, если процесс мёртв — удаляем stale pidfile
- **Пользователь удалил .desktop вручную:** при следующем запуске будет создан заново
- **Первый запуск без прав на запись ярлыка:** warn в лог, продолжаем работу
- ~~**macOS:** `.app` bundle не создаём (он требует .app структуру, прав и подписи) — только warn, если отсутствует~~ macOS tray отложен
- **Несколько мониторов:** окно восстанавливается на тот же монитор, где было (WM запоминает)

## Открытые вопросы

- GtkStatusIcon deprecated в GTK 3.24+ в пользу StatusNotifierItem. Насколько это проблема на практике для пользователей KVN? **Решено:** используем GtkStatusIcon, на Wayland без StatusNotifier — `--no-tray` fallback.
- ~~На macOS Objective-C код: выносить в отдельный `.mm` файл с `// #cgo LDFLAGS: -framework Cocoa`? **Решено:** отдельный `.mm` файл в `src/cmd/desktop/`.~~ macOS tray отложен на отдельную спеку — CGo + `extern "C" {}` требует CI-верификации на macOS runner.
- Windows `-H windowsgui` уже скрывает консоль. Трей-иконка не требует доп. модификаций. **Решено:** без изменений.
- Single-instance на Windows: `CreateMutexW` + `EnumWindows` + `SendMessage(WM_SHOW)` или `SetForegroundWindow`. **Отложено** — уточнить HWND от webview_go при реализации.
- ICO для трея: встроить в `.syso` или через `//go:embed` + временный файл? **Решено:** `//go:embed` + HICON через `LoadImage` (Windows), `GdkPixbuf` (Linux). macOS — отложено.
