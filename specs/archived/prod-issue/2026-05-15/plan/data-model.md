# Data Model — prod-issue

**status:** no-change

Ни одна сущность data model не меняется:
- конфигурация (server.yaml / client.yaml) — без изменений полей
- BoltDB схема (IP pool buckets) — без изменений
- WebSocket frame protocol — без изменений
- Session struct — без изменений
- DNS cache — внутренняя структура меняется (защита RLock → Lock для expired entries), но внешний контракт (Get/Set signature) остаётся тем же

Причина: spec фиксирует только баги/race/bad practices, не расширяет функциональность.
