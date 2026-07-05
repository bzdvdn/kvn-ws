# KV Web UI Redesign

## Scope Snapshot

- In scope: редизайн UI kvn-web — замена `<select>` на визуальные ServerCards, табы в форме настроек, панель метрик трафика, улучшение панели логов, валидация полей.
- Out of scope: server-side метрики (Prometheus), админ-панель (sessions), адаптивная вёрстка (mobile), темизация (light theme), Desktop/Android UI.

## Цель

Пользователи kvn-web получают современный интерфейс: наглядный выбор сервера со статус-индикатором, навигация по настройкам через табы без потери данных, live-панель трафика при connected с графиком скорости, логи с level-badge и timestamp HH:mm:ss.SSS, поиск с подсветкой и паузой скролла, валидация полей с подсветкой ошибок. Фича повышает UX без изменений бэкенд-логики.

## Основной сценарий

1. Пользователь открывает kvn-web в браузере.
2. Видит левую панель: карточка текущего сервера со статусом, метриками (если connected), под ней — действия Save/Connect/Export/Import.
3. Клик на серверной пилюле открывает список серверов-карточек с точкой статуса (connected/disconnected/error), URL, последней ошибкой, hover-действиями (Edit/Delete/Copy).
4. Пользователь переключает сервер кликом по карточке.
5. Настройки сервера — в табах: General / TLS / Routing / Advanced / Global. Переключение таба без потери введённых данных.
6. Валидация полей в реальном времени: невалидное поле подсвечивается красной рамкой с текстом ошибки под label.
7. При connected — над настройками отображается Traffic Meter: RX/TX speed (Mbps), total (MB), sparkline-график за последние секунды, latency, uptime, reconnect count.
8. Справа — панель логов с level-badge (`ERR`/`WRN`/`INF`/`DBG`), timestamp `HH:mm:ss.SSS`, action, IP, поиск с подсветкой, пауза автоскролла, кнопки Export и Clear.

## User Stories

- P1 (MVP): ServerCards + TabbedSettings + Log Panel UX (пауза, бейджи, timestamp, поиск с подсветкой) + Traffic Meter.
- P2: Form Validation + Export/Clear логов.

## MVP Slice

ServerCards + TabbedSettings + Traffic Meter + Log Panel UX (pause, badges, timestamps, search highlight). Без Form Validation (P2).

## First Deployable Outcome

После сборки:
- Открыть kvn-web → видно карточки серверов вместо `<select>`
- Переключение между табами настроек работает
- При connect — панель Traffic Meter показывает скорость и sparkline
- Логи отображаются с level-badge, читаемым timestamp, поиск подсвечивает результаты, работает пауза скролла

## Scope

### Frontend (`src/internal/webui/frontend/src/`)

- `App.tsx` — декомпозиция на компоненты, удаление inline-стилей
- `ServerCards.tsx` — новый компонент: карточки серверов со статусом
- `TrafficMeter.tsx` — новый компонент: панель метрик + sparkline
- `TabbedForm.tsx` — новый компонент: табы General/TLS/Routing/Advanced/Global
- `LogPanel.tsx` — новый компонент: улучшенный лог с паузой/поиском/экспортом
- `FormField.tsx` — новый компонент: поле с валидацией, label, error
- `Theme.ts` — вынесенные стили (CSS-in-JS объекты или CSS modules)

### Backend (`src/internal/webui/`)

- `state.go` — добавить канал `metricCh` для метрик трафика
- `handler_logs.go` — добавить `event: metric` в SSE
- `server.go` — зарегистрировать новый endpoint опционально

### Client-side metrics (`src/internal/metrics/client/` — новый пакет)

- Кольцевой буфер для `tx_bytes`, `rx_bytes`, `latency_ms`, `errors`
- Подписка на события из `tunnel/session.go` и `protocol/control/`
- Прокидывание метрик в WebUI SSE через `state.PushMetric()`

### Файлы (новые)

| Файл | Роль |
|------|------|
| `src/internal/webui/frontend/src/ServerCards.tsx` | Server card list component |
| `src/internal/webui/frontend/src/TrafficMeter.tsx` | Traffic meter + sparkline |
| `src/internal/webui/frontend/src/TabbedForm.tsx` | Tab navigation + form sections |
| `src/internal/webui/frontend/src/LogPanel.tsx` | Log panel with pause/export/search |
| `src/internal/webui/frontend/src/FormField.tsx` | Validated form field wrapper |
| `src/internal/webui/frontend/src/types.ts` | Shared TypeScript types (вынос из App.tsx) |
| `src/internal/webui/frontend/src/theme.ts` | Shared styles/theme |
| `src/internal/metrics/client/buffer.go` | Ring buffer for metrics collection |
| `src/internal/metrics/client/sender.go` | Push metrics to WebUI SSE |

## Контекст

