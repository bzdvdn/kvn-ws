# Context — src/internal/webui

## Purpose

Локальный v2-only Web UI (`kvn-web`): встроенная React SPA + REST/SSE API для управления серверами, конфигом и v2-туннелем. Слушает loopback, по умолчанию `127.0.0.1:2311`.

## Responsibilities

- Владеет: HTTP-роутингом, embedded frontend, REST API конфигов/серверов, SSE логов/статуса/метрик, lifecycle `client.Client` и единым `~/.config/kvn/config.yaml`.
- Не владеет: v2 transport/TUN (это `src/internal/client`), типами конфигурации (`src/internal/config`), React-разметкой (`frontend/src`).

## How it works

- `New(port)` создаёт `AppState`, каталог конфига `0700`, чистит stale DNS и регистрирует `/api/*`; SPA отдаётся из `go:embed all:frontend/dist`.
- `Serve(ctx)` запускает broadcast-горутины логов/статуса/метрик и HTTP-сервер; shutdown по контексту.
- `handleConnect` под `connectMu` освобождает stale TUN, мержит глобальный конфиг с активным сервером, создаёт `config.V2ClientConfig`, включает `uiLogCore` с выбранным уровнем и запускает `client.Run`.
- `state.go` — единственное разделяемое состояние: status, current client, cancel/done, metric hub; подписки non-blocking с потерей событий при переполнении.
- `config_store.go` — чтение/запись единого конфига, миграция `v2/config.yaml`, backup v1 и фильтрация только v2-серверов.
- `handler_config.go` — CRUD серверов, global config, refresh geoip/geosite; `handler_logs.go` — SSE `status|log|metric`.

## Key files

- `server.go` — `New`, embed, маршруты, `Serve`, `/api/platform`.
- `handler_connect.go` — connect/disconnect и `stopStaleClientLocked`.
- `handler_config.go`, `config_store.go` — серверы и unified config.
- `handler_logs.go`, `state.go`, `webui_log_core.go` — SSE и фильтрация логов.
- `frontend/src/` — React/TypeScript UI (`TabbedForm`, `TrafficMeter`, `LogPanel`).

## Public contracts

- REST: `GET /api/config`, `PUT /api/config/global`, `GET|POST /api/servers`, `PUT|DELETE /api/servers/{name}`, `POST /api/connect|/api/disconnect`, `GET /api/logs`, `GET /api/platform`, `POST /api/config/refresh-sources`.
- SSE `/api/logs`: `status`, `log`, `metric` (`tx_bytes`, `rx_bytes`, `latency_ms`, `uptime_s`, `tx_speed`, `rx_speed`, `reconnects`).
- Go API: `webui.New(port)`, `(*Server).Serve(ctx)`.

## Gotchas

- `//go:embed all:frontend/dist` требует собранного frontend; без `npm run build` Go-сборка падает.
- v2 TUN на Linux требует root/`CAP_NET_ADMIN`; на Windows разрешён, на прочих ОС — ошибка.
- v1 config не показывается в UI и сохраняется как `.v1.bak`; старый `v2/config.yaml` импортируется один раз.
- Статус завершающегося клиента не должен перезаписывать новый — сверяйте `s.state.Client() == cl`.
- Логи в UI фильтруются по `log.level`; в консоль/syslog фильтрация не влияет.
