# KVN Web UI Задачи

## Phase Contract

Inputs: plan, spec, inspect, data-model.
Outputs: упорядоченные задачи с покрытием AC и DEC.
Stop if: задачи overlapping с другими spec (net).

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/config/client.go | T1.1 |
| src/internal/webui/server.go | T1.2 |
| src/internal/webui/state.go | T1.3 |
| src/internal/webui/handler_config.go | T2.1 |
| src/internal/webui/handler_connect.go | T2.2 |
| src/internal/webui/handler_logs.go | T2.3 |
| src/cmd/web/main.go | T2.4 |
| src/internal/webui/frontend/ (React SPA) | T3.1, T3.2, T3.3 |
| scripts/build-web.sh | T4.1 |

## Implementation Context

- Цель MVP: `kvn-web` с формой, connect/disconnect, SSE логом, сохранением конфига
- Границы приемки: AC-001..AC-008
- Ключевые правила: cmd/client не меняется, React SPA встраивается через `embed.FS`, config.yaml совместим с kvn-client
- Инварианты: конфиг — полная перезапись (DEC-005), состояние через context (DEC-004), SSE для логов (DEC-003)
- Контракты: API endpoints (GET/POST /api/config, POST /api/connect, POST /api/disconnect, GET /api/logs SSE)
- Proof signals: `go build ./src/cmd/web && ./bin/kvn-web && curl http://127.0.0.1:2311`
- Вне scope: server dashboard, mobile app, HTTPS для web UI, multi-user

## Фаза 1: Основа

Цель: подготовить config.SaveClientConfig и webui-бэкенд.

- [x] T1.1 Добавить SaveClientConfig в internal/config/client.go — запись `config.ClientConfig` в YAML через `os.UserConfigDir()`. Touches: src/internal/config/client.go
- [x] T1.2 Создать internal/webui/server.go — HTTP server (mux, embed.FS). Touches: src/internal/webui/server.go
- [x] T1.3 Создать internal/webui/state.go — состояние приложения (status, контекст клиента, SSE broadcast). Touches: src/internal/webui/state.go

## Фаза 2: MVP Slice

Цель: cmd/web/main.go + бэкенд handlers — работает через curl.

- [x] T2.1 Реализовать handler_config.go — GET /api/config (читать из config.yaml) и POST /api/config (писать config.yaml). Touches: src/internal/webui/handler_config.go
- [x] T2.2 Реализовать handler_connect.go — POST /api/connect (запустить client.NewFromConfig + Run(ctx)), POST /api/disconnect (cancel ctx). Touches: src/internal/webui/handler_connect.go, src/internal/bootstrap/client/client.go
- [x] T2.3 Реализовать handler_logs.go — SSE /api/logs (логи + статус). Touches: src/internal/webui/handler_logs.go
- [x] T2.4 Создать cmd/web/main.go — entrypoint с флагами --port (default 2311) и --open-browser. Touches: src/cmd/web/main.go
- [x] T2.5 Проверить MVP через curl — go build, ./kvn-web, curl возвращает HTML + JSON. Touches: все выше

## Фаза 3: React SPA

Цель: браузерная панель с формой, connect/disconnect, логом.

- [x] T3.1 Инициализировать React + Vite + TypeScript проект. package.json, tsconfig, vite.config. Touches: src/internal/webui/frontend/
- [x] T3.2 Реализовать React компоненты: App (форма + статус + лог). Touches: src/internal/webui/frontend/src/
- [x] T3.3 Интегрировать API: хуки useConfig, useConnect, useLogs (SSE). Touches: src/internal/webui/frontend/src/App.tsx

## Фаза 4: Сборка и интеграция

Цель: React SPA встроена в Go бинарник, сборка автоматизирована.

- [x] T4.1 Создать scripts/build-web.sh — npm run build + go build -o bin/kvn-web ./src/cmd/web. Touches: scripts/build-web.sh
- [x] T4.2 Добавить //go:embed all:frontend/dist в internal/webui/server.go. Touches: src/internal/webui/server.go
- [x] T4.3 Финальная проверка: go build, go vet, go test -race, gatetest — все pass. Touches: все поверхности

## Покрытие критериев приемки

- AC-001 (один бинарник) -> T2.4, T4.1, T4.2
- AC-002 (web форма) -> T2.1, T3.2
- AC-003 (подключение) -> T2.2, T2.3, T3.3
- AC-004 (disconnect) -> T2.2, T3.3
- AC-005 (сохранение конфига) -> T1.1, T2.1
- AC-006 (proxy mode) -> T2.2 (mode=proxy в форме)
- AC-007 (restart survival) -> T1.1, T2.1
- AC-008 (совместимость) -> T2.4 (не трогает cmd/client)

## Покрытие решений

- DEC-001 (React+Vite) -> T3.1
- DEC-002 (API-first) -> T2.1, T2.2, T2.3, T3.3
- DEC-003 (SSE) -> T2.3, T3.3
- DEC-004 (context+cancel) -> T1.3, T2.2
- DEC-005 (полная перезапись) -> T1.1

## Заметки

- React SPA (Фаза 3) может выполняться параллельно с Фазой 2 — API контракты уже зафиксированы
- Порядок: T1.x → T2.x → T4.1 (build-web.sh) → T3.x → T4.2→T4.3
- T3.1 требует Node.js на машине разработчика
- После завершения всех задач: verify
