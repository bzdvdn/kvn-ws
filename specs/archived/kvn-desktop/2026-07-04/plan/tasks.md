# kvn-desktop — Задачи

## Phase Contract

Inputs: plan.md (DEC-001 — DEC-004), data-model.md (no-change).
Outputs: tasks.md с фазами, покрытием AC и Touches.
Stop if: нет.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/cmd/desktop/main.go` | T1.1 |
| `src/cmd/desktop/service.go` | T1.1 |
| `src/cmd/desktop/app_linux.go` | T2.1 |
| `src/cmd/desktop/app_darwin.go` | T2.2 |
| `src/cmd/desktop/app_windows.go` | T2.3 |
| `src/cmd/desktop/error.go` | T3.1 |
| `src/cmd/desktop/restart.go` | T3.2 |
| `src/cmd/desktop/winres/` | T1.2 |
| `scripts/build-web.sh` | T4.1 |
| `scripts/install-web.sh` | T4.2 |
| `scripts/install-web.ps1` | T4.2 |

## Implementation Context

- Цель MVP: один бинарник `kvn-desktop` с WebView на всех платформах, restart-кнопкой, error screen и установкой через install-скрипты
- Инварианты:
  - SPA не трогаем — все desktop-specific features (restart, error) делаем через JS-инъекции и `SetHtml()` со стороны Go
  - webui.Server переиспользуется как есть, без модификаций
  - Платформенный код через build tags (`_linux.go`, `_darwin.go`, `_windows.go`)
- Библтотека: `github.com/webview/webview_go` — `webview.New()`, `Navigate()`, `SetHtml()`, `Eval()`, `Bind()`, `Run()`
- DEC-002: Restart button через `Eval()` + `Bind()`, не нативный toolbar
- DEC-003: Error screen через `SetHtml()` с inline CSS+JS
- DEC-004: Windows UAC через embedded manifest, не runtime elevation
- Запуск: `go build -o bin/kvn-desktop ./src/cmd/desktop`

## Фаза 1: Основа

Цель: скелет проекта — entrypoint, service interface, Windows UAC.

- [x] T1.1 Создать `src/cmd/desktop/main.go` и `service.go` — парсинг флагов (`--port`, `--server`), платформенный dispatch, service interface (Start, Stop, Restart). Touches: `src/cmd/desktop/main.go`, `src/cmd/desktop/service.go`, `src/cmd/desktop/service_unix.go`, `src/cmd/desktop/service_windows.go`, `src/cmd/desktop/util.go`
- [x] T1.2 Создать Windows UAC manifest + ресурсы — `src/cmd/desktop/winres/kvn-desktop.exe.manifest` с `level="requireAdministrator"`, + `build.bat`. Touches: `src/cmd/desktop/winres/`

## Фаза 2: MVP — WebView на всех платформах

Цель: WebView окно с UI kvn-web на всех трёх платформах.

- [x] T2.1 Реализовать `app_linux.go` — webview_go init, Navigate(`http://localhost:2311`), error detection (http.Get перед WebView). Touches: `src/cmd/desktop/app_linux.go`, `src/cmd/desktop/main.go`
- [x] T2.2 Реализовать `app_darwin.go` — то же что T2.1, build tag darwin. Touches: `src/cmd/desktop/app_darwin.go`
- [x] T2.3 Реализовать `app_windows.go` — `webui.New(port)` → `webview.Open()`, defer shutdown + hook server restart/stop. Touches: `src/cmd/desktop/app_windows.go`, `src/cmd/desktop/winres/`, `src/cmd/desktop/service_windows.go`

## Фаза 3: Error handling + Restart

Цель: экран ошибки с запуском сервиса, restart-кнопка через JS-инъекцию.

- [x] T3.1 Реализовать error screen — `SetHtml()` с inline страницей "Служба kvn-web не запущена" + кнопка "Запустить". По клику — вызов `service.Start()` (Linux: pkexec systemctl start, macOS: osascript launchctl, Windows: N/A). После успеха — `Navigate()` reload. Touches: `src/cmd/desktop/error.go`, `src/cmd/desktop/app_*.go`, `src/cmd/desktop/service.go`
- [x] T3.2 Реализовать restart button — `Eval()` JS-инъекция floating кнопки "Restart" после загрузки SPA, `Bind()` Go-функции `service.Restart()`. Touches: `src/cmd/desktop/restart.go`, `src/cmd/desktop/app_*.go`, `src/cmd/desktop/service.go`

## Фаза 4: Проверка и дистрибуция

Цель: build-скрипт, install-скрипты, trace-маркеры, ручная верификация.

- [x] T4.1 Обновить `scripts/build-web.sh` — добавить сборку `bin/kvn-desktop` через `go build -o bin/kvn-desktop ./src/cmd/desktop`. Touches: `scripts/build-web.sh`
- [x] T4.2 Обновить install-скрипты — `install-web.sh --desktop` (копирует бинарник + .desktop файл / .app bundle) и `install-web.ps1 -Desktop`. Touches: `scripts/install-web.sh`, `scripts/install-web.ps1`
- [x] T4.3 Проставить `@sk-task` / `@sk-test` trace-маркеры в новом коде. Touches: `src/cmd/desktop/*.go`, `scripts/install-web.sh`, `scripts/install-web.ps1`

## Покрытие критериев приемки

- AC-001 -> T1.1, T2.1, T3.1, T4.1
- AC-002 -> T1.1, T2.2, T3.1, T4.1
- AC-003 -> T1.1, T1.2, T2.3, T4.1
- AC-004 -> T2.3
- AC-005 -> T4.1
- AC-006 -> T3.1
- AC-007 -> T4.2
- AC-008 -> T2.1, T2.2, T2.3
- AC-009 -> T3.2
- AC-010 -> T3.2
- AC-011 -> T3.2
- AC-012 -> T3.1

## Заметки

- Все AC покрыты одной или более задачами
- T2.1, T2.2, T2.3 — полностью независимы, можно делать параллельно
- T3.1 зависит от T2.* (нужен работающий WebView чтобы увидеть error)
- T3.2 зависит от T2.* (нужна загруженная SPA для инъекции)
- T4.1/T4.2 не зависят от T3.* — можно делать сразу после T2.*
- Фаза 4 может выполняться частично параллельно с Фазой 3
