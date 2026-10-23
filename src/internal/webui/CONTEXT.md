# Context — src/internal/webui

## Purpose

Локальный HTTP-сервер Web UI для клиента KVN: отдаёт встроенную React SPA и REST/SSE API, через которое браузер (или desktop-обёртка) настраивает клиента и управляет подключением туннеля. Работает только на loopback (`127.0.0.1:<port>`, по умолчанию 2311).

## Responsibilities

- Владеет: HTTP-роутингом `net/http`, встроенным фронтендом (`go:embed frontend/dist`), REST-API конфигов/серверов, SSE-стримом логов/статуса/метрик, жизненным циклом `client.Client` (connect/disconnect) и файлом `config.yaml` в `os.UserConfigDir()/kvn/`.
- Не владеет: логикой туннеля (`src/internal/bootstrap/client`), парсингом/типами конфигурации (`src/internal/config`), сбором метрик (`src/internal/metrics/client`), UI-разметкой (это `frontend/src`).

## How it works

- `New(port)` создаёт `AppState`, готовит конфиг-каталог, чистит протухший DNS (`dnsproxy.CleanupStaleDNS`) и регистрирует маршруты на `http.ServeMux`; SPA раздаётся `http.FileServer` поверх `fs.Sub(embed)`, при отсутствии сборки — заглушка HTML.
- `Serve(ctx)` запускает три broadcast-горутины (`broadcastLogs`, `broadcastStatus`, `broadcastMetrics`) и `ListenAndServe`; по `ctx.Done()` делает `Shutdown` с таймаутом 2с.
- `AppState` — единственное разделяемое состояние: статус (`disconnected/connecting/connected/error`), текущий клиент, cancel/done-каналы, `metricCollector`. Fan-out подписчикам идёт через channel-hub (send в `*SubCh`, non-blocking send подписчикам).
- `handleConnect` под `connectMu`: сбрасывает «зависший» клиент (`stopStaleClientLocked`), мержит глобальный конфиг с активным сервером (`mergeConfig`), вешает логгер с `uiLogCore` через `zapcore.NewTee`, запускает `metricclient.Sender` и `cl.Run(ctx)` в горутине; статус публикуется только если клиент всё ещё активен.
- `handleDisconnect` отменяет контекст, force-closes TUN и ждёт завершения (до 3с), затем ставит `disconnected`.
- `handleLogs` — SSE с событиями `status` (сразу при подписке), `log`, `metric`; подписки по логам/статусу/метрикам снимаются через `defer`.
- Конфиг-хендлеры читают/пишут один YAML; при сохранении нормализуют значения (`SetClientDefaults`, `dedupRoutingStrings`), на создании/обновлении сервера подставляют routing-дефолты.
- `handleRefreshSources` резолвит geoip/geosite/url-источники через `routing.NewResolver` в `configDir/.source-cache` и возвращает диапазоны/домены.
- Фронтенд (`frontend/src`) — React 18 + Vite + TypeScript; состояние в `context.tsx` (fetch + `EventSource("/api/logs")`), экраны/компоненты в `App.tsx`, `ServerCards.tsx`, `TabbedForm.tsx`, `LogPanel.tsx`, `TrafficMeter.tsx`.

## Key files

- `src/internal/webui/server.go` — конструктор, `go:embed`, маршруты, `Serve`, `/api/platform`.
- `src/internal/webui/handler_connect.go` — connect/disconnect, `mergeConfig`, `stopStaleClientLocked`.
- `src/internal/webui/handler_config.go` — CRUD серверов, глобальный конфиг, refresh-sources, `writeJSON`.
- `src/internal/webui/handler_logs.go` — SSE `status`/`log`/`metric`.
- `src/internal/webui/state.go` — `AppState`, `Status`, `LogEntry`, broadcast-хубы.
- `src/internal/webui/webui_log_core.go` — `zapcore.Core`, транслирующий все уровни в SSE-лог.
- `src/internal/webui/frontend/` — React SPA (собирается в `dist/`, который встраивается).

## Public contracts

- REST (все под `127.0.0.1`): `GET /api/config`, `PUT /api/config/global`, `GET|POST /api/servers`, `PUT|DELETE /api/servers/{name}`, `POST /api/connect|/api/disconnect`, `GET /api/logs`, `GET /api/platform`, `POST /api/config/refresh-sources`.
- SSE `/api/logs`: `event: status` → `{"status"}`, `event: log` → `LogEntry` (`line`,`level`,`action`,`ip`,`ts`), `event: metric` → `{tx_bytes,rx_bytes,latency_ms,uptime_s,tx_speed,rx_speed,reconnects}`.
- Go API: `webui.New(port) (*Server, error)`, `(*Server).Serve(ctx) error` — единственные экспортируемые символы пакета.
- JSON-модели совпадают с `config.ClientConfig`/`config.ServerEntry`/`config.WebUIConfig` и с `frontend/src/types.ts`.

## Dependencies

- `src/internal/config` (`LoadWebUIConfig`, `SaveWebUIConfig`, `SetClientDefaults`, `DefaultExcludeRanges`).
- `src/internal/bootstrap/client` (`NewFromConfig`, `SetLogger`, `SetMetricCollector`, `Run`, `StopTun`).
- `src/internal/logger`, `src/internal/metrics/client`, `src/internal/routing`, `src/internal/dnsproxy`, `go.uber.org/zap`.

## Consumed by

- `src/cmd/web/main.go` — единственный импортёр пакета (`webui.New` + `Serve`), открывает браузер.
- `src/cmd/desktop/` — не импортирует пакет, а управляет сервисом `kvn-web` (systemd/launchd/Windows service) и открывает WebView на тот же порт 2311.
- `scripts/build-web.sh`, `Dockerfile` — собирают `frontend/dist` до компиляции Go.

## Gotchas

- `//go:embed all:frontend/dist` требует существующей директории с файлами: без `npm run build` в `frontend/` сборка Go падает (dist в `.gitignore`), рантайм-заглушка покрывает только `fs.Sub`-ошибку.
- `uiLogCore` не пишет в консоль/syslog — он только tee-ится с фильтрованным core, поэтому SSE получает все уровни независимо от `log.level`.
- `connectMu` обязателен: `stopStaleClientLocked` предполагает удержание блокировки; порядок «cancel → StopTun → ждать doneCh».
- Статус нельзя ставить из завершающейся горутины напрямую — только через проверку `s.state.Client() == cl`, иначе старый клиент перезапишет новый статус.
- Broadcast-хубы non-blocking: при переполнении канала подписчик молча теряет события (логи буфер 1000, метрики/статус — 100/10).
- Конфиг живёт вне репозитория: `os.UserConfigDir()/kvn/config.yaml`; кэш источников — `os.UserConfigDir()/kvn/.source-cache`.
- TUN-режим проверяется в хендлере: Linux требует root/`CAP_NET_ADMIN`, Windows разрешён, прочие ОС — ошибка; `/api/platform` отдаёт `transparent_supported` (только Linux) и `tun_supported`.
