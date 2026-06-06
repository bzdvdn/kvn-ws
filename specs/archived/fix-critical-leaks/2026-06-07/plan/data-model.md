---
data_model_status: no-change
---

# Data Model: fix-critical-leaks

**Status**: no-change

Ни одна таблица, структура данных или схема БД не меняется. Все изменения в плане — локальные патчи в существующих файлах без расширения data model.

## Обоснование

- AC-001..AC-003: меняется только lifecycle горутин, не данные
- AC-004..AC-006: context propagation, не данные
- AC-007: параметр `bbolt.Options.Timeout`, не схема
- AC-008: замена `time.After` на `time.NewTimer`, не данные
- AC-009: логирование ошибок, не данные
- AC-010: новый mutex в `SessionManager`, не данные
- AC-011: замена type assertion, не данные
- AC-012: вынос helper-функций, не данные
- AC-013: sync.Pool, не данные

## Влияние

- `SessionManager` получает новое поле `cancelFuncsMu sync.Mutex` (AC-010) — это добавление поля, но не изменение логики существующих полей
- `Listener` получает новое поле `sem chan struct{}` (AC-002) — аналогично
- `quic.Dial` сигнатура: добавляется первый параметр `ctx context.Context` — compile-time breaking, легко чинится (DEC-003)

## Миграция

- Не требуется
