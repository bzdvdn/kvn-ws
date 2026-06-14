# QUIC Relay Mode — Модель данных

## Scope

- Связанные `AC-*`: `AC-003`
- Связанные `DEC-*`: `DEC-001`, `DEC-004`
- Статус: `changed`

## Сущности

### DM-001 RelayQuicCfg

- Назначение: конфигурация QUIC listener в relay mode
- Источник истины: YAML-конфиг (`relay.quic`)
- Инварианты:
  - Если секция присутствует — QUIC listener запускается
  - idle_timeout > 0
  - keep_alive >= 0
- Связанные `AC-*`: `AC-003`
- Связанные `DEC-*`: `DEC-001`, `DEC-004`
- Поля:
  - `keep_alive` — `time.Duration` (в YAML: `7s`), опционально, default `7s`. Период QUIC keepalive pings.
  - `idle_timeout` — `time.Duration` (в YAML: `60s`), опционально, default `60s`. Максимальное время бездействия QUIC-соединения.
- Жизненный цикл:
  - Создаётся при загрузке конфига, если присутствует блок `relay.quic`
  - Не обновляется и не удаляется после старта relay
- Замечания по консистентности:
  - Если `idle_timeout` < ClientHello timeout (30s) — QUIC-соединение может закрыться раньше, чем relay прочитает ClientHello. Валидация на уровне конфига.

### DM-002 RelayCfg (изменение)

- Назначение: расширение существующей структуры конфига relay
- Изменения:
  - Добавляется поле `Quic *RelayQuicCfg` (omitempty) — включает QUIC listener
- Жизненный цикл: без изменений

## Связи

- `RelayCfg.Quic -> RelayQuicCfg`: опциональная ссылка; если не nil — QUIC listener запускается.

## Производные правила

- Если `RelayCfg.Quic != nil` → в `runRelayMode` добавляется QUIC accept loop в errgroup.

## Вне scope

- `ObfuscatedQUICConn` конфиг — P2.
- mTLS для QUIC — отложено.
