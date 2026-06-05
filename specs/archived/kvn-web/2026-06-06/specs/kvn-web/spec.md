# KVN Web UI (v2rayA-like wrapper)

## Scope Snapshot

- In scope: отдельная команда `kvn-web` (или `kvn-client web`), встраивающая web-интерфейс для управления клиентом в режиме TUN и proxy. Конфиг редактируется через веб-формы, клиент запускается внутри того же процесса (library mode).
- Out of scope: server-side dashboard, мобильное приложение, desktop GUI, REST API для внешних систем.

## Цель

Пользователь, который не хочет разбираться с YAML-конфигами и systemd, получает браузерную панель для настройки и запуска VPN-клиента. Успех: пользователь открывает `http://127.0.0.1:2311`, заполняет 3-4 поля, нажимает "Connect" и получает рабочий туннель.

## Основной сценарий

1. Пользователь скачивает один бинарник `kvn-web` и запускает без аргументов.
2. Браузер открывается на `http://127.0.0.1:2311` (или пользователь открывает сам).
3. На странице — форма: server address, token, mode (TUN/proxy), опционально routing/kill-switch/exclude ranges.
4. Пользователь вводит данные → "Save Config" (сохраняется в `~/.config/kvn/config.json`).
5. "Connect" — внутри процесса стартует `client.New().Run(ctx)`, на странице отображается статус "Connected", live-log.
6. "Disconnect" — cancel контекста, клиент останавливается.
7. При повторном открытии — форма заполнена из сохранённого конфига.

## User Stories

- P1 Story: как пользователь без опыта, я хочу ввести server и token в веб-форму и нажать "Connect", чтобы получить рабочий VPN.
- P2 Story: как пользователь, я хочу видеть статус подключения и лог в реальном времени, чтобы понимать, что происходит.
- P3 Story: как опытный пользователь, я хочу редактировать routing/kill-switch/proxy-mode через UI, чтобы не лезть в YAML.

## MVP Slice

Одна страница: форма + статус + live-log. Connect/Disconnect, конфиг сохраняется в `~/.config/kvn/config.json`.

## First Deployable Outcome

Запускается `kvn-web`, в браузере доступна форма, можно подключиться/отключиться, лог видно.

## Scope

- Новый cmd/web/main.go — entrypoint для `kvn-web`
- Новый internal/webui/ — HTTP server, handlers, SSE для логов, embed статика
- Встраивание React SPA (или Go templates) через `embed.FS`
- Поддержка конфига через веб-формы, сохранение в `~/.config/kvn/config.json`
- Программное использование internal/bootstrap/client через client.New() с конфигом из UI
- TUN и proxy режимы
- Сохранение/загрузка конфига между сессиями

## Контекст

- Уже есть `internal/bootstrap/client` — готовый пакет для программного запуска
- Сейчас клиент требует YAML-конфиг и запускается как демон
- v2rayA — референс-архитектура: один бинарник, web UI, library-mode запуск ядра
- Целевые пользователи: ноутбуки, одноплатники (RPi), временные инстансы

## Требования

- RQ-001 `kvn-web` — один бинарник, без внешних зависимостей (всё встроено через `embed.FS`).
- RQ-002 Web UI доступен на `127.0.0.1:2311` (порт конфигурируемый через --port).
- RQ-003 Поля формы: Server, Token, Mode (TUN/proxy), Proxy Listen, Proxy Auth, Routing (default route, include/exclude CIDR), Kill Switch on/off.
- RQ-004 Кнопки Connect/Disconnect, индикатор статуса, live-log через SSE.
- RQ-005 Конфиг сохраняется через `os.UserConfigDir() + "/kvn/config.yaml"`, совместим с `kvn-client --config`. При старте форма заполняется из него.
| Linux | `~/.config/kvn/config.yaml` |
| macOS | `~/Library/Application Support/kvn/config.yaml` |
| Windows | `%APPDATA%\kvn\config.yaml` |
- RQ-006 TLS verify mode, MTU, IPv6 on/off — опциональные поля.
- RQ-007 Сборка: `go build -o bin/kvn-web ./src/cmd/web` без дополнительных шагов.
- RQ-008 `kvn-client` (systemd-mode) продолжает работать без изменений.

