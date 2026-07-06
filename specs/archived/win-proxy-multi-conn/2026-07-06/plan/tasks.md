# Win Proxy — Multi-Connection Architecture: Tasks

## Phase Contract

Inputs: `specs/active/win-proxy-multi-conn/spec.md`, `specs/active/win-proxy-multi-conn/plan.md`.
Outputs: задачи с покрытием AC-001, AC-002, AC-003, AC-004.
Stop if: нет.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/config/client.go` | T1.1 |
| `src/internal/bootstrap/client/proxy.go` | T2.1, T2.2, T2.3, T3.1, T3.2, T3.3, T3.4, T3.5, T3.6, T4.1 |
| `src/internal/bootstrap/client/dial.go` | (reference only — `dialStream` reused) |
| `src/internal/bootstrap/client/client.go` | T4.2 (reference — TUN path unchanged) |
| `src/internal/proxy/stream.go` | (reference only — Manager already thread-safe) |

## Implementation Context

- Цель MVP: устранить `wmu` contention за счёт N независимых WS-соединений (слотов) с round-robin распределением stream'ов. N задаётся в конфиге (`proxy_connections`, default 10).
- Границы приемки: AC-001, AC-002, AC-003, AC-004 (все уже пройдены).
- Ключевые правила:
  - trace-маркеры `@sk-task win-proxy-multi-conn#<id>` на изменённых объявлениях
  - TUN mode не затронут — никаких изменений в `runSession`
  - Сервер не требует изменений — каждое WS-соединение обрабатывается независимо
- Proof signals: `grep -c "handshake complete" <client-log>` == N; round-robin метрики; reconnect при любой ошибке

## Фаза 1: Конфиг

- [x] T1.1 Добавить поле `ProxyConnections` в `ClientConfig`.
  Touches: `src/internal/config/client.go:47`
  - `ProxyConnections int \`json:"proxy_connections" mapstructure:"proxy_connections"\``
  - Дефолт 10 в `setClientDefaults`
  - AC-001

## Фаза 2: Структуры и создание N соединений

- [x] T2.1 Определить `proxySlot` структуру и читать `proxy_connections` из конфига.
  Touches: `src/internal/bootstrap/client/proxy.go:27-32`, `proxy.go:39-43`
  - `type proxySlot struct { stream transport.StreamConn; mgr *proxy.Manager }`
  - `numConns := c.cfg.ProxyConnections` с fallback 10
  - Каждый слот владеет отдельным `transport.StreamConn` и своим `proxy.Manager`
  - AC-001

- [x] T2.2 Реализовать `doHandshake()` — полный ClientHello↔ServerHello обмен на одном соединении.
  Touches: `src/internal/bootstrap/client/proxy.go:99-152`
  - Encode `ClientHello` → отправка через `stream.WriteMessage`
  - Чтение ответа через `stream.ReadMessage`
  - Обработка `FrameTypeAuth` (reject), `FrameTypeHello` (success), прочие (error)
  - Логирование `session_id` и `assigned_ip` при успехе
  - Возврат `false` при любой ошибке
  - AC-001

- [x] T2.3 Реализовать `dialProxySlots(N)` — последовательное создание N соединений с handshake.
  Touches: `src/internal/bootstrap/client/proxy.go:67-96`
  - Цикл `for i := 0; i < numConns; i++`
  - Каждый вызов `dialStream(ctx, c.cfg, c.logger)` — общая функция dial из `dial.go`
  - Опциональная обёртка в `transport.CountingStreamConn` для метрик
  - Вызов `doHandshake()` на каждом stream
  - При ошибке любого — закрыть все уже открытые, вернуть `nil`
  - AC-001

## Фаза 3: Multi-Connection сессия

- [x] T3.1 Создать отдельный `proxy.Manager` для каждого слота.
  Touches: `src/internal/bootstrap/client/proxy.go:238-244`
  - Цикл по `slots[]`: `slot.mgr = proxy.NewManager(slot.stream, logFn)`
  - Каждый Manager мультиплексирует stream'ы внутри своего слота через `framing.FrameTypeProxy`
  - Нет разделяемых структур между слотами
  - AC-001, AC-002

