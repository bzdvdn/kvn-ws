# Context — src/internal/metrics/client

## Purpose

Клиентские метрики для Web UI: атомарные счётчики трафика/латентности/реконнектов, периодический `Sender` со скоростями и `RingBuffer` истории, которые транслируются в SSE-событие `metric`.

## Responsibilities

- Владеет: `MetricSnapshot` (публичный JSON-контракт), `Collector` (атомарные счётчики + uptime + ринг), `RingBuffer`, `Sender` (расчёт скоростей по тикам).
- Не владеет: HTTP/SSE-доставкой (`webui`), подсчётом байт на транспорте (`transport.CountingStreamConn`), серверными Prometheus-метриками (`metrics`).

## How it works

- `MetricSnapshot` — поля `tx_bytes, rx_bytes, latency_ms, uptime_s, tx_speed, rx_speed, reconnects`; именно этот JSON уходит в SSE.
- `Collector` хранит `txBytes/rxBytes/reconnects` в `atomic.Int64`, `latencyMs` — как биты `float64` в `atomic.Uint64`; `Start()` фиксирует `startedAt` (uptime считается от него), `Stop()` идемпотентно закрывает `done`.
- `AddTX/AddRX` — накопление; `Snapshot()` возвращает текущие значения с `TxSpeed/RxSpeed = 0` (скорости считает только `Sender`).
- `RingBuffer` — фиксированная ёмкость (у `Collector` — 60 точек), кольцевая запись под мьютексом; `Read()` возвращает копию в хронологическом порядке.
- `Sender.Run(ctx)`: каждые `interval` берёт `Snapshot`, считает скорость `Δbytes * 8 / seconds / 1e6` (Мбит/с) относительно предыдущего снапшота, кладёт в `collector.Ring()`, затем non-blocking отправляет в `out chan MetricSnapshot` (при переполнении — тихо дропает).
- Скорости при первом тике считаются от нулевого `prev`, поэтому первый замер — «средняя за всё время до тика».

## Key files

- `src/internal/metrics/client/buffer.go` — `MetricSnapshot`, `RingBuffer`, `Collector`.
- `src/internal/metrics/client/sender.go` — `Sender`, расчёт скоростей.

## Public contracts

- `MetricSnapshot` с JSON-тегами (совпадает с `metricEvent` в `src/internal/webui/handler_logs.go`).
- `NewRingBuffer(capacity int)`, `Push`, `Read() []MetricSnapshot`, `Len`, `Cap`.
- `NewCollector()`, `Start`, `Stop`, `Done`, `AddTX`, `AddRX`, `SetLatency(ms float64)`, `IncReconnects`, `Snapshot`, `Ring`.
- `NewSender(collector, out chan MetricSnapshot, interval time.Duration)`, `(*Sender).Run(ctx)`.

## Dependencies

- Только стандартная библиотека (`sync/atomic`, `math`, `time`).

## Consumed by

- `src/internal/bootstrap/client` — `SetMetricCollector`, `Sender`, обёртка `CountingStreamConn`.
- `src/internal/webui` — `AppState.metricCollector`, SSE `event: metric`.
- `src/internal/tunnel` — `AddThroughput` соседнего пакета `metrics`, не этого.

## Gotchas

- `Collector.Stop()` закрывает `done` навсегда: повторный `Start()` не переоткрывает канал и не сбрасывает счётчики — коллектор одноразовый, для новой сессии создаётся новый (это делает Web UI).
- `Sender` на первом тике считает скорость от нуля, а не от запуска — первый показатель завышен.
- Скорость в **мегабитах** (`*8/1e6`), а не в байтах/с — не путать с `tx_bytes`.
- `Snapshot()` всегда отдаёт `TxSpeed/RxSpeed = 0`; реальные скорости появляются только в снапшотах `Sender`.
- `RingBuffer` с `capacity <= 0` паникует на делении по модулю/индексации — ёмкость должна быть > 0.
- `Sender` шлёт в `out` non-blocking (`default`), поэтому медленный потребитель молча теряет точки; история при этом сохраняется в ринге.
- `RingBuffer.Cap()` читается без блокировки, но поле неизменно после конструктора — это безопасно, а `Len()` блокируется.
- `SetLatency` принимает любой `float64` без валидации (включая `NaN`/`Inf`) — фильтрация значений остаётся на вызывающем.
