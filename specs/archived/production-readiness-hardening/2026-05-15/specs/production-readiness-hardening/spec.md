# Production Readiness Hardening — kvn-ws

## Scope Snapshot

- **In scope**: устранение критических и высокоприоритетных проблем, блокирующих эксплуатацию kvn-ws в production-окружении (утечки ресурсов, уязвимости, операционные пробелы).
- **Out of scope**: новый функционал (crypto, connection migration, HTTP3), рефакторинг архитектуры, расширение тестового покрытия.

## Цель

Разработчик/оператор получает сервер и клиент kvn-ws, которые можно запустить в ограниченном production-режиме (до 500 сессий, uptime >72ч) без риска утечки горутин, fd, OOM, и без уязвимости к slow-loris. Успех измеряется отсутствием регрессий по тестам + прохождением 24h stability test под нагрузкой.

## Основной сценарий

1. Команда применяет патчи по 6 критическим и 6 высокоприоритетным направлениям.
2. Для каждого направления — соответствующие юнит-тесты и trace-маркеры `@sk-task` / `@sk-test`.
3. CI-пайплайн дополнен `go test -race` и gosec.
4. Docker-compose переведён с `privileged: true` на минимальные capabilities.
5. После verify создаётся git-ветка `feature/production-readiness-hardening`, изменения проходят review.

## User Stories

- **P1** — Оператор: сервер не умирает от утечки горутин/fd после 24ч работы под 100 сессиями.
- **P2** — Оператор: при slow-loris атаке http-сервер не падает (ReadHeaderTimeout).
- **P3** — Разработчик: может подключить pprof для диагностики инцидента на production.
- **P4** — Оператор: метрики и логи пригодны для алертинга (нет `log.Printf`, есть latency histograms).

## MVP Slice

Закрыть AC-001–AC-006 (утечки горутин, slow-loris, BatchWriter, SessionManager, proxyStreams, log.Printf). Этого достаточно для деплоя на ограниченную нагрузку.

## First Deployable Outcome

После имплементации MVP Slice: сервер проходит `go test -race ./...`, `go vet ./...`, и 1-часовой smoke-test с 10 параллельными клиентами. Результат — `docker-compose up` без privileged mode, health endpoint проверяет зависимости.

## Scope

- `src/cmd/server/main.go` — deadline-ы, sm.Stop(), proxyStreams cleanup
- `src/internal/session/session.go` — SessionManager lifecycle
- `src/internal/session/bolt.go` — log.Printf → zap
- `src/internal/routing/router.go`, `rule_set.go`, `domain_matcher.go` — log.Printf → zap
- `src/internal/transport/websocket/websocket.go` — log.Printf → zap, BatchWriter sync.Once
- `src/internal/transport/websocket/dataframe.go` — BatchWriter Close
- `src/internal/config/server.go` — convertRawTokens safe type assertions
- `src/internal/dns/resolver.go` — context timeout
- docker-compose.yml, examples/docker-compose.yml — privileged → capabilities
- `.github/workflows/ci.yml` — race detector, gosec

## Контекст

- Проект — MVP/бета, архитектура валидна, но реализация имеет классические «болезни роста» Go-стартапов: незакрытые горутины, sync.Map без cleanup, отсутствие таймаутов на write.
- Зависимости: BoltDB диск IO, nftables, TUN device — это накладывает ограничения на контейнеризацию (capabilities вместо privileged).
- Предположение: максимальная нагрузка на первом этапе — 500 одновременных сессий, 1000 proxy streams, ~100 Mbps throughput.

## Требования

- RQ-001 Все WebSocket Read/Write операции ДОЛЖНЫ иметь deadline (ReadDeadline / WriteDeadline).
- RQ-002 http.Server ДОЛЖЕН содержать `ReadHeaderTimeout` не более 20с.
- RQ-003 BatchWriter.Close() ДОЛЖЕН быть идемпотентным (sync.Once).
- RQ-004 SessionManager.Stop() ДОЛЖЕН вызываться при graceful shutdown сервера.
- RQ-005 proxyStreams sync.Map ДОЛЖНА очищаться при завершении сессии: все сохранённые net.Conn закрываются.
- RQ-006 Весь `log.Printf` ДОЛЖЕН быть заменён на zap-логгер, переданный через dependency injection.
- RQ-007 `convertRawTokens` ДОЛЖНА использовать безопасные type assertions с `ok`.
- RQ-008 DNS resolution ДОЛЖЕН использовать контекст с таймаутом не более 10с.
- RQ-009 Docker-контейнеры ДОЛЖНЫ использовать минимальные capabilities вместо `privileged: true`.
- RQ-010 CI-пайплайн ДОЛЖЕН включать `go test -race` и gosec.
- RQ-011 `/debug/pprof` эндпоинты ДОЛЖНЫ быть зарегистрированы на админ-сервере.
- RQ-012 `/health` ДОЛЖЕН проверять состояние BoltDB и TUN устройства.

## Вне scope

- Полное тестовое покрытие config/proxy/logger — отложено до отдельной spec.
- E2E-шифрование (crypto) — отложено.
- Connection migration / resume session.
- QUIC/HTTP3.
- WebSocket compression performance monitoring.
- Rate limiting для `/metrics`.

## Критерии приемки

### AC-001 WebSocket deadlines защищают от зависших peer-ов

- Почему это важно: заблокированная горутина не освобождает ресурсы сессии, ведёт к утечке.
- **Given** сервер с установленными SetWriteDeadline/SetReadDeadline
- **When** peer перестаёт отвечать (network partition)
- **Then** Read/Write завершаются с timeout ошибкой в течение заданного таймаута, горутина не блокируется навсегда
- **Evidence**: `TestWebSocketDeadlines` в `websocket_test.go` с mock peer, который зависает; таймаут срабатывает < deadline + epsilon

