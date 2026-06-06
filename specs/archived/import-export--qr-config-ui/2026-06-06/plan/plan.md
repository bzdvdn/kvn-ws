# Import / Export Config + QR — План

## Цель

Добавить в Web UI кнопки Export (clipboard), Import (paste JSON) и QR-код для конфига. Frontend-only, без изменений бэкенда.

## MVP Slice

Export + Import через clipboard — закрывает AC-001, AC-002. QR (AC-003) — отдельная фича в рамках того же PR.

## First Validation Path

1. Открыть Web UI, настроить server + token
2. Нажать Export → вставить в текстовый файл → валидный JSON
3. Нажать Import → вставить JSON → поля формы заполнились
4. Нажать QR → появился QR-код, читаемый любым сканером

## Implementation Surfaces

| Surface | Change |
|---------|--------|
| `src/internal/webui/frontend/src/App.tsx` | 3 кнопки, textarea для Import, модал для QR |
| `src/internal/webui/frontend/package.json` | Добавить `qrcode` зависимость |

## Bootstrapping Surfaces

- `App.tsx` — существующий файл, добавляются UI компоненты
- `package.json` — добавить `qrcode` в `dependencies`

## Влияние на архитектуру

- Backend zero impact — все изменения только в React SPA
- QR-генерация на клиенте (npm `qrcode`), без сетевых запросов
- Clipboard API (`navigator.clipboard`) — стандартное Web API, без полифилов

## Acceptance Approach

### AC-001 Export to clipboard
- Surfaces: `App.tsx`
- Подход: кнопка Export → `JSON.stringify(config)` → `navigator.clipboard.writeText()`
- Validation: после нажатия Ctrl+V в блокноте — валидный JSON

### AC-002 Import from clipboard
- Surfaces: `App.tsx`
- Подход: кнопка Import → textarea → `JSON.parse` + `setConfig()` → подсветка Save
- Validation: вставить JSON → все поля заполнились, ошибка парсинга показывает alert

### AC-003 QR code
- Surfaces: `App.tsx`, `package.json`
- Подход: модальное окно с `<canvas>`, `QRCode.toCanvas()` из npm `qrcode`
- Validation: скан QR телефоном → читаемая JSON-строка

### AC-004 Backward compat
- Surfaces: `App.tsx`
- Подход: `setConfig()` обновляет только те поля, что есть в JSON; остальные остаются с предыдущими значениями или дефолтами
- Validation: импорт `{"server":"wss://x.com/ws"}` не сбрасывает token/obfuscation/etc.

## Данные и контракты

- Формат: JSON, минимизированный (без пробелов), соответствует `ClientConfig` интерфейсу
- QR-контент: та же JSON-строка, закодированная в QR (`qrcode` npm)

## Стратегия реализации

### DEC-001 Export через Clipboard API
- **Why**: нативный Web API, не требует зависимостей, работает в современных браузерах
- **Tradeoff**: `navigator.clipboard` не работает в HTTP (только HTTPS/localhost) — kvn-web всегда под HTTPS
- **Affects**: `App.tsx`
- **Validation**: Paste в текстовый редактор — валидный JSON

### DEC-002 Import через JSON.parse + setConfig
- **Why**: текущий state конфига уже в React, достаточно `setConfig(JSON.parse(text))`
- **Tradeoff**: JSON-схема должна совпадать с `ClientConfig` — forward compat через игнорирование неизвестных полей
- **Affects**: `App.tsx`
- **Validation**: Поля формы заполнились, невалидный JSON показывает alert

### DEC-003 QR через npm `qrcode`
- **Why**: `qrcode` — самая популярная библиотека (2.6k ⭐), генерирует canvas без сервера
- **Tradeoff**: +15 KB gzip к бандлу — приемлемо для Web UI
- **Affects**: `App.tsx`, `package.json`
- **Validation**: Скан QR телефоном — читаемая JSON-строка

## Порядок реализации

1. Export (AC-001) — 10 строк React
2. Import (AC-002) — 30 строк React  
3. QR (AC-003) — 20 строк React + `npm install qrcode`
4. Backward compat (AC-004) — проверка при реализации Import

## Риски

- `navigator.clipboard` не работает в HTTP (только HTTPS/localhost) — но kvn-web уже HTTPS или localhost
- `qrcode` npm может увеличить размер бандла (~15 KB gzip) — приемлемо для Web UI
- JSON с чувствительными данными (token) в буфере обмена — UI должен показывать предупреждение

## Проверка

| Шаг | AC |
|-----|----|
| Export → clipboard → вставить в блокнот | AC-001 |
| Import → валидный JSON → поля заполнились | AC-002 |
| Import → невалидный JSON → alert | AC-002 |
| QR → сканировать → строка совпадает с Export | AC-003 |
| Import с неполным JSON → остальные поля не сброшены | AC-004 |
