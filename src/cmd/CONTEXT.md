# Context — src/cmd

## Purpose

Все исполняемые entrypoints проекта. Каждый `main.go` — тонкая обёртка: разобрать флаги, сконструировать пакет из `src/internal/...`, вызвать `Run/Serve` и залогировать результат. Бизнес-логика живёт в соответствующих пакетах.

## Responsibilities

- Владеет: именами и флагами бинарей, process-level wiring (сигналы/выход), desktop-обёрткой WebView и gate-утилитами.
- Не владеет: конфигом, туннелем, транспортом, Web UI, серверной/relay-логикой — всё делегируется в `internal/*`.

## How it works

- `client/main.go` → `client.New()` (pflag `--config`, дефолт `configs/client.yaml`) → `Run(ctx)`; SIGTERM/SIGINT обрабатываются внутри `bootstrap/client`.
- `server/main.go` → `server.New(--config, configs/server.yaml)` (открывает TUN/NAT и требует root) → `Run(ctx)`; сигналы внутри `bootstrap/server`, SIGHUP перечитывает конфиг.
- `relay/main.go` → `relay.New()` (pflag `--config`, дефолт `configs/relay.yaml`) → `Run(ctx)`; режим `bridge`/`terminator` из конфига.
- `web/main.go` → `webui.New(--port, 2311)` → `Serve(ctx)`; флаги `--open-browser` (по умолчанию true) и `--no-browser`; браузер открывается только при TTY-выводе и слушает только loopback.
- `desktop/main.go` → флаги `--port` (2311) и `--server`; single-instance guard, опциональный ярлык, `platformRun` (WebView 1280×800). Если `kvn-web` не отвечает — HTML-страница с кнопкой «Запустить службу»; в успешный UI внедряется кнопка «Restart Service». Управление службой: Linux `pkexec systemctl`, macOS `osascript`+`launchctl`, Windows — запуск `kvn-web.exe` рядом с бинарём / `taskkill`.
- `gatetest/main.go` → `--mode routing|loadtest`, `--config`; `routing` прогоняет проверки `RuleSet`, `loadtest` читает `configs/loadtest.yaml` (env `KVN_LOADTEST_*`) и меряет пропускную способность/латентность.
- `stability/main.go` → `stability [seconds]`; крутит `RuleSet.Route` и печатает rps/heap (soak/утечки).
- Desktop-файлы по build-tag: `app_{linux,darwin,windows}.go` (WebView), `service_{unix,windows}.go` (управление службой), `single_{unix,windows}.go`, `shortcut_*`, `error.go`, `restart.go`, `iconsource.go` (`go:embed` иконок).

## Key files

- `src/cmd/client/main.go`, `src/cmd/server/main.go`, `src/cmd/relay/main.go`, `src/cmd/web/main.go` — основные бинари.
- `src/cmd/desktop/main.go` + `app_*.go` + `service_*.go` — нативная обёртка Web UI.
- `src/cmd/gatetest/main.go` — routing/load gate.
- `src/cmd/stability/main.go` — soak-тест маршрутизации.
- `scripts/build.sh` — сборка `bin/client`, `bin/server`, `bin/relay`, `bin/kvn-web` (+ cross-compile).
- `scripts/build-web.sh` — `npm build` фронтенда + `bin/kvn-web` + `bin/kvn-desktop`.

## Public contracts

- CLI: `client --config`; `server --config`; `relay --config`; `web --port --open-browser --no-browser`; `desktop --port --server`; `gatetest --mode --config`; `stability [seconds]`.
- Имена артефактов: `bin/client`, `bin/server`, `bin/relay`, `bin/kvn-web`, `bin/kvn-desktop`; Windows-варианты с `.exe`.
- Поведение: ошибка конструктора → `log.Fatalf` (exit 1); ошибка во время `Run/Serve` → `log.Printf` и exit 0 (важно для systemd/супервизоров).

## Dependencies

- `src/internal/bootstrap/{client,server,relay}`, `src/internal/webui`, `src/internal/{config,routing}`, `github.com/webview/webview_go` (desktop, CGo), `spf13/{pflag,viper}` (client/relay/gatetest).

## Consumed by

- `scripts/build.sh`, `scripts/build-web.sh`, `Dockerfile`, `scripts/install-web.{sh,ps1}`, `scripts/install-client.*`, `scripts/install-server.sh`, systemd unit `kvn-web.service`, launchd plist и Windows scheduled task — все запускают эти бинари.

## Gotchas

- `client.New()` и `relay.New()` сами вызывают `pflag.Parse()` внутри конструктора — флаги, разобранные в `main`, уже не добавятся; для клиента доступен только `--config`.
- `server`/`relay`/`client` после успешного старта при ошибке завершаются с кодом 0 — systemd `Restart=on-failure` такую ошибку не увидит.
- `server.New` открывает TUN и настраивает NAT до запуска листенеров: без root бинарь падает на старте (exit 1), а не в рантайме.
- `web` слушает только `127.0.0.1`; `--no-browser` нужен для daemon/systemd, иначе `--open-browser` попытается запустить `xdg-open`/`open`/`rundll32`.
- Desktop `Stop` на Windows убивает **все** процессы `kvn-web.exe` по имени; на Linux/macOS операции идут через `pkexec`/`osascript` и требуют подтверждения пользователя.
- Desktop single-instance: Unix — `/tmp/kvn-desktop.pid` + `flock`, при уже запущенном экземпляре просто `exit 0`; Windows — именованный мьютекс.
- `desktop` требует CGo-зависимостей (GTK/WebKitGTK, WKWebView, WebView2); `build-web.sh` при их отсутствии только предупреждает и не собирает `kvn-desktop`.
- `gatetest`/`stability` — тестовые утилиты, не для production; они работают напрямую с `routing` и не поднимают сервер.
- Протокол генерируется `scripts/generate-protocol.sh` перед сборкой (Go и Kotlin) — ручные правки `*_gen.go`/`Frames.kt` потеряются.