- Весь UI сейчас — `App.tsx` на 983 строки с inline-стилями. Рефакторинг на компоненты — предпосылка для любых UX-улучшений.
- SSE поток (`/api/logs`) уже есть, расширяется `event: metric`.
- Client-side метрик нет — нужен новый пакет `src/internal/metrics/client/`.
- LogEntry формат: `{line, level, action?, ip?, ts?}` — достаточно для level-badge + action/ip отображения.
- Нативные JSON-логи содержат `caller` — в LogEntry не передаётся, но может быть добавлен позже.
- WS Padding — числовое поле (по умолчанию 512), не select.
- Default Route: "Server (VPN)" / "Direct (bypass)".
- DNS Proxy Listen по умолчанию `127.0.0.1:5353`.
- DNS Upstreams — список trusted resolver-ов (1.1.1.1, 8.8.8.8).

## Зависимости

- `uPlot` (или `lightweight-charts`) — библиотека для sparkline-графика в Traffic Meter. Выбор остаётся открытым (см. Открытые вопросы), ~35KB gzip.
- Новых внешних Go-зависимостей не требуется (go.mod не меняется).
- Пакет `src/internal/metrics/client/` — новый, без внешних зависимостей.

## Требования

- RQ-001 При connected состоянии НАД формой настроек ДОЛЖНА отображаться панель Traffic Meter: RX/TX скорость (Mbps), total (MB), sparkline за последние 30s, latency, uptime, reconnect count.
- RQ-002 Селектор сервера ДОЛЖЕН быть визуальными карточками: статус (connected/disconnected/error), URL, имя, последняя ошибка, hover-действия Edit/Delete/Copy.
- RQ-003 Форма настроек ДОЛЖНА быть разделена на табы: General, TLS, Routing, Advanced, Global. Переключение таба НЕ ДОЛЖНО сбрасывать введённые данные.
- RQ-004 Каждое поле в табах ДОЛЖНО иметь валидацию: невалидное поле — красная рамка + текст ошибки под label. Сохранение с невалидными полями блокируется.
- RQ-005 Лог-панель ДОЛЖНА: (a) level-badge `ERR`/`WRN`/`INF`/`DBG` с цветовой кодировкой, (b) timestamp `HH:mm:ss.SSS`, (c) поиск с подсветкой `<mark>`, (d) пауза автоскролла при скролле вверх, (e) кнопки Export (TXT) и Clear, (f) copy строки по клику.
- RQ-006 Отображение action в логах ДОЛЖНО быть human-readable: `action:1` → `action:server`, `action:2` → `action:direct`, `action:0` → `action:none` (или скрывать). Маппинг выполняется на UI-стороне без изменения бэкенда.
- RQ-007 Client-side метрики ДОЛЖНЫ собираться в кольцевом буфере (последние 60s минимум) и передаваться в UI через `event: metric` в SSE.
- RQ-008 Sparkline в Traffic Meter ДОЛЖЕН показывать историю RX/TX за последние 30s (минимум 60 точек данных).

## Вне scope

- Server-side метрики (Prometheus) и их отображение.
- Таблица активных сессий (Admin API).
- Адаптивная / мобильная верстка (media queries).
- Переключатель темы (light/dark).
- Android / Desktop UI.
- Drag-to-reorder серверов.
- Wizard первого запуска.
- Step-by-step индикатор соединения.

## Критерии приемки

### AC-001 Traffic Meter отображается при connected

- **Given** kvn-web UI загружен, клиент подключен (status=connected)
- **When** пользователь смотрит на левую панель
- **Then** над формой настроек видна панель Traffic Meter: RX speed (Mbps), TX speed (Mbps), sparkline, latency, uptime, reconnect count
- Evidence: панель видна, значения обновляются, sparkline меняется

### AC-002 Traffic Meter скрыт при disconnected

- **Given** клиент отключён (status=disconnected)
- **When** пользователь смотрит на левую панель
- **Then** Traffic Meter НЕ отображается
- Evidence: панель отсутствует, форма настроек начинается сразу

### AC-003 ServerCards отображают статус сервера

- **Given** пользователь открыл список серверов
- **When** просматривает карточки
- **Then** каждая карточка содержит: цветную точку статуса (зелёный=connected, серый=disconnected, красный=error), имя, URL, при error — текст ошибки
- Evidence: визуально разные точки статуса, текст ошибки виден на карточке dev-test

### AC-004 кнопки действий на ServerCard

- **Given** отображается карточка сервера
- **When** пользователь смотрит на карточку
- **Then** видны кнопки Copy config и Delete (с модальным подтверждением); при наведении подсвечиваются
- Evidence: кнопки видны всегда, Copy подсвечивается ярким, Delete — красным при hover

### AC-005 Табы сохраняют состояние

- **Given** пользователь заполнил поле в табе Advanced
- **When** переключается на General, потом обратно на Advanced
- **Then** введённое значение сохранилось
- Evidence: поле содержит то же значение

### AC-006 Валидация поля Server URL

