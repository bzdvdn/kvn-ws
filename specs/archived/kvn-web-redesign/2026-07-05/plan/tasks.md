# KV Web UI Redesign Задачи

## Phase Contract

Inputs: plan, data-model, spec, inspect.
Outputs: упорядоченные исполнимые задачи с покрытием критериев.
Stop if: нет — план и spec чёткие.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/metrics/client/buffer.go` | T1.1 |
| `src/internal/metrics/client/sender.go` | T1.1 |
| `src/internal/webui/state.go` | T1.2 |
| `src/internal/webui/handler_logs.go` | T1.2 |
| `src/internal/webui/frontend/src/types.ts` | T1.3 |
| `src/internal/webui/frontend/src/theme.ts` | T1.3 |
| `src/internal/webui/frontend/src/ServerCards.tsx` | T2.1 |
| `src/internal/webui/frontend/src/TabbedForm.tsx` | T2.2 |
| `src/internal/webui/frontend/src/TrafficMeter.tsx` | T2.3 |
| `src/internal/webui/frontend/src/LogPanel.tsx` | T2.4 |
| `src/internal/webui/frontend/src/App.tsx` | T2.5 |
| `src/internal/webui/frontend/src/FormField.tsx` | T3.1 |
| `src/internal/metrics/client/buffer_test.go` | T4.1 |

## Implementation Context

- Цель MVP: ServerCards + TabbedForm + Traffic Meter + LogPanel с паузой/поиском/badge + client-side metrics. P2: валидация + Export/Clear.
- Инварианты: action=1 → server, action=2 → direct (маппинг на UI); WS Padding — число (default 512); Default Route — "Server (VPN)" / "Direct (bypass)"; DNS Proxy — 127.0.0.1:5353.
- Контракты: SSE `event: metric` с полями `tx_bytes, rx_bytes, latency_ms, uptime_s, tx_speed, rx_speed, reconnects`. Частота ≤ 1/500ms.
- Proof signals: `npm run build` ok, `go build ./src/cmd/web` ok, SSE DevTools показывает `event: metric`, UI рендерит все компоненты без ошибок.
- Вне scope: Form Validation (P2 — T3.1), Export/Clear (P2 — T3.2), drag-to-reorder, mobile-адаптация, light theme, server-side метрики.
- DEC ссылки: DEC-001 (metrics пакет), DEC-002 (плоская структура), DEC-003 (Context), DEC-004 (uPlot), DEC-005 (CSS modules), DEC-006 (validation rules).

## Фаза 1: Foundation

Цель: подготовить бэкенд-инфраструктуру (client-side metrics + SSE расширение) и фронтенд-базу (types + theme), от которых зависят все компоненты.

- [x] T1.1 Создать пакет `src/internal/metrics/client/` с RingBuffer (thread-safe, последние 60s сэмплов) и sender, который подписывается на события session.go и control.go, формирует MetricSnapshot и передаёт в `state.PushMetric()`. Touches: `src/internal/metrics/client/buffer.go`, `src/internal/metrics/client/sender.go`, `src/internal/tunnel/session.go` (read), `src/internal/protocol/control/` (read)

- [x] T1.2 Расширить `state.go` — добавить канал `metricCh chan MetricSnapshot`, методы `PushMetric()`, `SubscribeMetric()`, `UnsubscribeMetric()`, горутину `broadcastMetrics()`. В `handler_logs.go` — третий select-case на `metricCh`, отправка `event: metric`. Touches: `src/internal/webui/state.go`, `src/internal/webui/handler_logs.go`

- [x] T1.3 Создать `types.ts` (вынос интерфейсов: LogEntry, ServerEntry, ClientConfig, MetricSnapshot, Status, SourceRule) и `theme.ts` (CSS module-файлы с цветами/размерами/шрифтами из mockup). Touches: `src/internal/webui/frontend/src/types.ts`, `src/internal/webui/frontend/src/theme.ts`

## Фаза 2: MVP Slice

Цель: реализовать все компоненты UI, собрать App.tsx, получить работающий MVP со всеми AC-001–AC-005, AC-008–AC-010, AC-012–AC-014.

- [x] T2.1 Реализовать `ServerCards.tsx` — компонент с пилюлей активного сервера и выпадающим списком карточек. Каждая карточка: точка статуса (connected/disconnected/error), имя, URL, при error — текст ошибки. Всегда видимые кнопки Copy/Delete (с модальным confirm), подсветка при hover. AC-003, AC-004. Touches: `src/internal/webui/frontend/src/ServerCards.tsx`

- [x] T2.2 Реализовать `TabbedForm.tsx` — контейнер с табами General / TLS / Routing / Advanced / Global. Каждый таб — отдельный render-блок. Состояние полей хранится в parent (App.tsx), переключение таба не сбрасывает данные. AC-005. Touches: `src/internal/webui/frontend/src/TabbedForm.tsx`

- [x] T2.3 Реализовать `TrafficMeter.tsx` — панель метрик: RX/TX speed (Mbps), total (MB), sparkline (uPlot через ref), latency, uptime, reconnect count. Рендерится только при status=connected. Получает данные из `event: metric` SSE. AC-001, AC-002, AC-014. Touches: `src/internal/webui/frontend/src/TrafficMeter.tsx`, `package.json` (добавить uPlot)

- [x] T2.4 Реализовать `LogPanel.tsx` — level-badge (ERR/WRN/INF/DBG) с цветом, timestamp HH:mm:ss.SSS из Unix ts, поиск с фильтрацией + `<mark>` подсветка, пауза автоскролла при скролле вверх (плашка "⏸ Paused · N new entries"), copy строки по клику, маппинг action: 1→server, 2→direct, 0→none. AC-008, AC-009, AC-010, AC-012. Touches: `src/internal/webui/frontend/src/LogPanel.tsx`

- [x] T2.5 Рефакторинг `App.tsx` — подключить все компоненты через React Context (AppStateProvider), вынести глобальное состояние (servers, status, logs, metrics, активный таб). Удалить старый inline-код. AC-005 (переключение табов). Touches: `src/internal/webui/frontend/src/App.tsx`, `src/internal/webui/frontend/src/context.ts` (новый, если нужен)

- [x] T2.6 Проверить end-to-end: собрать frontend (`npm run build`), собрать Go (`go build ./src/cmd/web`), запустить, проверить все AC MVP. Touches: все файлы MVP

## Фаза 3: P2 — Расширение

Цель: Form Validation + Export/Clear логов.

- [x] T3.1 Реализовать `FormField.tsx` с пропом `rules: ((value) => string | null)[]`. Валидация на onChange (дебаунс 300ms) и на submit. Правила для полей: Server URL (ws:// или wss://), MTU (1280-1500), токен (не пустой). Подсветка красной рамкой + текст ошибки. Блокировка Save при невалидных полях. AC-006, AC-007. Touches: `src/internal/webui/frontend/src/FormField.tsx`, `src/internal/webui/frontend/src/TabbedForm.tsx` (интегрировать FormField)

- [x] T3.2 Добавить в LogPanel кнопки Export (скачивание .txt через Blob) и Clear (очистка массива логов). Export disabled при пустом логе, Clear disabled при пустом логе. AC-011. Touches: `src/internal/webui/frontend/src/LogPanel.tsx`

## Фаза 4: Проверка

Цель: доказать, что фича работает.

- [x] T4.1 Добавить unit-тесты для RingBuffer (вставка, чтение, overflow, thread-safety), sender (форматирование MetricSnapshot). Touches: `src/internal/metrics/client/buffer_test.go`, `src/internal/metrics/client/sender_test.go`

- [x] T4.2 Выполнить verify: `go build ./src/cmd/web`, `npm run build`, ручная проверка всех 14 AC по списку, проверка bundle size (≤ 50KB gzip прирост). Touches: все файлы фичи

## Покрытие критериев приемки

- AC-001 -> T2.3, T2.6
- AC-002 -> T2.3, T2.6
- AC-003 -> T2.1, T2.6
- AC-004 -> T2.1, T2.6
- AC-005 -> T2.2, T2.5, T2.6
- AC-006 -> T3.1, T4.2
- AC-007 -> T3.1, T4.2
- AC-008 -> T2.4, T2.6
- AC-009 -> T2.4, T2.6
- AC-010 -> T2.4, T2.6
- AC-011 -> T3.2, T4.2
- AC-012 -> T2.4, T2.6
- AC-013 -> T1.2, T2.6
- AC-014 -> T1.1, T2.3, T2.6
