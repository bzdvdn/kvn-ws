# KV Web UI Redesign План

## Phase Contract

Inputs: spec, inspect (pass), repo-surface `src/internal/webui/`, `src/internal/webui/frontend/src/`, `src/internal/metrics/`, `src/internal/tunnel/`, `src/internal/protocol/control/`.
Outputs: plan, data-model stub.
Stop if: нет — spec чёткая, inspect pass.

## Цель

Реализовать редизайн UI kvn-web: декомпозировать монолитный `App.tsx` (983 строки) на компоненты, добавить ServerCards, TabbedForm, Traffic Meter, улучшить Log Panel, добавить client-side метрики. Backend-изменения минимальны — новый канал в state.go + `event: metric` в SSE. Новый пакет `src/internal/metrics/client/` для сбора метрик.

## MVP Slice

ServerCards + TabbedSettings + Traffic Meter + Client-side метрики + Log Panel UX (pause, badges, timestamps, search highlight, action mapping). Без Form Validation и Export/Clear.

AC покрывает: AC-001, AC-002, AC-003, AC-004, AC-005, AC-008, AC-009, AC-010, AC-012, AC-013, AC-014.

## First Validation Path

1. `cd src/internal/webui/frontend && npm run build` — сборка без ошибок
2. `go build -o /tmp/kvn-web ./src/cmd/web && /tmp/kvn-web`
3. Открыть `http://127.0.0.1:2310`
4. Видны ServerCards вместо `<select>`
5. Переключение табов General/TLS/Routing/Advanced/Global работает
6. Нажать Connect — появляется Traffic Meter с данными
7. Логи отображаются с level-badge и timestamp HH:mm:ss.SSS, action:1 → action:server

## Scope

### Frontend — декомпозиция App.tsx

- `ServerCards.tsx` — карточки серверов со статусом, всегда видимыми кнопками Copy/Delete
- `TrafficMeter.tsx` — панель метрик: скорость, total, sparkline, latency, uptime, reconnect
- `TabbedForm.tsx` — контейнер с табами: General / TLS / Routing / Advanced / Global
- `LogPanel.tsx` — улучшенный лог: level-badge, timestamp, search highlight, pause, copy, action mapping
- `FormField.tsx` — базовый компонент поля с label + error (используется всеми табами)
- `types.ts` — вынос типов (LogEntry, ServerEntry, ClientConfig, MetricSnapshot)
- `theme.ts` — вынос стилей
- `App.tsx` — редуцирован до композиции компонентов + глобальное состояние

### Backend — SSE расширение

- `state.go` — новый канал `metricCh chan MetricSnapshot`, методы `PushMetric()`, `SubscribeMetric()`, `UnsubscribeMetric()`
- `handler_logs.go` — третий select-case на `metricCh`, отправка `event: metric`
- Никаких новых HTTP endpoint-ов

### Client-side metrics — новый пакет

- `src/internal/metrics/client/buffer.go` — кольцевой буфер (RingBuffer) на последние 60s сэмплов, thread-safe
- `src/internal/metrics/client/sender.go` — подписка на события session.go (bytes) и control.go (latency), форматирование MetricSnapshot, отправка в state.PushMetric()

### Не меняется

- `src/internal/webui/server.go` — не требует изменений (уже рендерит SPA)
- `src/cmd/web/main.go` — не меняется
- `go.mod` — не меняется (все зависимости уже есть; sparkline — npm-зависимость)

## Performance Budget

- SC-001: Sparkline обновляется ≤ 1s после metric event.
- SC-003: Поиск по логу 1000 entry ≤ 100ms.
- SC-004: Export 1000 строк ≤ 500ms.
- SC-006: Прирост frontend bundle ≤ 50KB gzip.
- Client-side metrics: ≤ 1 alloc/op на сэмпл в RingBuffer.
- SSE `event: metric` не чаще 1 раз/500ms (дебаунс в sender.go).

## Implementation Surfaces

| Surface | Статус | Роль |
|---------|--------|------|
| `src/internal/webui/frontend/src/App.tsx` | рефакторинг | композиция компонентов, глобальный state |
| `src/internal/webui/frontend/src/ServerCards.tsx` | новая | ServerCard list + pill |
| `src/internal/webui/frontend/src/TrafficMeter.tsx` | новая | Traffic meter + sparkline |
| `src/internal/webui/frontend/src/TabbedForm.tsx` | новая | Tab container |
| `src/internal/webui/frontend/src/LogPanel.tsx` | новая | Log panel |
| `src/internal/webui/frontend/src/FormField.tsx` | новая | Validated field |
| `src/internal/webui/frontend/src/types.ts` | новая | Shared types |
| `src/internal/webui/frontend/src/theme.ts` | новая | Shared styles |
| `src/internal/webui/state.go` | модификация | metricCh, PushMetric, SubscribeMetric |
| `src/internal/webui/handler_logs.go` | модификация | event: metric в SSE |
| `src/internal/metrics/client/buffer.go` | новая | RingBuffer |
| `src/internal/metrics/client/sender.go` | новая | Metric сборщик |

