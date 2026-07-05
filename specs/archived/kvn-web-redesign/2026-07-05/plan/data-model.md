---
status: no-change
---

# Data Model: kvn-web-redesign

## Status

`no-change` — ни один persisted тип данных не меняется.

## Обоснование

- **ClientConfig** — не меняется: те же поля ServerEntry, GlobalConfig, RoutingConfig и т.д.
- **LogEntry** — не меняется: `{line, level, action?, ip?, ts?}` остаётся как есть.
- **MetricSnapshot** — новый in-memory тип, не сохраняется в BoltDB/config. Существует только в runtime:
  ```go
  type MetricSnapshot struct {
    TxBytes    int64   `json:"tx_bytes"`
    RxBytes    int64   `json:"rx_bytes"`
    LatencyMs  float64 `json:"latency_ms"`
    UptimeS    int64   `json:"uptime_s"`
    TxSpeed    float64 `json:"tx_speed"`   // Mbps
    RxSpeed    float64 `json:"rx_speed"`   // Mbps
    Reconnects int64   `json:"reconnects"`
  }
  ```
- **RingBuffer** — внутренняя структура пакета `metrics/client/`, не сериализуется.
- **TypeScript типы** — расширяются новым `MetricSnapshot` интерфейсом, но это pure frontend.

## Затронутые модули

- `src/internal/metrics/client/` — новый пакет, новые типы
- `src/internal/webui/state.go` — новый канал `chan MetricSnapshot`
- `src/internal/webui/frontend/src/types.ts` — новый интерфейс `MetricSnapshot`
