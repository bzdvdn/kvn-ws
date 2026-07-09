# Arch Fix: Critical Paths — Data Model

## Status: no-change

## Обоснование

Ни одно исправление не требует изменения структур данных, полей конфигурации, протокольных сообщений или контрактов. Все изменения локальны:

- AC-001 (QUIC backoff): runtime helper, без данных
- AC-002 (DNS pool): sync.Pool, без структуры
- AC-003 (DNS cache limit): int-константа `defaultMaxCacheSize`, поле `r.dnsCache` уже существует
- AC-004 (relay session): вызовы существующих методов SessionManager
- AC-005 (uint16 overflow): guard condition, без данных
- AC-006 (boundary checks): guard conditions, без данных
- AC-007 (тесты): mock-структуры в _test.go

## Сущности

Не изменяются.