## Bootstrapping Surfaces

- `src/internal/webui/frontend/src/` — уже существует, нужны только новые файлы
- `src/internal/metrics/client/` — новая директория
- `src/internal/webui/state.go` — уже существует, модификация in-place

## Влияние на архитектуру

- **App.tsx теряет ~700 строк** — логика разделяется на 6 компонентов + types + theme
- **state.go получает третий канал** — архитектура pub/sub остаётся той же (pattern уже есть для logCh и statusCh)
- **Client-side metrics** — новый кросс-пакетный модуль, подписывается на session.go и control.go
- **SSE поток** — новый event type `metric`, не ломает существующие `log` и `status`
- **Никакого влияния** на TUN, routing, transport, config, или другие модули

## Acceptance Approach

| AC | Подход | Surfaces |
|----|--------|----------|
| AC-001 Traffic Meter visible | TrafficMeter рендерится при status=connected | TrafficMeter.tsx, App.tsx (state.status) |
| AC-002 Traffic Meter hidden | TrafficMeter не рендерится при status≠connected | TrafficMeter.tsx |
| AC-003 ServerCards status | ServerCard принимает status из server entry | ServerCards.tsx |
| AC-004 Always-visible actions | ServerCard показывает кнопки Copy/Delete всегда | ServerCards.tsx |
| AC-005 Tab state preserved | TabbedForm хранит activeTab в useState, поля в serverConfig (React state) | TabbedForm.tsx, App.tsx |
| AC-008 Level-badge + timestamp | LogPanel рендерит lv-бейдж + HH:mm:ss.SSS из Unix ts | LogPanel.tsx |
| AC-009 Search highlight | filter + `<mark>` в msg, как в mockup | LogPanel.tsx |
| AC-010 Pause auto-scroll | scroll handler → флаг paused + плашка | LogPanel.tsx |
| AC-012 Action human-readable | switch(action) в рендере: "1"→"server", "2"→"direct" | LogPanel.tsx |
| AC-013 SSE metric event | state.go metricCh + handler_logs.go | state.go, handler_logs.go |
| AC-014 Sparkline updates | RingBuffer данных → uPlot update | TrafficMeter.tsx, buffer.go |

## Данные и контракты

**Data model**: не меняется. ClientConfig/ServerEntry/LogEntry — те же поля. Новый тип MetricSnapshot (in-memory, не сохраняется). Подробнее — `data-model.md`.

**Контракты**: SSE получает новый event type `metric`. Существующие `log` и `status` не меняются. REST API endpoints не меняются.

## Стратегия реализации

### DEC-001 Client-side metrics: кольцевой буфер в отдельном пакете

Why: метрики нужны в реальном времени для sparkline, но не должны сохраняться в config/BoltDB. Отдельный пакет `src/internal/metrics/client/` изолирует concern сбора и форматирования метрик от UI-логики.
Tradeoff: дополнительный пакет и горутина-сборщик. Но без внешних зависимостей и с thread-safe доступом через каналы.
Affects: `buffer.go`, `sender.go`, `state.go`.
Validation: `go test ./src/internal/metrics/client/...` + SSE DevTools проверка.

### DEC-002 Компонентная декомпозиция: одна директория, плоская структура

Why: текущий App.tsx (983 строки) — монолит. Декомпозиция на 6-7 файлов в той же директории без вложенных поддиректорий — минимальный рефакторинг, достаточный для читаемости. Все компоненты разделяют единый `types.ts`.
Tradeoff: файлов больше, но навигация проще. Когда компонентов станет > 15 — можно перейти на `components/` поддиректорию.
Affects: `App.tsx`, новые `.tsx` файлы.
Validation: `npm run build` проходит, UI работает.

### DEC-003 Глобальное состояние через React Context (без внешних библиотек)

Why: текущий UI держит всё состояние в `useState` в App.tsx. Добавление Redux/Zustand — лишняя зависимость. React Context + useReducer достаточно для ~6 компонентов.
Tradeoff: при > 20 компонентов Context может вызывать лишние ререндеры. Для текущего scope — не проблема.
Affects: `App.tsx` (Provider), все компоненты (useContext).
Validation: переключение табов, сохранение полей, обновление метрик — без видимых лагов.

### DEC-004 uPlot для sparkline

