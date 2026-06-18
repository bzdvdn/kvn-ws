# Multi-Server Management План

## Phase Contract

Inputs: spec, inspect (pass), минимальный repo-контекст.
Outputs: plan, data-model, contracts/api.
Stop if: — (спека стабильна, inspect pass).

## Цель

Перевести KVN Web UI с единственного конфига на multi-server: новая секция `servers` в `config.yaml`, CRUD-API, селектор в UI, адаптация Connect/Import/Export/QR под выбранный сервер.

## MVP Slice

Все AC (001–006) одним срезом — без multi-server сломаются Import/Export/QR.

## First Validation Path

1. Открыть Web UI → селектор показывает 1 сервер "Default"
2. Добавить сервер "Test" → полный CRUD
3. Переключиться → форма загружает конфиг "Test"
4. Connect использует "Test" → в логах виден адрес "Test"
5. Export/QR — конфиг "Test"

## Scope

- `src/internal/config/` — новый тип `WebUIConfig` + `ServerEntry`, функции `LoadWebUIConfig`/`SaveWebUIConfig`
- `src/internal/webui/` — новые API-эндпоинты, рефакторинг `handler_config.go`, `handler_connect.go`, `server.go`, `state.go`
- `src/internal/webui/frontend/` — селектор серверов, разделение глобальных/серверных полей, адаптация Import/Export/QR
- `~/.config/kvn/config.yaml` — новая структура с `servers: []`
- Не трогается: CLI (`src/cmd/client/`), Android, relay, остальные транспорты

## Implementation Surfaces

| Surface | Почему участвует | Новая/Сущ. |
|---------|-----------------|------------|
| `src/internal/config/client.go` | Новый тип `WebUIConfig`, `ServerEntry`; функции `LoadWebUIConfig`, `SaveWebUIConfig` | Сущ. |
| `src/internal/webui/handler_config.go` | API для списка серверов, CRUD | Сущ. |
| `src/internal/webui/handler_connect.go` | Читает выбранный сервер, подставляет в подключение | Сущ. |
| `src/internal/webui/state.go` | Хранит `activeServer`, сигнализирует переключение | Сущ. |
| `src/internal/webui/server.go` | Регистрация новых роутов | Сущ. |
| `src/internal/webui/frontend/src/App.tsx` | Селектор, dirty-контроль, разделение полей | Сущ. |

## Bootstrapping Surfaces

- `src/internal/config/webui.go` — новый файл для `WebUIConfig`-логики (чтение/запись/миграция).

## Влияние на архитектуру

- `config.yaml` меняет формат: глобальные поля (ClientConfig) сверху + секция `servers: []` + `active_server`.
- При первом запуске: миграция существующего `config.yaml` в `servers[0]` с именем "Default".
- `POST /api/connect` больше не читает `config.yaml` как плоский ClientConfig — берёт выбранный сервер из `WebUIConfig`.
- `/api/config` GET/POST заменяются на `/api/servers` CRUD + отдельный эндпоинт для глобальных полей.
- Нарушений backward compatibility нет: старый `config.yaml` без `servers` корректно мигрируется.

## Acceptance Approach

- AC-001: `GET /api/servers` → массив; UI рендерит селектор; используется `state.ActiveServer`
- AC-002: React useState(dirty) → диалог подтверждения → `POST /api/servers/:name/select` или PUT active_server
- AC-003: `POST /api/servers` (create), `PUT /api/servers/:name` (update), `DELETE /api/servers/:name` (delete)
- AC-004: Import → UI формирует `POST /api/servers` с именем "Imported <ts>"
- AC-005: Export/QR берут JSON выбранного сервера из `state.activeServer.config`
- AC-006: Connect читает `POST /api/servers/:name/connect` или адаптированный POST /api/connect с query-параметром `server`

## Данные и контракты