- **Given** поле Server URL содержит "http://example.com"
- **When** пользователь вводит значение
- **Then** поле подсвечивается красной рамкой, под label текст "Must start with ws:// or wss://"
- Evidence: красная рамка, текст ошибки виден

### AC-007 Валидация блокирует сохранение

- **Given** хотя бы одно поле формы невалидно
- **When** пользователь нажимает Save
- **Then** сохранение не выполняется, невалидные поля подсвечены
- Evidence: Save не отправляет PUT запрос, поля подсвечены красным

### AC-008 Лог: level-badge и timestamp

- **Given** в логе есть entry с level=error, ts=1234567890
- **When** entry рендерится
- **Then** виден красный бейдж `ERR` и timestamp в формате `HH:mm:ss.SSS`
- Evidence: текст `ERR` на красном фоне, время читаемое

### AC-009 Лог: поиск с подсветкой

- **Given** пользователь ввёл "error" в поиск
- **When** лог отфильтрован
- **Then** все вхождения "error" в `msg` подсвечены `<mark>`, несовпадающие entry скрыты
- Evidence: подсветка видна, счётчик показывает `N / total`

### AC-010 Лог: пауза автоскролла

- **Given** пользователь прокрутил лог вверх
- **When** появляются новые entry
- **Then** автоскролл приостановлен, показывается плашка "⏸ Paused · N new entries ▼ Scroll to bottom"
- Evidence: плашка видна, новые записи не сдвигают окно просмотра

### AC-011 Лог: Export и Clear

- **Given** в логе есть записи
- **When** пользователь нажимает Export (⤓)
- **Then** скачивается `.txt` файл со всеми записями
- **When** пользователь нажимает Clear (✕)
- **Then** лог очищается
- Evidence: файл скачан, после Clear лог пуст

### AC-012 Action в логах human-readable

- Почему это важно: пользователь видит "server" / "direct" вместо "1" / "2"
- **Given** в SSE пришёл log entry с `action: "1"`
- **When** entry рендерится в лог-панели
- **Then** отображается `action:server` (не `action:1`)
- Evidence: `action:1` → `action:server`, `action:2` → `action:direct`

### AC-013 Client-side метрики поступают в SSE

- **Given** клиент подключён
- **When** SSE поток активен
- **Then** приходят `event: metric` с полями `tx_bytes`, `rx_bytes`, `latency_ms`, `uptime_s`, `tx_speed`, `rx_speed`
- Evidence: в DevTools Network видны SSE сообщения с event type `metric`

### AC-014 Sparkline обновляется

- **Given** Traffic Meter виден, данные поступают
- **When** проходит >2s
- **Then** sparkline график в Traffic Meter меняется (бары/линия двигаются)
- Evidence: визуальное изменение sparkline

## Допущения

- React 18 + TypeScript 5.6 остаются.
- Библиотека sparkline (uPlot / lightweight-charts) определится на этапе реализации.
- SSE EventSource уже используется — расширение новым типом события не ломает существующие.
- Client-side метрики читаются из tunnel/session.go (bytes) и protocol/control/ (latency).
- LogEntry.ts — числовой Unix timestamp, преобразуется в `HH:mm:ss.SSS` на UI.
- WS Padding size — число, по умолчанию 512.
- DNS Proxy listen — `127.0.0.1:5353`.
- Default Route: "Server (VPN)" / "Direct (bypass)".

## Критерии успеха

- SC-001 Sparkline обновляется ≤ 1s после получения metric event.
- SC-002 Переключение таба ≤ 50ms (без лагов UI).
- SC-003 Поиск по логу среди 1000 entry ≤ 100ms.
- SC-004 Export 1000 строк лога ≤ 500ms.
- SC-005 Traffic Meter не вызывает reflow формы настроек (layout shift).
- SC-006 Прирост bundle-размера frontend ≤ 50KB gzip.

## Краевые случаи

- **Нет серверов:** ServerCards пустой + кнопка "Add Server" на весь блок.
- **Один сервер:** скрыть Delete (нельзя удалить последний), как сейчас.
- **Очень длинные URL в ServerCard:** text-overflow ellipsis.
- **Очень много серверов (20+):** ServerCards скроллится, высота списка ограничена.
- **SSE reconnect:** после переподключения EventSource метрики продолжают поступать.
- **Browser tab в фоне:** метрики накапливаются в кольцевом буфере, при возвращении — sparkline догоняет.
- **Log entry без level/timestamp:** fallback — серый бейдж `---`, ts = "?".
- **Export при пустом логе:** кнопка Export disabled, Clear disabled.

## Открытые вопросы

- uPlot или lightweight-charts для sparkline? uPlot меньше (~35KB), lightweight-charts (~120KB) от TradingView имеет больше фич.
- `event: metric` отправлять раз в 1s или раз в 500ms? Зависит от частоты данных из tunnel/session.go.
- Сохранять ли collapsed/expanded состояние табов между сессиями? (localStorage)
- Стили: CSS modules или inline `React.CSSProperties` как сейчас? Рекомендуется CSS modules для читаемости.