Why: uPlot (~35KB gzip) — минимальная библиотека для sparkline, без зависимостей, быстрая (нет виртуального DOM), легко встраивается в React через ref. lightweight-charts (~120KB) избыточен для sparkline.
Tradeoff: uPlot не имеет встроенного React-враппера — потребуется хук `useSparkline(ref, data)`. API низкоуровневый.
Affects: `TrafficMeter.tsx`, `package.json`.
Validation: sparkline обновляется в реальном времени, bundle size ≤ 50KB gzip.

### DEC-005 CSS modules для стилей

Why: inline `React.CSSProperties` в текущем App.tsx неподдерживаемы (нет autocomplete, нельзя переиспользовать, нельзя media queries). CSS modules дают scoped-стили без рантайм-оверхеда, с autocomplete и без внешних зависимостей.
Tradeoff: дополнительные `.module.css` файлы. Vite поддерживает CSS modules из коробки.
Affects: все компоненты (import styles from `*.module.css`).
Validation: стили применяются корректно, имена классов хэшированы.

### DEC-006 Валидация: per-field rules + validateOnChange

Why: валидация запускается на onChange (с дебаунсом 300ms) и на submit. Правила валидации — функции `(value) => string | null`, возвращающие текст ошибки или null. FormField принимает `rules` пропом.
Tradeoff: дебаунс может создать ощущение задержки. 300ms — компромисс между мгновенной обратной связью и производительностью.
Affects: `FormField.tsx`, все табы (передают rules).
Validation: AC-006, AC-007.

## Incremental Delivery

### MVP — ServerCards + TabbedForm + Traffic Meter + Log Panel UX + Metrics

- AC-001, AC-002, AC-003, AC-004, AC-005, AC-008, AC-009, AC-010, AC-012, AC-013, AC-014
- Все компоненты реализованы, client-side метрики работают, SSE event: metric приходит
- Валидация отсутствует (всегда save)

### P2 — Form Validation + Export/Clear

- AC-006, AC-007, AC-011
- FormField с правилами валидации
- Export (Blob + download) и Clear в LogPanel

## Порядок реализации

1. **Backend: metrics пакет** — `buffer.go` + `sender.go`. Независим от UI.
2. **Backend: SSE расширение** — `state.go` (metricCh) + `handler_logs.go` (event: metric). Зависит от (1).
3. **Frontend: types.ts + theme.ts** — база для всех компонентов.
4. **Frontend: ServerCards.tsx** — самый независимый компонент, не требует метрик.
5. **Frontend: TabbedForm.tsx** — переиспользует theme.ts.
6. **Frontend: TrafficMeter.tsx** — использует SSE metric event. Зависит от (2).
7. **Frontend: LogPanel.tsx** — использует существующий SSE log event + action mapping.
8. **Frontend: FormField.tsx** — для P2.
9. **App.tsx** — сборка, подключение всех компонентов.

Параллельно: (1) и (3) можно делать одновременно. (4) и (7) независимы.

## Риски

- **uPlot API неудобен для real-time обновлений sparkline.** Mitigation: если uPlot не подходит — fallback на lightweight-charts (больше, но с готовым API). Решение на этапе имплементации.
- **App.tsx рефакторинг сломает существующее поведение.** Mitigation: инкрементальный подход — каждый новый компонент сначала используется параллельно со старым кодом, затем старый удаляется. Валидация через `npm run build` + ручная проверка после каждого компонента.
- **Client-side metrics не успевают за SSE (частые обновления).** Mitigation: дебаунс 500ms в sender.go + кольцевой буфер с фиксированным размером. Если данных нет — sparkline показывает flat line.
- **Browser в фоне — SSE приостанавливается.** Mitigation: Page Visibility API + накопление метрик в RingBuffer. При возвращении вкладки — sparkline "догоняет" за 1-2 тика.

## Rollout и compatibility

- Новые npm-зависимости — стандартный `npm install`.
- SSE `event: metric` — обратно совместим: старые клиенты игнорируют неизвестный event type.
- Флаг/feature toggle не требуется — весь UI обновляется атомарно.
- Старый `App.tsx` не сохраняется — полная замена на компонентную архитектуру.

## Проверка

- `go test ./src/internal/metrics/client/...` — unit-тесты кольцевого буфера и sender
- `go build ./src/cmd/web` — компиляция Go
- `npm run build` — сборка frontend без ошибок
- `npm run dev` — ручная проверка UI
- DevTools SSE — проверка `event: metric` в Network
- Поэлементная проверка: каждый AC-* проверяется вручную через UI

## Соответствие конституции

- нет конфликтов. Go 1.22+, TypeScript 5.6, без глобального состояния, trace-маркеры `@sk-task` над функциями/компонентами.
