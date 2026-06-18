# Multi-Server Management Задачи

## Phase Contract

Inputs: plan, data-model, contracts/api.
Outputs: исполнимые задачи с покрытием AC.
Stop if: — (план стабилен, AC покрываемы).

## Surface Map

| Surface                                   | Tasks      |
| ----------------------------------------- | ---------- |
| `src/internal/config/webui.go` (new)      | T1.1, T1.2 |
| `src/internal/config/client.go`           | T1.2       |
| `src/internal/webui/handler_config.go`    | T2.1, T4.1 |
| `src/internal/webui/handler_connect.go`   | T2.2       |
| `src/internal/webui/state.go`             | T2.2       |
| `src/internal/webui/server.go`            | T2.1       |
| `src/internal/webui/frontend/src/App.tsx` | T3.1, T3.2 |

## Implementation Context

- Цель MVP: перевести Web UI с одного конфига на список серверов с CRUD, переключением и адаптированным Connect/Import/Export/QR.
- Инварианты/семантика:
  - `config.yaml`: глобальные поля ClientConfig сверху + `active_server` + `servers: []` с уникальными `name`
  - При `len(servers)==0` после загрузки — миграция: обернуть глобальный ClientConfig в `servers[0]` с name="Default"
  - При подключении: merge серверного ClientConfig поверх глобального (серверные поля приоритетнее)
  - name уникален, непустой, без `/` и `#`
- Ошибки/коды:
  - `409 Conflict` — дубликат name при create/rename
  - `404 Not Found` — сервер :name не существует
  - `409 Conflict` — попытка удалить последний сервер
- Контракты/протокол:
  - `GET /api/servers` → `{ active_server, servers: [...] }`
  - `POST /api/servers` → 201, тело: `{ name, ...ClientConfig }`
  - `PUT /api/servers/:name` → 200, тело: `{ name?, ...ClientConfig }`
  - `DELETE /api/servers/:name` → 200
  - `PUT /api/config/global` → тело: поля ClientConfig
  - `POST /api/connect` — без изменений (читает active_server из WebUIConfig)
- Границы scope: не трогаем CLI, Android, relay, transport-пакеты
- Proof signals: селектор показывает серверы; переключение → форма обновляется; Connect → в логах адрес выбранного сервера; Export/QR → конфиг выбранного сервера
- References: DEC-001, DEC-002, DEC-003, DEC-004, DM-001, DM-002, API-001–006

## Фаза 1: Data Model

Цель: WebUIConfig + ServerEntry типы, Load/Save с миграцией.

- [x] T1.1 Создать `src/internal/config/webui.go` — типы `WebUIConfig` и `ServerEntry` (name + inline ClientConfig), функции `LoadWebUIConfig(path)` и `SaveWebUIConfig(path, cfg)`. AC-001. Touches: `src/internal/config/webui.go`, `src/internal/config/client.go`
- [x] T1.2 Реализовать миграцию: при `len(servers)==0` обернуть глобальные поля ClientConfig в `servers[0]` с name="Default". Обработать случай отсутствия файла — создать дефолтный конфиг. Добавить unit-тесты на marshal/unmarshal и миграцию. AC-001. Touches: `src/internal/config/webui.go`, `src/internal/config/client.go`

## Фаза 2: Backend API

Цель: CRUD эндпоинты серверов + адаптация connect.

- [x] T2.1 Реализовать эндпоинты в `handler_config.go`: `GET /api/servers` (список + active_server), `POST /api/servers` (create), `PUT /api/servers/:name` (update/rename), `DELETE /api/servers/:name` (delete), `PUT /api/config/global` (сохранение глобальных полей). Зарегистрировать роуты в `server.go`. Старый `GET /api/config` адаптирован для обратной совместимости. Touches: `src/internal/webui/handler_config.go`, `src/internal/webui/server.go`
- [x] T2.2 Адаптировать `handler_connect.go` — POST /api/connect читает `WebUIConfig.active_server`, находит сервер в списке, мержит с глобальными полями, создаёт клиент. Добавлена функция `mergeConfig`. Touches: `src/internal/webui/handler_connect.go`, `src/internal/webui/state.go`

## Фаза 3: Frontend

Цель: селектор серверов, dirty-контроль, форма, адаптация Import/Export/QR.

- [x] T3.1 Добавить селектор серверов в хедере (выпадающий список рядом со статусом). Реализовать dirty-флаг (useState) — при переключении с несохранёнными изменениями показывать диалог Save/Discard/Cancel. Форма редактирования: разделить на глобальные поля (log, proxy_listen и т.д.) и поля выбранного сервера (server, auth, tls, routing и т.д.). Save сохраняет оба через PUT /api/config/global + PUT /api/servers/:name. Touches: `src/internal/webui/frontend/src/App.tsx`
- [x] T3.2 Import: POST /api/servers с именем "Imported <timestamp>". Export: скопировать JSON конфига выбранного сервера (GET /api/servers/active или из локального state). QR: генерировать из JSON выбранного сервера. Touches: `src/internal/webui/frontend/src/App.tsx`

## Фаза 4: Проверка

Цель: тесты, lint, manual проверка.

- [x] T4.1 Добавить Go unit-тесты: `WebUIConfig` marshal/unmarshal, миграция из пустого/старого формата, merge-логика, CRUD через API (httptest). Touches: `src/internal/config/webui.go`, `src/internal/webui/handler_config.go`, `src/internal/webui/handler_connect.go`
- [x] T4.2 Прогнать `go vet ./...`, `gosec ./...`, `golangci-lint run ./...`, `go test -race ./...`. Убедиться, что нет новых предупреждений. Провести manual smoke-тест: открыть UI → добавить 2 сервера → переключиться → Connect → Export → QR → Delete. Touches: `src/internal/webui/frontend/src/App.tsx`, `src/internal/config/webui.go`, `src/internal/webui/handler_config.go`, `src/internal/webui/handler_connect.go`

## Покрытие критериев приемки

- AC-001 -> T1.1, T2.1, T3.1
- AC-002 -> T2.1, T3.1
- AC-003 -> T2.1, T3.1
- AC-004 -> T3.2
- AC-005 -> T3.2
- AC-006 -> T2.2, T3.1
  н