## Вне scope

- Server-side dashboard
- Мобильное приложение (iOS/Android)
- GUI не на Web (TUI, Qt, GTK)
- REST API для automation/third-party
- Multi-user, авторизация в web UI
- HTTPS для web UI (только localhost)

## Критерии приемки

### AC-001 Один бинарник

- Почему это важно: zero dependency install.
- **Given** `go build -o bin/kvn-web ./src/cmd/web`
- **Then** бинарник собран, не требует внешних файлов (статики, конфигов)
- Evidence: `./bin/kvn-web` запускается и слушает порт

### AC-002 Web форма

- Почему это важно: пользователь настраивает клиент без YAML.
- **Given** открыт `http://127.0.0.1:2311`
- **When** пользователь вводит Server, Token, выбирает Mode TUN
- **Then** поля отображаются, форма валидна
- Evidence: curl/браузер возвращает HTML с формой

### AC-003 Подключение через UI

- Почему это важно: Connect в UI запускает клиент внутри процесса.
- **Given** форма заполнена валидными данными
- **When** пользователь нажимает "Connect"
- **Then** клиент запускается, статус меняется на "Connected", логи появляются
- Evidence: SSE стримит логи, индикатор показывает connected

### AC-004 Disconnect

- Почему это важно: пользователь может остановить клиент из UI.
- **Given** клиент подключён
- **When** пользователь нажимает "Disconnect"
- **Then** клиент останавливается, статус "Disconnected"
- Evidence: SSE шлёт done, индикатор disconnected

### AC-005 Сохранение конфига

- Почему это важно: не вводить данные каждый раз.
- **Given** пользователь ввёл и сохранил конфиг
- **When** страница перезагружена
- **Then** форма предзаполнена сохранёнными данными
- Evidence: `cat $(os.UserConfigDir)/kvn/config.yaml` содержит валидный client config

### AC-006 Proxy mode

- Почему это важно: пользователь может выбрать SOCKS5/HTTP proxy вместо TUN.
- **Given** выбран Mode=proxy, заполнен Proxy Listen
- **When** Connect
- **Then** proxy listener запущен, клиент в proxy mode
- Evidence: curl через SOCKS5/HTTP CONNECT работает

### AC-007 Reconnect survives web UI restart

- Почему это важно: при перезапуске `kvn-web` конфиг восстанавливается.
- **Given** конфиг сохранён в `os.UserConfigDir() + "/kvn/config.yaml"`
- **When** `kvn-web` перезапущен и открыта страница
- **Then** форма предзаполнена, можно нажать Connect без ввода
- Evidence: поля server/token/mode заполнены

### AC-008 Совместимость с kvn-client

- Почему это важно: systemd-пользователи не теряют функционал.
- **Given** cmd/client/main.go не изменён
- **When** `go build ./src/cmd/client`
- **Then** бинарник собирается без ошибок
- Evidence: `go build ./src/cmd/client` успешен

## Допущения

- Web UI только на localhost — HTTPS/авторизация не нужны.
- Пользователь имеет браузер (Chromium, Firefox, Safari — современные).
- `kvn-web` запускается на устройстве пользователя (не на сервере).
- Один инстанс — один клиент. Не multi-session.

## Открытые вопросы

1. ~~React SPA через `embed.FS` или vanilla HTML+JS (Go templates)?~~ **React SPA через `embed.FS`**.
2. ~~Использовать существующий config пакет или отдельный web config model?~~ **Существующий `config.ClientConfig`, сохранение в YAML** — конфиг переиспользуется между `kvn-web` и `kvn-client`.
3. ~~Как передавать конфиг в client.New()?~~ **Запись YAML → `config.LoadClientConfig`** — `kvn-web` сохраняет конфиг в `config.yaml` через `os.UserConfigDir()`, затем вызывает `config.LoadClientConfig(path)` — без изменений в `client.New()`.
