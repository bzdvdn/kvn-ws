# Multi-Server Management Модель данных

## Scope

- Связанные `AC-*`: AC-001, AC-002, AC-003, AC-004, AC-005, AC-006
- Связанные `DEC-*`: DEC-001, DEC-003
- Статус: `changed`

## Сущности

### DM-001 WebUIConfig

- Назначение: корневой конфиг KVN Web UI, хранится в `~/.config/kvn/config.yaml`.
- Источник истины: файл `config.yaml`.
- Инварианты:
  - `active_server` должен ссылаться на существующий сервер в списке `servers`, либо быть пустым.
  - При пустом `servers` после загрузки создаётся сервер "Default".
- Связанные `AC-*`: AC-001, AC-006
- Связанные `DEC-*`: DEC-001, DEC-003
- Поля:
  - (inline) `ClientConfig` — глобальные поля (log, proxy_listen, mtu, auto_reconnect и т.д.) — служат значениями по умолчанию для всех серверов
  - `active_server` — string, имя выбранного сервера, optional
  - `servers` — `[]ServerEntry`, список серверов, required
- Жизненный цикл:
  - создаётся при первом запуске Web UI: если `config.yaml` не существует или без секции `servers`, оборачивается в `servers[0]` с name="Default"
  - обновляется при каждом Save/PUT /api/servers
  - удаляется только при удалении файла пользователем
- Замечания по консистентности:
  - не допускается `active_server`, указывающий на несуществующий сервер — при удалении выбранного сервера active_server переключается на первый оставшийся

### DM-002 ServerEntry

- Назначение: конфигурация одного VPN-сервера.
- Источник истины: секция `servers` в `config.yaml`.
- Инварианты:
  - `name` уникален в пределах `servers`.
  - `name` непустой, не содержит `/`, `#` (безопасен для URL).
- Связанные `AC-*`: AC-001, AC-002, AC-003, AC-004
- Связанные `DEC-*`: DEC-001, DEC-003
- Поля:
  - `name` — string, required, уникальный идентификатор сервера
  - (inline) `ClientConfig` — поля сервера (server, auth, transport, tls, routing и т.д.), при подключении мержатся поверх глобальных
- Жизненный цикл:
  - создаётся: `POST /api/servers` или Import
  - обновляется: `PUT /api/servers/:name` или Save в UI
  - удаляется: `DELETE /api/servers/:name`
- Замечания по консистентности:
  - не допускается дублирование `name` — при попытке создать/переименовать сервер с существующим именем возвращается 409 Conflict

## Связи

- `WebUIConfig.servers` → `ServerEntry`: один ко многим, композиция.
- `WebUIConfig.active_server` → `ServerEntry.name`: ссылка, внешний ключ.

## Производные правила

- **Merge при подключении**: поля выбранного `ServerEntry.ClientConfig` мержатся поверх глобального `WebUIConfig.ClientConfig`. Серверные поля приоритетнее.
- **Миграция**: если после загрузки `len(servers) == 0`, весь текущий `ClientConfig` (глобальные поля) оборачивается в `servers[0]` с name="Default". Если файла не существует — создаётся `servers[0]` с конфигом по умолчанию.

## Переходы состояний

- Создание сервера: `nil` → `POST /api/servers` → сервер в списке (не выбран)
- Выбор сервера: `active_server != name` → PUT active_server → `active_server == name`
- Удаление: сервер в списке → `DELETE /api/servers/:name` → сервер удалён; если это был active_server → active_server = первый оставшийся

## Вне scope

- Валидация полей `ClientConfig` на backend (кроме уникальности name) — доверяем существующей логике `config.LoadClientConfig`.
