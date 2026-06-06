# Архитектурный рефакторинг kvn-ws: безопасность, качество, обслуживаемость

## Scope Snapshot

- In scope: устранение критической OOM-уязвимости в QUIC-транспорте (с выносом MaxMessageSize в конфиг и UI), консолидация дублированного интерфейса StreamConn и логики подключения, рефакторинг монолитного обработчика wsToTun, замена магических чисел на именованные константы/конфиг, миграция `exec.Command("ip")` на netlink.
- Out of scope: добавление нового транспорта, изменение протокола фреймов, переработка схемы конфигурации (YAML structure), рефакторинг серверного bootstrap, переписывание Web UI.

## Цель

Разработчик и ревьюер получают кодовую базу без OOM-уязвимости, с единым контрактом StreamConn, читаемым и тестируемым обработчиком сессий, устранив ~150 строк дублирования, хардкод и внешнюю зависимость от iproute2. Успех фичи измеряется прохождением тестов + lint, отсутствием регрессий в интеграционных тестах и закрытием всех AC.

## Основной сценарий

1. Разработчик открывает репозиторий на ветке `feature/arch-refactoring`.
2. В `transport/quic/conn.go` добавлен лимит `MaxMessageSize` — при msgLen > limit соединение закрывается с ошибкой вместо аллокации.
3. Единый интерфейс `StreamConn` вынесен в `internal/transport/transport.go`, дублирующие объявления удалены.
4. В `bootstrap/client/` появилась общая `dialStream()`, tun.go и proxy.go вызывают её вместо дублирования.
5. `wsToTun()` в `tunnel/session.go` разбита на handleDataFrame/handleCloseFrame/handleProxyFrame с `defer Release()`.
6. Магические числа (timeout, concurrency limit, MTU, маски CIDR) вынесены в `config` или именованные константы.
7. Управление TUN-маршрутизацией переведено с `exec.Command("ip")` на netlink.
8. Все изменения покрыты юнит-тестами; `@sk-task` и `@sk-test` проставлены.

## User Stories

none — рефакторинг без пользовательской истории.

## MVP Slice

QUIC OOM fix (RQ-001) + консолидация StreamConn (RQ-002). Покрывает AC-001, AC-002, AC-003.

## First Deployable Outcome

После первого implementation pass можно прогнать `go test ./... -race` и `golangci-lint run ./...` — все тесты проходят, linter без ошибок.

## Scope

- `internal/transport/quic/conn.go` — лимит на размер сообщения в ReadMessage
- `internal/transport/quic/obfuscated.go` — лимит на размер сообщения после XOR
- `internal/transport/transport.go` — новый файл с единым интерфейсом StreamConn
- `internal/tunnel/stream.go` — удаление дублирующего интерфейса, импорт из transport
- `internal/proxy/stream.go` — удаление дублирующего интерфейса, импорт из transport
- `internal/bootstrap/client/tun.go` — вынос QUIC/WS dial в общую функцию
- `internal/bootstrap/client/proxy.go` — вынос QUIC/WS dial в общую функцию
- `internal/bootstrap/client/helpers.go` — консолидация clientTLSConfig, parseBackoff, paddingSizeOrDefault
- `internal/tunnel/session.go` — декомпозиция wsToTun()
- `internal/tun/tun.go` — замена exec.Command на netlink
- `internal/config/` — новые/изменённые поля конфига для магических чисел
- `internal/config/client.go` — поле `MaxMessageSize` в `ClientConfig` + default в `LoadClientConfig()`
- `internal/webui/handler_config.go` — default в `defaultConfig()`
- `internal/webui/frontend/src/App.tsx` — поле `max_message_size` в TypeScript интерфейсе + форма ввода в секции Advanced

## Контекст

