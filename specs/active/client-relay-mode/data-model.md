# Client Relay Mode — Модель данных

## Scope

- Связанные `AC-*`: AC-003 (конфиг), AC-004 (max_connections)
- Связанные `DEC-*`: DEC-001, DEC-002
- Статус: `changed`
- Добавляется структура конфигурации `RelayCfg`, новых persisted entities нет.

## Сущности

### DM-001 RelayCfg

- Назначение: конфигурация relay-режима в `ClientConfig`
- Источник истины: YAML-файл конфига клиента + env override
- Инварианты:
  - `Listen` обязателен при `mode: relay`
  - `MaxConnections >= 1` (default 100)
  - Либо `TLS.Cert` + `TLS.Key`, либо ни одного (self-signed fallback)
- Связанные `AC-*`: AC-003, AC-004
- Связанные `DEC-*`: DEC-002
- Поля:
  - `listen` — string, required, addr:port для TLS listener
  - `ws_paths` — `[]string`, optional, default `["/tunnel"]`, allowlist WS path для входящих подключений
  - `max_connections` — int, optional, default 100, макс. число bridge-сессий
  - `tls` — `RelayTLSCfg`, optional; если не указан — self-signed fallback
    - `cert` — string, optional, путь к PEM-сертификату
    - `key` — string, optional, путь к PEM-ключу
- Жизненный цикл:
  - загружается при старте клиента через `config.LoadClientConfig()`
  - не меняется в runtime (SIGHUP reload — deferred, не входит в scope P1)
- Замечания по консистентности:
  - `mode: relay` + `RelayCfg == nil` → ошибка валидации при старте

### DM-002 RelaySession (runtime, не persisted)

- Назначение: runtime-структура bridge-сессии между клиентом и upstream
- Источник истины: память (не сохраняется)
- Инварианты: одно in-memory соединение = одна bridge-сессия
- Поля:
  - `clientConn` — `transport.StreamConn`, входящее WS-соединение
  - `upstreamConn` — `transport.StreamConn`, исходящее upstream-соединение
  - `ctx` — `context.Context` с cancel, управляет временем жизни сессии
  - `wg` — `sync.WaitGroup`, ожидает завершения двух bridge-горутин
  - `closeOnce` — `sync.Once`, предотвращает double-close
- Жизненный цикл:
  - создаётся после успешного handshake между client и upstream
  - завершается при обрыве любого из двух соединений (через cancel ctx)
  - удаляется из памяти при закрытии обоих bridge-горутин

## Связи

- `ClientConfig.Relay` -> `RelayCfg` (1:0..1) — relay config присутствует только при `mode: relay`
- `RelaySession.clientConn` -> `transport.StreamConn` (1:1) — одно входящее соединение
- `RelaySession.upstreamConn` -> `transport.StreamConn` (1:1) — одно upstream-соединение

## Производные правила

- `RelaySession` не сериализуется, не восстанавливается после рестарта relay.

## Переходы состояний

- RelaySession:
  - `connecting` (upstream dial) → `handshake` (forward ClientHello/ServerHello) → `active` (bridge loop) → `done` (close)
  - Ошибка на любой фазе → немедленный переход в `done`

## Вне scope

- Persistence: `RelaySession` — runtime-only, не сохраняется.
- Client-side session state: relay не управляет сессиями — это ответственность upstream.