### AC-002 ReadHeaderTimeout предотвращает slow-loris

- Почему это важно: без ReadHeaderTimeout любой клиент может держать соединение открытым, отправляя заголовки по одному байту.
- **Given** http.Server с ReadHeaderTimeout=20s
- **When** клиент шлёт HTTP-заголовки медленнее 20с
- **Then** сервер закрывает соединение по таймауту
- **Evidence**: `TestReadHeaderTimeout` в `server` пакете (или integration test)

### AC-003 BatchWriter.Close идемпотентен

- Почему это важно: повторный Close на закрытом канале вызывает panic.
- **Given** BatchWriter после первого Close()
- **When** Close() вызывается повторно
- **Then** второй вызов — no-op, без паники и без записи в закрытый канал
- **Evidence**: `TestBatchWriterCloseIdempotent` в `dataframe_test.go`

### AC-004 SessionManager.Stop() вызывается при shutdown

- Почему это важно: reclaimLoop горутина течёт при каждом запуске сервера.
- **Given** сервер получает SIGTERM
- **When** graceful shutdown завершается
- **Then** `sm.Stop()` вызван, reclaimLoop завершён
- **Evidence**: код server/main.go содержит `defer sm.Stop()` после `sm.Start()`

### AC-005 proxyStreams очищаются при завершении сессии

- Почему это важно: накопленные net.Conn в global sync.Map ведут к утечке fd.
- **Given** активная сессия с proxy streams
- **When** сессия завершается (disconnect/timeout)
- **Then** все net.Conn в proxyStreams закрыты и удалены из мапы
- **Evidence**: `TestProxyStreamsCleanup` — открыть N proxy streams, закрыть сессию, проверить что все conn закрыты

### AC-006 log.Printf заменён на zap во всех пакетах

- Почему это важно: log.Printf не даёт structured output, уровней, тегов — непригоден для production observability.
- **Given** код без log.Printf вызовов (кроме main.go startup)
- **When** любое событие логируется
- **Then** оно проходит через zap.Logger с корректным уровнем и structured полями
- **Evidence**: `grep -r 'log\.Printf' src/` возвращает только main.go разрешённые места

### AC-007 convertRawTokens безопасен к типу

- Почему это важно: type assertion без ok паникует на битом конфиге.
- **Given** конфиг с некорректным типом для токена (например string вместо map)
- **When** сервер загружает конфиг
- **Then** возвращается ошибка, а не panic
- **Evidence**: `TestConvertRawTokensUnsafeTypes` в config_test.go

### AC-008 DNS resolution имеет таймаут

- Почему это важно: DNS может заблокировать горутину навсегда при недоступном резолвере.
- **Given** routing engine разрешает домен
- **When** DNS-сервер не отвечает
- **Then** запрос завершается по таймауту (10с)
- **Evidence**: `TestDNSResolveTimeout` в dns_test.go

### AC-009 Docker без privileged mode

- Почему это важно: privileged: true — антипаттерн безопасности, даёт контейнеру полный доступ к ядру.
- **Given** docker-compose конфигурация
- **When** контейнер запускается
- **Then** он использует только CAP_NET_ADMIN + CAP_SYS_ADMIN + device /dev/net/tun
- **Evidence**: `grep 'privileged' docker-compose.yml` пуст; контейнер работает с capabilities

### AC-010 CI с race detector и gosec

- Почему это важно: race detector ловит data races, gosec — уязвимости. Без них — вслепую.
- **Given** CI pipeline
- **When** на каждый push/PR
- **Then** run: `go test -race ./...` и `gosec ./...`
- **Evidence**: `.github/workflows/ci.yml` содержит соответствующие шаги

### AC-011 pprof доступен на админ-сервере

- Почему это важно: без pprof диагностика production инцидентов (утечки памяти, горутин) затруднена.
- **Given** сервер работает
- **When** запрос к `/debug/pprof/`
- **Then** возвращаются стандартные pprof данные
- **Evidence**: curl `/debug/pprof/heap?debug=1` возвращает heap profile

### AC-012 Health check проверяет зависимости

- Почему это важно: /health должен отражать реальное состояние сервиса, а не только факт запуска.
- **Given** сервер работает
- **When** запрос к `/health`
- **Then** ответ включает статус BoltDB (open/closed) и TUN устройства
- **Evidence**: `TestHealthEndpoint` в admin_test.go

## Допущения

- Все изменения сохраняют обратную совместимость конфигурации и API.
- Тесты до и после изменений проходят (`go test -race ./...`).
- Zap-логгер передаётся через DI (конструктор/параметр), а не через глобальный синглтон.
- `Add(_)` в prometheus гистограммах можно добавить без брейкинга метрик (новые метрики — новые 이름).

## Критерии успеха

- SC-001 uptime >72ч под 100 сессиями без роста числа горутин (pprof).
- SC-002 error rate < 0.1% при 50 реконнектах клиентов в минуту.
- SC-003 Memory leak = 0 (flatline на pprof heap после 2ч стабильной нагрузки).

## Краевые случаи

- Пустой config / nil zap logger — panic в production.
- SIGHUP reload во время закрытия сессии — гонка между старым и новым config.
- mTLS verify failure + одновременно reconnect — порядок операций.
- BoltDB file lock при повторном запуске (если предыдущий экземпляр не закрыл БД).

## Открытые вопросы

- Нужна ли миграция BoltDB схемы? (сейчас схема — ключ=сессия, значение=IP, миграций нет)
- Какой дефолтный таймаут для SetWriteDeadline? (предложение: 30с для туннеля, 10с для proxy)
- Стоит ли добавить в `/health` uptime и version?