- Протокол QUIC — управляемый пиром; uint32 длины приходит по сети без проверки.
- QUIC obfuscation XOR-ит длину перед отправкой; на приёме длина демутится после XOR.
- Интерфейсы StreamConn объявлены в трёх пакетах и идентичны — добавление нового транспорта (HTTP/3) потребует четвёртого.
- `wsToTun()` — hot path для всех данных сессии; ошибка управления памятью ведёт к утечкам.
- `exec.Command("ip")` на каждом изменении маршрута — fork+exec, латентность ~5-20ms.
- Используется Go 1.25, `github.com/vishvananda/netlink` совместим.

## Требования

- RQ-001 Система ДОЛЖНА отклонять QUIC-сообщения с размером > `MaxMessageSize` (конфигурируемый параметр, default 10MB) без выделения более 64KB буфера.
- RQ-002 Система ДОЛЖНА иметь единый контракт `StreamConn` в пакете `transport`, импортируемый всеми потребителями.
- RQ-003 Клиентский bootstrap ДОЛЖЕН использовать общую функцию `dialStream()` для WebSocket и QUIC подключений, без дублирования кода в tun.go и proxy.go.
- RQ-004 Обработчик входящих фреймов `wsToTun()` ДОЛЖЕН быть декомпозирован на отдельные методы по типу фрейма с `defer Release()`.
- RQ-005 Магические числа ДОЛЖНЫ быть заменены: wsTunnelTimeout, defaultProxyConcurrency и MaxMessageSize — в конфиг; CIDR маски и read limit — в именованные константы.
- RQ-006 Управление TUN-устройством и маршрутизацией ДОЛЖНО использовать netlink вместо `exec.Command("ip")`.

## Вне scope

- Рефакторинг серверного bootstrap/bootstrap/server
- Изменение бинарного протокола фреймов (framing)
- Добавление нового транспорта (HTTP/3, gRPC)
- Переработка схемы конфигурации (YAML structure)
- Оптимизация производительности за пределами netlink migration
- Изменение Admin API (кроме необходимого минимума для MaxMessageSize)
- Миграция на другую версию Go
- Рефакторинг QUIC obfuscation (алгоритм XOR)

## Критерии приемки

### AC-001 QUIC ReadMessage отклоняет oversized сообщения

- Почему это важно: предотвращение OOM-атаки через подконтрольную пиром длину
- **Given** установленное QUIC-соединение, `MaxMessageSize` прочитан из конфига (default 10MB)
- **When** пир отправляет сообщение с `msgLen > MaxMessageSize` (например, 100 MB)
- **Then** `ReadMessage` возвращает ошибку «message too large», соединение закрывается, память не выделена сверх 64KB
- Evidence: unit test с msgLen = MaxMessageSize + 1; интеграционный тест с mock QUIC stream; Web UI отображает поле Max Message Size с default 10MB

### AC-002 ObfuscatedQUICConn имеет аналогичную защиту

- Почему это важно: obfuscated QUIC использует тот же протокол; длина после XOR должна проходить ту же проверку
- **Given** установленное ObfuscatedQUICConn, `MaxMessageSize` из конфига
- **When** пир отправляет сообщение с декодированной `msgLen > MaxMessageSize`
- **Then** `ReadMessage` возвращает ошибку до аллокации полного буфера
- Evidence: unit test для ObfuscatedQUICConn с oversized сообщением

### AC-003 StreamConn объявлен ровно один раз

- Почему это важно: дублирование интерфейса ведёт к расхождению и усложняет добавление транспортов
- **Given** кодовая база после рефакторинга
- **When** выполнен поиск `type StreamConn interface`
- **Then** найдено ровно одно объявление: в `internal/transport/transport.go`
- Evidence: `grep -r "type StreamConn interface" internal/ | wc -l` == 1; `tunnel/stream.go` и `proxy/stream.go` импортируют его из transport

### AC-004 Клиент использует единую dialStream()

- Почему это важно: устранение ~100 строк дублированного кода подключения
- **Given** `bootstrap/client/` после рефакторинга
- **When** tun.go и proxy.go устанавливают соединение с сервером
- **Then** обе функции вызывают общую `dialStream(ctx, cfg) (transport.StreamConn, error)`, а не дублируют QUIC/WS logic
- Evidence: grep по tun.go и proxy.go — блоки `if transport == "quic"` и WebSocket fallback присутствуют только в `dialStream()`; тесты на `dialStream()` покрывают оба транспорта

