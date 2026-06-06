# Import / Export Config + QR — Задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/webui/frontend/package.json` | T3.1 |
| `src/internal/webui/frontend/src/App.tsx` | T1.1, T2.1, T3.1, T4.1 |

## Implementation Context

- Все изменения только в React SPA (`App.tsx`)
- Бэкенд не меняется
- Формат: минимизированный JSON, `navigator.clipboard`, `qrcode` npm
- Proof signals: `npm run build` проходит, кнопки работают в браузере

## Фаза 1: Export

- [x] T1.1 Добавить кнопку "Export" рядом с Save. При нажатии — `JSON.stringify(config)` + `navigator.clipboard.writeText()`. Показать toast "Config copied to clipboard". Touches: src/internal/webui/frontend/src/App.tsx

## Фаза 2: Import

- [x] T2.1 Добавить кнопку "Import". При нажатии — показать textarea. После вставки JSON — `JSON.parse()` + `setConfig()`. Если JSON невалидный — ошибка под textarea. При успехе — подсветить кнопку Save (оранжевый). Touches: src/internal/webui/frontend/src/App.tsx

## Фаза 3: QR

- [x] T3.1 Установить `qrcode` npm-пакет. Добавить кнопку "QR". При нажатии — модал с `<canvas>` (QRCode.toCanvas). При пустом конфиге — кнопка disabled. Закрытие модала по клику вне или кнопке "Copy & Close". Touches: src/internal/webui/frontend/src/App.tsx, src/internal/webui/frontend/package.json

## Фаза 4: Backward compat

- [x] T4.1 При импорте — `setConfig()` merge через `{...prev, ...parsed}`. Остальные поля сохраняют текущие значения. Неизвестные поля игнорируются (JSON.parse их просто добавляет в объект). Touches: src/internal/webui/frontend/src/App.tsx

## Покрытие критериев приемки

- AC-001 -> T1.1
- AC-002 -> T2.1
- AC-003 -> T3.1
- AC-004 -> T4.1

## Заметки

- T1.1 и T2.1 можно делать последовательно (Export проще, разогрев)
- T3.1 требует `npm install qrcode` — сделать последним, чтобы не плодить rebase
- T4.1 — проверка при реализации T2.1
