# kvn-desktop — План

## Phase Contract

Inputs: spec, inspect (pass), repo context (webui, tun, systemproxy, build scripts).
Outputs: plan, data-model stub.
Stop if: нет.

## Цель

Создать новый entrypoint `src/cmd/desktop/`, который собирает бинарник `kvn-desktop`, открывающий системный WebView с UI kvn-web. На Linux/macOS — viewer для существующего сервиса. На Windows — самодостаточный запуск с UAC и cleanup.

Data model не меняется — webui.Server, AppState, config — всё переиспользуется.

## MVP Slice

Один инкремент: core WebView на всех платформах + обработка ошибки + restart-функция.

AC: 001, 002, 003, 004, 005, 006, 008, 011, 012
(Windows самодостаточность, Linux/macOS viewer, build, error screen, restart button)

Отложено: AC-007 (install scripts), AC-009/AC-010 (restart как отдельная кнопка — входит в MVP как часть toolbar)

## First Validation Path

1. `go build -o /tmp/kvn-desktop ./src/cmd/desktop`
2. На Linux: `./kvn-desktop` → окно открывается, показывает UI
3. `pkill kvn-web` → окно показывает "Служба kvn-web не запущена" + кнопка "Запустить"
4. Нажать "Запустить" → pkexec диалог → сервис стартует → окно перезагружает UI

## Scope

- Новый: `src/cmd/desktop/` — entry point, platform impl, service mgmt
- Изменён: `scripts/build-web.sh` — добавить сборку `kvn-desktop`
- Изменён: `scripts/install-web.sh`, `scripts/install-web.ps1` — добавить `--desktop`
- Новый: `src/cmd/desktop/winres/` — embedded manifest для Windows UAC
- Нетронуто: `src/internal/webui/`, `src/internal/webui/frontend/` — SPA не меняется
- Нетронуто: `src/cmd/web/` — веб-версия остаётся как есть

## Performance Budget

- none. Время загрузки окна ≤ 2s (лимитируется скоростью WebView + сервера, не нашим кодом)
- Размер бинарника < 15 MB (системный WebView, не Chromium)

## Implementation Surfaces

| Surface | Тип | Зачем |
|---|---|---|
| `src/cmd/desktop/main.go` | новая | Entry point: флаги `--port`, `--server`, выбор платформы |
| `src/cmd/desktop/app_linux.go` | новая | WebView → `localhost:2311`, build tag `linux` |
| `src/cmd/desktop/app_darwin.go` | новая | WebView → `localhost:2311`, build tag `darwin` |
| `src/cmd/desktop/app_windows.go` | новая | `webui.Server` + WebView2, build tag `windows`, embedded manifest |
| `src/cmd/desktop/service.go` | новая | platform-agnostic запуск/стоп/рестарт службы (Linux: systemctl, macOS: launchctl, Win: встроенный HTTP stop/start) |
| `src/cmd/desktop/winres/` | новая | `.manifest` + `.rc` для Windows UAC + иконка |
| `scripts/build-web.sh` | изменена | Добавить `go build -o bin/kvn-desktop ./src/cmd/desktop` |

## Bootstrapping Surfaces

- `src/cmd/desktop/` — создать директорию, в ней минимальный `main.go` с разбором флагов

## Влияние на архитектуру

- Нет: `webui.Server` переиспользуется как есть (на Windows)
- Нет: SPA, REST API, SSE — не меняются
- Локально: платформенные build tags (уже есть паттерн в `src/internal/tun/tun.go`)

## Acceptance Approach

- AC-001 (Linux viewer): `app_linux.go` → `webview.Open()` на `localhost:2311`. Проверка: окно открывается, UI загружен.
- AC-002 (macOS viewer): `app_darwin.go` — то же. Проверка: окно, UI загружен.
- AC-003 (Windows self-contained): `app_windows.go` → `webui.New(port)` + `webview.Open()`. Проверка: UAC, окно, UI.
- AC-004 (Windows cleanup): `defer` shutdown server + `systemproxy.Restore()` при закрытии. Проверка: реестр чист.
- AC-005 (build): `scripts/build-web.sh` → `bin/kvn-desktop`. Проверка: файл есть, `file` показывает ELF/Mach-O/PE.
- AC-006 (error screen): `http.Get(localhost:2311)` до webview → fail → `SetHtml()` с ошибкой. Проверка: HTML с сообщением.
- AC-008 (browser): сервер на 2311 — независим от окна. Проверка: curl localhost:2311 в браузере.
- AC-011 (restart button): JS-инъекция floating button через `Eval()` + `Bind()`. Проверка: кнопка видна, клик → restart.
- AC-012 (start service): `service.Start()` через pkexec/osascript. Проверка: диалог пароля, сервис стартует.
- AC-009/AC-010 (restart): `service.Restart()` через pkexec/osascript/kill+start. Проверка: сервис перезапускается.

## Данные и контракты

- Data model: не меняется. Используется существующий `config.WebUIConfig` и `webui.AppState`.
- API: не меняется. WebView потребляет существующие REST/SSE эндпоинты.
- `data-model.md`: stub (no-change).

## Стратегия реализации

### DEC-001 Единый бинарник с платформенными build tags

