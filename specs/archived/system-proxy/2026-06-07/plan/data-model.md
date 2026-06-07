# System Proxy — Data Model

## Status

**no-change** (минимальное расширение существующей модели)

## Changes

### ClientConfig

| Поле | Тип | Обязательность | По умолчанию | Описание |
|------|-----|---------------|-------------|----------|
| `system_proxy` | `*bool` | optional | `nil` (auto) | `true` — установить системные прокси; `false` — не трогать; `nil` — включить для `mode: proxy` |

### RoutingCfg

Без изменений. exclude_ranges, exclude_ips, exclude_domains уже существуют и используются для построения NO_PROXY.

## Rationale

- `*bool` позволяет отличить "явно false" от "не задано" (auto = true для proxy mode)
- NO_PROXY строится runtime из существующих routing exclude-правил — новые поля не нужны
- Системные прокси не сохраняются в конфиг ОС — только в runtime окружение процесса
