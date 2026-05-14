# Production Gap Модель данных

## Scope

- Связанные `AC-*`: `AC-001`, `AC-002`, `AC-003`, `AC-005`
- Связанные `DEC-*`: `DEC-001`, `DEC-002`, `DEC-003`
- Статус: `changed`
- Изменения ограничены configuration/value-object surfaces; persisted session/entities не меняются

## Сущности

### DM-001 Client TLS Trust Settings

- Назначение: описать, как клиент определяет доверие к серверному сертификату в production runtime.
- Источник истины: `ClientConfig` и связанный YAML/env override.
- Инварианты:
  - insecure fallback не является default behavior
  - trusted CA/material и server identity проверяются согласованно
  - verify mode интерпретируется однозначно при запуске клиента
- Связанные `AC-*`: `AC-001`
- Связанные `DEC-*`: `DEC-001`
- Поля:
  - `ca_file` - string, optional, путь к CA bundle для self-managed trust
  - `server_name` - string, optional, ожидаемый TLS server name/SNI when needed
  - `verify_mode` - string, required for explicit semantics, определяет normal trusted verify vs controlled local/dev exception if such mode remains supported
- Жизненный цикл:
  - создаётся при чтении client config
  - используется при построении client TLS config перед dial
  - обновляется только через config reload/restart path клиента
- Замечания по консистентности:
  - недопустимо состояние, где runtime silently падает обратно на insecure verify при отсутствии trust material

### DM-002 Server Client Certificate Policy

- Назначение: описать operator-facing режим проверки client certificate на сервере.
- Источник истины: `ServerConfig.TLS` и server TLS builder.
- Инварианты:
  - режимы `request`, `require`, `verify` различаются предсказуемо
  - режим, требующий доверия, не принимает unknown client cert
  - `client_ca_file` обязателен для trust-enforcing режимов
- Связанные `AC-*`: `AC-002`
- Связанные `DEC-*`: `DEC-002`
- Поля:
  - `client_auth` - string, required, режим проверки client cert
  - `client_ca_file` - string, optional for request mode, required for trust-enforcing modes
- Жизненный цикл:
  - читается из server config
  - преобразуется в `tls.ClientAuthType` при старте сервера
  - меняется только через config update/restart or reload path
- Замечания по консистентности:
  - недопустимо состояние, где `require` по смыслу включён, но runtime проверяет лишь наличие любого сертификата

### DM-003 Operational Endpoint Access Token

- Назначение: единый credential для доступа к operational endpoints, попадающим в scope текущей фичи.
- Источник истины: server config surface для operational/admin token и runtime middleware.
- Инварианты:
  - `/metrics` и admin surface не обслуживаются как production-ready без валидного токена
  - один и тот же token gate применяется согласованно к обеим operational surfaces в рамках текущего scope
- Связанные `AC-*`: `AC-005`
- Связанные `DEC-*`: `DEC-003`
- Поля:
  - `token` - string, required for protected operational access
  - `header_name` - derived/string, expected request header carrying token
- Жизненный цикл:
  - задаётся в server config
  - используется при обработке HTTP-запросов к `/metrics` и admin routes
  - ротируется через config update path
- Замечания по консистентности:
  - недопустимо состояние, где admin token требуется, а `/metrics` остаётся публичным на том же runtime perimeter

## Связи

- `DM-001 -> DM-002`: обе сущности формируют согласованный TLS trust boundary между клиентом и сервером, но владеются разными runtime-конфигами.
- `DM-003 -> DM-002`: operational token gate независим от mTLS, но обслуживает тот же production hardening perimeter сервера.

## Производные правила

- Если `verify_mode` клиента требует проверку доверия, runtime должен либо использовать системный trust store, либо явно заданный `ca_file`, но не fallback на insecure verify.
- Если server `client_auth` требует доверие, отсутствие корректного `client_ca_file` делает конфигурацию невалидной для production path.
- Operational access считается корректным только при наличии валидного токена в ожидаемом header.

## Переходы состояний

- `client config loaded` -> `TLS trust settings resolved` -> `client dial allowed or rejected`
- `server config loaded` -> `client certificate policy resolved` -> `mtls handshake accepted or rejected`
- `http request received` -> `operational token checked` -> `endpoint served or denied`

## Вне scope

- Session persistence, IP pool, routing rules, tunnel frame payloads и admin response payload shapes не моделируются заново в рамках этой фичи.
