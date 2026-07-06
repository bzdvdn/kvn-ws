# KVN Web Config Update — Модель данных

## Scope

- Связанные `AC-*`: `AC-005`, `AC-006`
- Связанные `DEC-*`: `DEC-003`
- Статус: `no-change` (Go), `expansion` (TypeScript)

## No-Change Stub

- Статус: `no-change` для Go-структур
- Причина: `ClientConfig.ProxyConnections` уже существует в `client.go:48` с дефолтом 10 (client.go:410-412). `RoutingCfg` и его `[]string` поля не меняются. Все изменения — только TypeScript-типы и UI.
- Единственное расширение: `proxy_connections?: number` в `types.ts:ClientConfig` (фронтенд). Поле опционально — старые конфиги без него используют дефолт 10 на бэкенде.
- Revisit triggers:
  - появление нового сохраняемого routing-поля
  - изменение структуры `RoutingCfg` в Go
  - per-server override для `proxy_connections`
