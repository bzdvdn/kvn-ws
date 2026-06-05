# KVN Web UI План

## Phase Contract

Inputs: spec, inspect report, кодовая база (internal/bootstrap/client, config, cmd/).
Outputs: план, data model (не требуется — повторное использование config.ClientConfig).
Stop if: spec не определяет поверхность React SPA (сборка, embed, API).

## Цель

Добавить команду `kvn-web` — один бинарник с React SPA внутри, который позволяет настраивать и запускать VPN-клиент через браузер. Существующий `kvn-client` не меняется.

## MVP Slice

Одна HTML-страница с React SPA: форма (server, token, mode) + кнопки Connect/Disconnect + live-log через SSE. Конфиг сохраняется в `os.UserConfigDir() + "/kvn/config.yaml"`, клиент запускается внутри процесса.

## First Validation Path

```sh
go build -o bin/kvn-web ./src/cmd/web
./bin/kvn-web &
curl http://127.0.0.1:2311
# -> HTML + React SPA с формой
```

## Scope

- Новый cmd/web/main.go — entrypoint (флаги: --port, --open-browser)
- Новый internal/webui/ — HTTP server, handlers, SSE, embed.FS статики
- React SPA в web/ui/ — сборка через npm, результат встраивается через `//go:embed web/ui/dist`
- API endpoints: GET /api/config, POST /api/config, POST /api/connect, POST /api/disconnect, GET /api/logs (SSE)
- Конфиг: чтение/запись `os.UserConfigDir() + "/kvn/config.yaml"` через config.LoadClientConfig/config.SaveClientConfig
- Graceful shutdown: Ctrl+C останавливает и web, и клиент
- Error handling: ошибки подключения отображаются в UI

## Implementation Surfaces

- src/cmd/web/main.go — новый entrypoint
- src/internal/webui/server.go — HTTP server + routes + embed
- src/internal/webui/handler_config.go — GET/POST /api/config
- src/internal/webui/handler_connect.go — POST /api/connect, POST /api/disconnect
- src/internal/webui/handler_logs.go — SSE /api/logs
- src/internal/webui/state.go — состояние (connected/disconnected), контекст клиента
- src/internal/config/client.go — расширение (SaveConfig функция)
- web/ui/ — React SPA (package.json, ts, компоненты)
- scripts/build-web.sh — сборка React + go build

## Bootstrapping Surfaces

- src/cmd/web/main.go
- src/internal/webui/ (директория + 5 файлов)
- web/ui/ (React проект)

## Влияние на архитектуру

- Новый entrypoint — не затрагивает существующие cmd/client, cmd/server
- internal/bootstrap/client не меняется (только читает config.yaml из UserConfigDir)
- config пакет: добавляется функция SaveConfig (или SaveClientConfig) для записи YAML
- Никаких изменений в протоколе, туннеле, маршрутизации

## Acceptance Approach

- AC-001 -> `go build ./src/cmd/web`, проверить бинарник
- AC-002 -> curl /api/config возвращает JSON, curl / возвращает HTML
- AC-003 -> POST /api/connect + SSE /api/logs показывает connected + логи
- AC-004 -> POST /api/disconnect + статус disconnected
- AC-005 -> POST /api/config + чтение файла YAML
- AC-006 -> POST /api/connect c mode=proxy + проверить SOCKS5
- AC-007 -> сохранить конфиг, перезапустить kvn-web, GET /api/config показывает данные
- AC-008 -> `go build ./src/cmd/client` без изменений

## Данные и контракты

- Конфиг: `config.ClientConfig` — полная перезапись файла при каждом Save. Никакой дифф/merge.
- API контракты (endpoints):
  - `GET /api/config` → `{"server":"...", "auth":{"token":"..."}, "mode":"tun", ...}`
  - `POST /api/config` ← `{"server":"...", "auth":{"token":"..."}, ...}` → `{"status":"ok"}`
  - `POST /api/connect` → `{"status":"connecting"}`
  - `POST /api/disconnect` → `{"status":"disconnected"}`
  - `GET /api/logs` → SSE stream: `event: log\ndata: {"line":"...","level":"info"}\n\n` и `event: status\ndata: {"status":"connected"|"disconnected"|"error"}\n\n`