### AC-005 wsToTun() декомпозирован без утечек

- Почему это важно: монолитная функция с ручным Release() — источник багов при изменениях
- **Given** `tunnel/session.go` после рефакторинга
- **When** фрейм любого типа получен в wsToTun
- **Then** логика обработки каждого типа фрейма выделена в отдельный метод; вызов `Release()` для каждого фрейма гарантирован
- Evidence: в `session.go` нет switch по FrameType внутри wsToTun (или минимальный диспатчер); каждый обработчик начинается с `defer f.Release()`; `go test -race ./internal/tunnel/` проходит без ошибок

### AC-006 Магические числа конфигурируемы

- Почему это важно: оператор может настраивать таймауты и лимиты без изменения кода
- **Given** конфигурационный файл
- **When** оператор задаёт `tunnel.timeout`, `proxy.max_concurrency`, `tunnel.mtu`, `tun.cidr_mask_v4`, `tun.cidr_mask_v6`, `quic.max_message_size`
- **Then** эти значения применяются вместо хардкода; если не заданы — используются defaults из конфига
- Evidence: integration test с кастомным конфигом; grep показывает отсутствие `wsTunnelTimeout = 30 * time.Second`, `defaultProxyConcurrency = 1000`, `maxMessageSize` как константы в Go-файлах (только в config defaults)

### AC-007 TUN маршруты управляются через netlink

- Почему это важно: устранение fork+exec на каждое изменение маршрута, отвязка от iproute2
- **Given** TUN-устройство активировано
- **When** добавляется или удаляется маршрут (default, exclude rule)
- **Then** операция выполняется через `github.com/vishvananda/netlink` без вызова `exec.Command("ip", ...)`
- Evidence: `grep -r 'exec\.Command.*"ip"' internal/tun/` не находит совпадений; `go test -race ./internal/tun/` проходит

## Допущения

- Кодовая база не меняет протокол фреймов — декомпозиция wsToTun сохраняет семантику.
- Зависимость `github.com/vishvananda/netlink` будет добавлена в go.mod; её API стабилен на уровне v1.
- Все изменения совместимы с Go 1.25.
- Интеграционные тесты в `integration/` проверяют сквозной handshake — они должны проходить без изменений.

## Критерии успеха

- SC-001 Все unit-тесты + race detector проходят: `go test -race ./...`
- SC-002 golangci-lint без ошибок: `golangci-lint run ./...`
- SC-003 Дублирование кода сокращено: `cloc --by-file internal/bootstrap/client/` показывает уменьшение строк tun.go + proxy.go на ≥30%
- SC-004 Добавлено ≥5 новых unit-тестов (QUIC OOM, dialStream, декомпозиция wsToTun, netlink, config defaults)

## Краевые случаи

- QUIC OOM: `msgLen = 0` — должно быть разрешено (пустое сообщение).
- QUIC OOM: `msgLen = MaxMessageSize` — должно быть разрешено (boundary).
- netlink: отсутствие прав на создание netlink socket — graceful fallback с ошибкой.
- config: невалидное значение таймаута (отрицательное) — ignored, используется default.
- wsToTun: FrameTypeClose с nil payload — не вызывает panic.

## Открытые вопросы

- **[RESOLVED]** `MaxMessageSize` — конфигурируемый параметр `quic.max_message_size` (default 10MB) в `ClientConfig` + поле в Web UI.
- netlink миграция: затрагивает `internal/tun/tun.go` целиком — риск регрессии в настройке TUN. Возможно, вынести в отдельный PR/spec? Оставлено в рамках этой spec с AC-007.
- Обратная совместимость ObfuscatedQUICConn: после добавления лимита старые клиенты с oversized фреймами будут падать. Это ожидаемо и желаемо.