- Новая сущность: `ServerEntry` (имя + inline ClientConfig).
- Новый корневой конфиг: `WebUIConfig` (ClientConfig global + Servers + ActiveServer).
- API-контракты: `GET/POST /api/servers`, `GET/PUT /api/servers/:name`, `DELETE /api/servers/:name`, `PUT /api/config/global`.
- См. `data-model.md` и `contracts/api.md`.

## Стратегия реализации

### DEC-001 Единый config.yaml с секцией servers

Why: пользователь явно решил хранить всё в одном файле. Миграция существующего конфига тривиальна.
Tradeoff: файл может вырасти при 10+ серверах, но это конфигурационный файл (не runtime-БД).
Affects: `src/internal/config/client.go`, новый `webui.go`.
Validation: после миграции config.yaml содержит секцию servers; старый формат корректно обёрнут.

### DEC-002 API: CRUD серверов через /api/servers/*

Why: отдельные эндпоинты чище, чем PUT /api/config со всем списком. Клиент (React) проще поддерживать.
Tradeoff: больше эндпоинтов, но каждый узкий и тестируемый.
Affects: `handler_config.go`, `server.go`.
Validation: curl POST/DELETE → GET /api/servers отражает изменения.

### DEC-003 Merge сервер → глобальный конфиг при Connect

Why: каждый сервер хранит полный ClientConfig (inline), при подключении поля сервера переопределяют глобальные. Не нужно выдумывать partial override logic.
Tradeoff: дублирование общих полей в каждом сервере, но это конфиг — не runtime.
Affects: `handler_connect.go`, `config.LoadClientConfig`.
Validation: если server-specific proxy_listen не задан, используется глобальный.

### DEC-004 dirty-флаг + диалог подтверждения в React

Why: защита от потери несохранённых изменений при переключении сервера. React-состояние `dirty` отслеживает любые изменения формы.
Tradeoff: небольшое усложнение UI-логики.
Affects: `App.tsx`.
Validation: переключение сервера с изменениями → диалог.

## Incremental Delivery

### MVP (Первая ценность)

1. Backend: `WebUIConfig` + чтение/запись + миграция + API `/api/servers` CRUD + адаптация `/api/connect`
2. Frontend: селектор серверов + dirty-диалог + CRUD-форма + адаптация Import/Export/QR

### Итеративное расширение

- none (одним срезом)

## Порядок реализации

1. Backend: data model (`WebUIConfig`, `ServerEntry`) + Load/Save + миграция
2. Backend: API CRUD `/api/servers` + адаптация `/api/connect`
3. Frontend: селектор + dirty-контроль + форма редактирования
4. Frontend: адаптация Import/Export/QR
5. Интеграция: полный цикл (CRUD → Connect → Export/QR)

Шаги 1–2 можно параллелить с 3 (после согласования API-контрактов).

## Риски

- Конфликт имён при inline `ClientConfig` в `ServerEntry` — нужно проверить, что mapstructure/yaml корректно обрабатывают inline с same-named fields.
  Mitigation: прототип Load/Save с тестами до реализации API.
- Старый `config.yaml` без секции `servers` — миграция должна быть идемпотентной.
  Mitigation: LoadWebUIConfig проверяет `len(servers)==0` → оборачивает.
- React-форма: поле `name` должно быть в серверной секции, но уникально.
  Mitigation: валидация на UI + на backend (409 Conflict при дубликате).

## Rollout и compatibility

- Специальных rollout-действий не требуется: существующий config.yaml мигрируется автоматически.
- При откате на старую версию KVW Web: старый код читает только глобальные поля ClientConfig (игнорируя servers) — частичная совместимость.

## Проверка

- Go unit tests: `config.WebUIConfig` marshal/unmarshal, миграция, merge логика.
- Go integration test: `POST /api/servers` → `GET /api/servers` → `DELETE /api/servers/:name`.
- Frontend: ручная проверка — Add/Edit/Delete/Import/Export/QR через UI.
- `go vet`, `gosec`, `golangci-lint`, `go test -race ./...` — обязательны.

## Соответствие конституции

- нет конфликтов.
