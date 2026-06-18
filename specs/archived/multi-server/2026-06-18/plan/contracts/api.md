# API Контракт

## Scope

- Связанные `AC-*`: AC-001, AC-002, AC-003, AC-004, AC-005, AC-006
- Связанные `DEC-*`: DEC-002

## API-001 GET /api/servers — список серверов

- Назначение: получить список всех серверов и имя активного.
- Trigger: загрузка Web UI, обновление после CRUD.
- Inputs: нет.
- Outputs:
  - `active_server` — string, имя выбранного сервера
  - `servers` — `[]ServerDTO`, массив серверов
  - `ServerDTO`:
    - `name` — string, имя сервера
    - `server` — string, URL сервера
    - (остальные поля ClientConfig)
- Ошибки: нет.
- Idempotency / Ordering: GET, всегда идемпотентно. Порядок серверов сохраняется как в файле.
- Связанные `AC-*`: AC-001

## API-002 POST /api/servers — создать сервер

- Назначение: добавить новый сервер в список.
- Trigger: кнопка "Add Server" или Import.
- Inputs:
  - `name` — string, required, уникальное имя
  - (остальные поля ClientConfig) — optional
- Outputs:
  - `201 Created` — `{ name: "..." }`
- Ошибки:
  - `409 Conflict` — сервер с таким именем уже существует
  - `400 Bad Request` — пустое имя
- Idempotency / Ordering: не идемпотентно (создаёт новый ресурс).
- Связанные `AC-*`: AC-003, AC-004

## API-003 PUT /api/servers/:name — обновить сервер

- Назначение: обновить поля существующего сервера (включая name — переименование).
- Trigger: Save формы редактирования.
- Inputs:
  - `name` — string, новое имя (опционально, если не меняется)
  - (остальные поля ClientConfig)
- Outputs:
  - `200 OK` — `{ name: "..." }`
- Ошибки:
  - `404 Not Found` — сервер :name не найден
  - `409 Conflict` — новое имя конфликтует с другим сервером
- Idempotency / Ordering: PUT, идемпотентно. Если name меняется, active_server корректируется.
- Связанные `AC-*`: AC-002, AC-003

## API-004 DELETE /api/servers/:name — удалить сервер

- Назначение: удалить сервер из списка.
- Trigger: кнопка "Delete".
- Inputs: нет.
- Outputs:
  - `200 OK` — `{ status: "deleted" }`
- Ошибки:
  - `404 Not Found` — сервер :name не найден
  - `409 Conflict` — попытка удалить последний сервер
- Idempotency / Ordering: идемпотентно (повторный DELETE вернёт 404).
- Связанные `AC-*`: AC-003

## API-005 PUT /api/config/global — сохранить глобальные настройки

- Назначение: обновить глобальные поля ClientConfig (не серверные).
- Trigger: Save в секции глобальных настроек.
- Inputs:
  - поля ClientConfig (log, proxy_listen, mtu, auto_reconnect и т.д.)
- Outputs:
  - `200 OK` — `{ status: "ok" }`
- Ошибки: `500 Internal Server Error`.
- Idempotency / Ordering: PUT, идемпотентно.
- Связанные `AC-*`: AC-006

## API-006 POST /api/connect — подключиться к выбранному серверу

- Назначение: запустить подключение к активному (выбранному) серверу.
- Trigger: кнопка "Connect".
- Inputs: нет (имя сервера берётся из active_server).
- Outputs:
  - `200 OK` — `{ status: "connecting" }`
- Ошибки:
  - `400 Bad Request` — active_server не задан
  - `400 Bad Request` — сервер active_server не найден
  - `409 Conflict` — уже подключено
- Idempotency / Ordering: не идемпотентно (меняет состояние).
- Связанные `AC-*`: AC-006

## Заметки

- Для импорта через POST /api/servers: если имя не указано в JSON, генерируется "Imported <timestamp>".
- Для экспорта: GET /api/servers/:name?format=json (или формат совпадает со структурой ServerDTO).
- QR-генерация остаётся на frontend (qrcode библиотека), данные берутся из текущего выбранного сервера.