- Data model не меняется.

## Стратегия реализации

### DEC-001 React SPA через Vite + TypeScript

  Why: минимальный boilerplate, быстрая сборка, хороший DX. Vite — стандарт для новых React-проектов.
  Tradeoff: требуется Node.js на этапе сборки. Runtime — только Go бинарник.
  Affects: web/ui/, scripts/build-web.sh
  Validation: `npm run build` в web/ui/ создаёт dist/ с index.html + js/css

### DEC-002 API-first: React SPA общается с бэкендом через JSON API

  Why: разделение фронтенда и бэкенда. Можно тестировать API через curl без браузера.
  Tradeoff: больше кода (endpoints + fetch в React) против SSR.
  Affects: internal/webui/handler_*.go, React компоненты
  Validation: curl POST /api/connect возвращает JSON

### DEC-003 SSE для логов и статуса

  Why: React SPA получает события в реальном времени без polling. Одно соединение для логов и смены статуса.
  Tradeoff: SSE однонаправленный (сервер→клиент). Для команд (connect/disconnect) — POST.
  Affects: internal/webui/handler_logs.go, React useLogs/useStatus хуки
  Validation: curl -N http://127.0.0.1:2311/api/logs получает поток

### DEC-004 Состояние клиента через context.Context + cancel

  Why: Connect создаёт контекст с cancel, Disconnect дёргает cancel. Никакого глобального состояния.
  Tradeoff: нельзя переподключиться без restart сессии (нужен новый контекст).
  Affects: internal/webui/state.go
  Validation: POST /api/connect запускает Run(ctx), POST /api/disconnect дёргает cancel

### DEC-005 Полная перезапись config.yaml при Save

  Why: простота, конфиг небольшой (~50 строк). Не нужно merge с существующим файлом.
  Tradeoff: теряются комментарии в YAML. Но UI их не генерирует.
  Affects: internal/config/client.go (новая SaveClientConfig)
  Validation: после POST /api/config, config.LoadClientConfig читает валидный YAML

## Incremental Delivery

### MVP (core flow)

- cmd/web/main.go + internal/webui/ (server, state, config handler)
- React SPA: форма + connect/disconnect + SSE лог
- config: SaveClientConfig
- Проверка: AC-001, AC-002, AC-003, AC-004, AC-005

### Итеративное расширение

- Proxy mode в UI (AC-006)
- Auto-open browser (флаг --open-browser)
- Routing, kill-switch, TLS verify в форме (RQ-003, RQ-006)
- Graceful shutdown (Ctrl+C)

## Порядок реализации

1. config.SaveClientConfig —必须先 (нужен для сохранения)
2. internal/webui/ (server, state, handlers) — основа бэкенда
3. cmd/web/main.go — entrypoint
4. web/ui/ (React SPA) — фронтенд
5. scripts/build-web.sh — сборочный скрипт
6. Интеграция: go:embed + сборка бинарника

## Риски

- Риск 1: React build не встроится в Go бинарник (embed path mismatch).
  Mitigation: чёткий путь `//go:embed web/ui/dist/*`, тест после сборки.
- Риск 2: SSE буферизуется nginx/reverse proxy.
  Mitigation: web UI только на localhost, без reverse proxy.
- Риск 3: Конфиг содержит секреты (token, crypto key) на диске.
  Mitigation: уже есть предупреждение в `config.LoadClientConfig`. В UI добавить визуальное предупреждение.

## Rollout и compatibility

- Новый бинарник — не влияет на существующие.
- config.yaml совместим с kvn-client --config.
- scripts/build.sh расширяется поддержкой web.

## Проверка

- `go build ./src/cmd/web` — сборка
- `go build ./src/cmd/client` — регрессия
- `go vet ./src/...` — статический анализ
- `go test -race ./src/...` — тесты
- `./bin/kvn-web & curl http://127.0.0.1:2311/api/config` — smoke test
- Сборка React: `cd web/ui && npm run build`
- Финальная сборка: `go build -o bin/kvn-web ./src/cmd/web`

## Соответствие конституции

- нет конфликтов. Без глобального состояния (state через контекст), Go, Clean Architecture (React SPA — отдельный слой, не смешан с бэкендом).
