# Import / Export Config + QR — Web UI

## Scope Snapshot

- In scope: импорт конфига из буфера обмена в Web UI, экспорт текущего конфига в буфер, генерация QR-кода с конфигом.
- Out of scope: multi-server профили, хранение профилей в BoltDB/файлах, переключение между серверами.

## Цель

Пользователь может поделиться конфигом с другого устройства (через QR), быстро вставить конфиг из буфера обмена в Web UI, скопировать настройки для передачи коллеге.

## Сценарии

### SC-001: Export config
1. Пользователь настроил клиент в Web UI
2. Нажимает «Export» — JSON-строка конфига копируется в буфер обмена
3. В UI всплывает toast/уведомление «Config copied»

### SC-002: Import config
1. Пользователь открывает Web UI
2. Нажимает «Import» — появляется текстовое поле
3. Вставляет JSON из буфера (Ctrl+V / кнопка Paste)
4. UI парсит JSON и заполняет все поля настроек (server, token, obfuscation, tls.sni, routing, etc.)
5. Если JSON невалидный — подсветка ошибки

### SC-003: QR code generation
1. Пользователь нажимает «QR» — открывается модал с QR-кодом
2. QR-код содержит JSON-строку текущего конфига
3. Под QR — кнопка «Close» и счётчик/прогресс обновления (config live)

### SC-004: Integration with live log
1. После импорта конфига — автоматический Save (или кнопка Save подсвечена)
2. В live log появляется `info config imported from clipboard`

## Критерии приемки

### AC-001 Export to clipboard
- **Given** Web UI открыт с любыми настройками
- **When** пользователь нажимает «Export»
- **Then** в буфере обмена — валидный JSON текущего `ClientConfig`
- Evidence: `navigator.clipboard.writeText()` вызван, toast «Copied»

### AC-002 Import from clipboard
- **Given** Web UI открыт
- **When** пользователь нажимает «Import» и вставляет JSON
- **Then** все поля формы обновляются в соответствии с JSON
- **And** если JSON содержит поля, которых нет в UI — они игнорируются (forward compat)
- **And** если JSON невалидный — показывается ошибка, форма не меняется

### AC-003 QR code
- **Given** Web UI открыт
- **When** пользователь нажимает «QR»
- **Then** отображается модал с QR-кодом, содержащим JSON конфига
- Evidence: `qrcode` npm-пакет генерирует `<canvas>`, конфиг сериализован в JSON

### AC-004 Backward compat
- **Given** конфиг без новых полей (например, без `obfuscation`, `tls.sni`)
- **When** пользователь импортирует такой JSON
- **Then** поля принимают значения по умолчанию (как при пустом config endpoint)
- Evidence: `fetchConfig()` возвращает defaults для отсутствующих полей

## Implementation Surfaces

| Surface | Change |
|---------|--------|
| `src/internal/webui/frontend/src/App.tsx` | Кнопки Export/Import/QR, JSON textarea для Import, модал с QR |
| `src/internal/webui/frontend/package.json` | Добавить `qrcode` npm-зависимость |

## Контекст

- `kvn-web` уже имеет форму редактирования всех полей конфига (`App.tsx`) и endpoint-ы `/api/config`, `/api/connect`/`/api/disconnect`
- Конфиг сериализуется как JSON на бэкенде и десериализуется на фронтенде — достаточно скопировать/вставить JSON
- `navigator.clipboard.writeText`/`.readText` — стандартное Web API (HTTPS only)
- `qrcode` (npm) — лёгкая библиотека для генерации QR в canvas

## Требования

- RQ-001 Система ДОЛЖНА позволять экспортировать текущий конфиг в буфер обмена одной кнопкой
- RQ-002 Система ДОЛЖНА позволять импортировать конфиг из буфера обмена через текстовое поле
- RQ-003 Система ДОЛЖНА генерировать QR-код с JSON-строкой текущего конфига
- RQ-004 Импорт ДОЛЖЕН парсить JSON и заполнять все поля формы без перезагрузки
- RQ-005 Система ДОЛЖНА игнорировать неизвестные поля в импортированном JSON (forward compat)
- RQ-006 При невалидном JSON импорт ДОЛЖЕН показывать ошибку, не трогая текущую форму

## Допущения

- `navigator.clipboard` доступен в современных браузерах (HTTPS/ localhost)
- `qrcode` npm-пакет стабилен, совместим с текущей версией Vite
- JSON-схема конфига не меняется между версиями в рамках обратной совместимости (неизвестные поля игнорируются)

## Решённые вопросы

1. QR при пустом конфиге — **не показывать**, кнопка QR disabled
2. Save после Import — **подсветить кнопку Save** (не автосохранение)
3. Формат JSON — **минимизированный** (без пробелов/переносов), удобнее для QR/буфера

## Вне scope

- Multi-server профили
- Сохранение истории конфигов
- Import/Export бинарного (YAML) конфига — только JSON через clipboard
- Chrome extension / native share API