Why: три разных поведения (Linux viewer, macOS viewer, Windows self-contained) при 90% общего кода (флаги, WebView init, error handling). Build tags — идиоматичный Go-паттерн, уже используется в tun.go, systemproxy/*.
Tradeoff: платформенный код размазан по `_linux.go`, `_darwin.go`, `_windows.go` — но это прозрачнее, чем три отдельных main.go.
Affects: `src/cmd/desktop/app_{linux,darwin,windows}.go`
Validation: `GOOS=linux go build ./src/cmd/desktop` → success; `GOOS=windows go build` → успех; `GOOS=darwin go build` → успех.

### DEC-002 Restart button через JS-инъекцию, а не нативный toolbar

Why: webview_go не предоставляет toolbar API. Native toolbar потребовал бы 3 платформенных реализации (NSToolbar, Win32 toolbar, GTK header bar) без единого API. JS-инъекция через `Eval()` после загрузки страницы — 5 строк кода, работает на всех платформах.
Tradeoff: кнопка отображается внутри WebView-контента, а не в рамке окна. Визуально может быть менее "нативной", но функционально идентична.
Affects: все `app_*.go` через общий код.
Validation: после загрузки SPA на странице видна floating кнопка "Restart", клик вызывает `service.Restart()`.

### DEC-003 Error screen: `SetHtml()` с встроенным CSS+JS, а не отдельный HTML-файл

Why: error screen — статическая страница с одной кнопкой. Отдельный HTML-файл = ещё один asset для embed или хранения. Go string literal достаточно.
Tradeoff: менее гибко для интернационализации. При необходимости — вынести в embed.
Affects: `src/cmd/desktop/error.go` или inline в `app_*.go`.
Validation: при недоступном сервере WebView показывает "Служба kvn-web не запущена" с кнопкой.

### DEC-004 Embedded manifest для Windows UAC, не runtime elevation

Why: runtime elevation (ShellExecute "runas") создаёт второй процесс — сложнее lifecycle. Embedded manifest в `.exe` — один процесс сразу с Admin rights, чище.
Tradeoff: нельзя показать UI до UAC (логин, выбор порта). Для kvn-desktop это ок — нет пред-UAC логики.
Affects: `src/cmd/desktop/winres/kvn-desktop.exe.manifest`, `src/cmd/desktop/winres/rsrc_386.syso`, `src/cmd/desktop/winres/rsrc_amd64.syso`.
Validation: `kvn-desktop.exe` при запуске показывает UAC-щит. После подтверждения — окно с UI.

## Incremental Delivery

### MVP (Первая ценность): core WebView + error handling

- `src/cmd/desktop/main.go` + платформенные `app_*.go`
- `service.go` с `Start()`, `Stop()`, `Restart()` (заглушки для unsupported)
- Error screen + start button
- JL-инъекция restart button
- Build через `scripts/build-web.sh`

AC: 001, 002, 003, 004, 005, 006, 008, 011, 012
Validation: на любой платформе `go build && ./bin/kvn-desktop` показывает окно с UI.

### Итеративное расширение: install scripts

- `install-web.sh --desktop`: копирует `bin/kvn-desktop`, создаёт `.desktop` / `.app` / ярлык
- `install-web.ps1 -Desktop`: то же для Windows

AC: 007
Validation: после установки в меню приложений появляется иконка "KVN Desktop".

## Порядок реализации

1. **Скелет**: `main.go` с флагами, платформенные файлы-заглушки, `service.go` с заглушками
2. **Linux viewer**: `app_linux.go` — webview.Open() на `localhost:2311`
3. **Error handling**: `SetHtml()` при недоступном сервере + service.Start() через pkexec
4. **macOS viewer**: `app_darwin.go` — то же что Linux с launchctl
5. **Windows self-contained**: `app_windows.go` — `webui.New()` + WebView2 + UAC manifest + cleanup
6. **Restart button**: JS-инъекция в общий код
7. **Build script**: обновляем `scripts/build-web.sh`
8. **Install scripts**: добавляем `--desktop` в `install-web.sh/ps1`

Параллельно: 2+6 можно делать вместе. 7+8 можно в любом порядке после 1-6.

## Риски

- **WebView2 не установлен на Windows**: Mitigation: проверка при старте, ссылка на aka.ms/webview2
- **WebKitGTK не установлен на Linux**: Mitigation: проверка при старте, сообщение с командой установки (apt install webkit2gtk-4.1-dev)
- **macOS sandbox .app**: Mitigation: пока без подписи, run from terminal. В плане не предусмотрен .app bundle
- **webview_go library нестабильна**: Mitigation: минимальное API (Navigate, SetHtml, Eval, Bind, Run). Если падает — легко заменить на Tauri или чистый системный WebView.

## Rollout и compatibility

- Специальных rollout-действий не требуется
- `kvn-desktop` — новый бинарник, не заменяет и не конфликтует с `kvn-web`
- `kvn-web` (systemd/launchd) продолжает работать независимо

## Проверка

- Go build: `GOOS=linux GOOS=darwin GOOS=windows` — успешная кросс-компиляция
- Linux manual: запуск `kvn-desktop` → окно, UI, restart, error screen
- Windows manual: запуск `kvn-desktop.exe` → UAC → окно → UI → proxy connect → close → реестр чист
- `go vet ./src/cmd/desktop/...` — без предупреждений

## Соответствие конституции

- нет конфликтов
- Go 1.22+ — соблюдено
- `@sk-task` — проставим на implement phase
- Docker/Android — не затрагиваются