- [x] T3.2 Добавить round-robin выбор слота в `onConn`.
  Touches: `src/internal/bootstrap/client/proxy.go:259-372`
  - `var nextSlot atomic.Uint64` — lock-free счётчик
  - `idx := nextSlot.Add(1) % numSlots` — выбор слота для каждого входящего прокси-стрима
  - Routing bypass (`routing.RouteDirect`) выполняется до round-robin
  - `slot.mgr.Add(s)` → `s.ForwardToStream(slot.stream)` → `slot.mgr.Remove(s.ID)`
  - AC-002

- [x] T3.3 Запустить N read-loop горутин — одна на слот.
  Touches: `src/internal/bootstrap/client/proxy.go:401-407`
  - `proxyReadLoop(gctx, slot.stream, slot.mgr, c)` — читает фреймы со своего stream
  - Маршрутизация `FrameTypeProxy` → `mgr.HandleIncomingFrame`
  - Маршрутизация `FrameTypeDNS` → `c.dnsSrv.HandleDNSResponse`
  - Deadline 60s с retry на `os.ErrDeadlineExceeded`
  - При ошибке любого read-loop — errgroup завершает все, триггеря reconnect
  - AC-001

- [x] T3.4 Привязать DNS proxy к слоту 0.
  Touches: `src/internal/bootstrap/client/proxy.go:217`
  - `c.dnsSrv.SetStream(slots[0].stream)` — DNS-ответы шлются через первое соединение
  - `c.dnsSrv.ClearStream()` при shutdown
  - AC-006 (RQ-006)

- [x] T3.5 Добавить QUIC keepalive на слоте 0.
  Touches: `src/internal/bootstrap/client/proxy.go:409-442`
  - Только когда `c.cfg.Transport == "quic"`
  - `time.NewTicker(25 * time.Second)` → PING-фрейм каждые 25s
  - Write deadline 10s на каждый PING
  - При ошибке: `return fmt.Errorf("keepalive: %w", err)` → errgroup → reconnect всех
  - AC-007 (RQ-007)

- [x] T3.6 TearDown всех слотов при любой ошибке.
  Touches: `src/internal/bootstrap/client/proxy.go:155-161, 444-453`
  - `defer` закрывает все N streams при выходе из `runProxySessionMulti`
  - `errgroup` запускает accept-loop + N read-loops + keepalive
  - Любая ошибка в errgroup → `eg.Wait()` возвращает → все streams закрыты
  - AC-003

## Фаза 4: Reconnect и защита TUN mode

- [x] T4.1 Реализовать reconnect loop в `runProxyMode()`.
  Touches: `src/internal/bootstrap/client/proxy.go:35-65`
  - Бесконечный цикл: `dialProxySlots(N)` → `runProxySessionMulti` → backoff
  - При `dialProxySlots() == nil` → sleep + увеличение backoff
  - При возврате из `runProxySessionMulti` (любая ошибка) → retry с backoff
  - `sleepWithContext` для прерывания по `ctx.Done()`
  - AC-003

- [x] T4.2 Проверить, что TUN mode не затронут.
  Touches: `src/internal/bootstrap/client/client.go` (reference)
  - `Run()` выбирает `reconnectLoop` при `mode != "proxy"` — multi-conn код недоступен
  - TUN: один вызов `dialStream` → один `runSession`
  - AC-004

## Покрытие критериев приемки

- AC-001 → T1.1, T2.1, T2.2, T2.3, T3.1, T3.3
- AC-002 → T3.1, T3.2
- AC-003 → T3.3, T3.5, T3.6, T4.1
- AC-004 → T4.2

## Заметки

- Все задачи уже реализованы. Tasks описывают завершённую реализацию для аудита и verify.
- Поле `proxy_connections` опционально в конфиге; при отсутствии / <= 0 используется 10.
