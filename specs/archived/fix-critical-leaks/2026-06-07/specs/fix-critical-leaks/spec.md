# Исправление критических утечек и ошибок (Critical Leaks & Errors Fix)

## Scope Snapshot

- In scope: исправление goroutine leak в TUN reader/proxy listener/RouteDirect, context leak в QUIC dial/DNS/webui broadcast, deadlock risk в SessionManager, отсутствие timeout в BoltDB, утечка time.After, swallowed errors, type assertion вместо errors.As, дублирование кода (TLS config/backoff) и per-packet аллокации без sync.Pool.
- Out of scope: рефакторинг архитектуры (разделение больших файлов), введение новых фич, изменение API контрактов, миграция схемы БД.

## Цель

Разработчик и эксплуататор получают стабильный KVN-туннель без утечки ресурсов при переподключениях, корректную обработку ошибок без silent data loss и предсказуемое завершение процессов. Успех фичи измеряется прохождением `go test -race ./...` + golangci-lint и отсутствием grow-паттернов горутин под нагрузкой.

## Основной сценарий

1. Разработчик запускает тесты с race detector и линтер — все зелёные.
2. При циклических reconnect (TUN/proxy/WS) количество горутин не растёт монотонно.
3. При shutdown сервера/клиента все горутины завершаются, BoltDB корректно закрывается.
4. При ошибках кодирования/декодирования стек логирует проблему вместо молчаливого игнора.

## MVP Slice

Исправление 4 критических goroutine leak + контекстные утечки + BoltDB timeout. Дублирование кода и sync.Pool — второй приоритет (P2).

## First Deployable Outcome

После имплементации: `go test -race ./...` проходит, `golangci-lint run` без новых предупреждений. Можно вручную проверить reconnect-цикл и shutdown скриптами из `scripts/`.

## Scope

- Исправление goroutine leak: `tunnel/session.go:tunReadInterruptible`, `proxy/listener.go:handleClient`, `bootstrap/client/proxy.go:RouteDirect`
- Исправление context propagation: `transport/quic/dial.go`, `bootstrap/client/tun.go:DNS`, `routing/domain_matcher.go:DNS`, `webui/server.go:broadcast`
- Исправление deadlock risk: `session/session.go:expireIdle/SetCancel`
- BoltDB timeout: `session/bolt.go:bolt.Open`
- Утечка time.After: `bootstrap/client/reconnect.go:sleepWithContext`
- Swallowed errors (json.Encode, frame.Encode, AddrFromSlice) — все файлы с `_ =` + `AddrFromSlice`
- Замена type assertion на `errors.As`: `bootstrap/client/proxy.go`
- Дубликаты: TLS config + backoff parsing — вынести в shared helper в `bootstrap/client/`
- Per-packet буферы: добавить `sync.Pool` для 4KB буферов в proxy stream и TUN reader

## Контекст

- Проект на Go 1.25 — доступны `errors.As`, `any`, `sync.Map`
- Существующий `sync.Pool` в `transport/framing` — паттерн для повторного использования
- BoltDB должен открываться с `Timeout` для предотвращения hang при блокировке
- Конституция запрещает глобальное мутабельное состояние — пакет `config` с `envPrefixForWarning` должен быть защищён мьютексом или переделан

## Требования

- RQ-001 При `ctx.Done()` все горутины TUN reader должны завершаться в течение 1 секунды без утечек
- RQ-002 Proxy listener ДОЛЖЕН иметь верхний лимит одновременных соединений (через semaphore)
- RQ-003 RouteDirect ДОЛЖЕН корректно дожидаться завершения обоих `io.Copy` через `errgroup` или `WaitGroup`
- RQ-004 QUIC dial ДОЛЖЕН принимать `ctx context.Context` от родителя и отменяться при cancel контекста
- RQ-005 DNS lookups (client tun + domain matcher) ДОЛЖНЫ использовать переданный контекст вместо `context.Background()`
- RQ-006 WebUI broadcast горутины ДОЛЖНЫ завершаться при shutdown сервера
- RQ-007 BoltDB open ДОЛЖЕН иметь `Timeout: 1*time.Second`
- RQ-008 `sleepWithContext` ДОЛЖЕН использовать `time.NewTimer` с `defer timer.Stop()` вместо `time.After`
- RQ-009 Все `_ = json.NewEncoder(w).Encode(...)` и `frame.Encode()` ДОЛЖНЫ логировать ошибки
- RQ-010 SessionManager.expireIdle/SetCancel ДОЛЖНЫ использовать отдельный lock для `cancelFuncs` во избежание deadlock
- RQ-011 Type assertion `err.(net.Error)` ДОЛЖЕН быть заменён на `errors.As(err, &netErr)`
- RQ-012 TLS config + backoff parsing в bootstrap/client ДОЛЖНЫ быть вынесены в shared helper-файл
- RQ-013 4KB буферы в proxy stream и TUN reader ДОЛЖНЫ браться из `sync.Pool`

## Вне scope

- Разделение больших файлов (server.go 518 строк, session.go 449 строк) — только P3
- Рефакторинг `interface{}` → `any` — только косметика
- `rate.Limiter` → `sync.Map` оптимизация — performance P3
- SetReadDeadline per-packet — требует понимания влияния на gorilla/websocket
- Frame payload zeroing before pool return — отдельный security review

## Критерии приемки

### AC-001 Нет монотонного роста горутин при reconnect TUN

- Почему это важно: утечка горутин ведёт к OOM при длительной работе с переподключениями
- **Given** запущенный клиент KVN с TUN-туннелем
- **When** TUN-соединение пересоздаётся 10 раз подряд
- **Then** количество горутин после 10-го reconnect не превышает baseline +-5%
- Evidence: `runtime.NumGoroutine()` до и после reconnect-цикла

### AC-002 Proxy listener не превышает лимит соединений

- Почему это важно: атакующий/нагрузка не может исчерпать ресурсы процесса через proxy listener
- **Given** запущенный клиент в proxy-режиме
- **When** 2000 одновременных соединений к SOCKS5/HTTP CONNECT listener
- **Then** не более 1000 горутин `handleClient` активны, остальные блокируются или получают ошибку
- Evidence: тест с параллельными соединениями и проверкой через `runtime.NumGoroutine`

### AC-003 RouteDirect не оставляет висящих горутин

- Почему это важно: маршруты Direct создаются/уничтожаются динамически
- **Given** настроенное правило RouteDirect
- **When** 5 concurrent direct-соединений создаются и разрываются
- **Then** после завершения всех соединений нет горутин из RouteDirect в активном состоянии
- Evidence: `pprof.Lookup("goroutine")` показывает 0 goroutines из proxy.go

### AC-004 QUIC dial отменяется при cancel контекста

- Почему это важно: клиент не зависает намертво при недоступном QUIC-сервере
- **Given** клиент конфигурирован на QUIC-транспорт
- **When** контекст родителя cancelled через 100ms
- **Then** QUIC dial завершается ошибкой в течение 200ms
- Evidence: тест с timeout и assert на `ctx.Err()`

### AC-005 DNS lookup использует переданный контекст

- Почему это важно: DNS-запросы не должны переживать shutdown сессии
- **Given** сессия клиента с правилом на DNS-имя
- **When** контекст сессии cancelled во время DNS lookup
- **Then** lookup прерывается немедленно
- Evidence: юнит-тест с `context.WithTimeout` и проверкой `errors.Is(err, context.DeadlineExceeded)`

### AC-006 WebUI broadcast горутины завершаются при shutdown

- Почему это важно: webui сервис не должен оставлять фоновые горутины после Stop
- **Given** запущенный webui.Server
- **When** `ctx.Done()` получен (сигнал shutdown)
- **Then** обе broadcast-горутины (logs + status) завершены в течение 1s
- Evidence: `sync.WaitGroup` + тест с shutdown

### AC-007 BoltDB открывается с timeout

- Почему это важно: процесс не должен block forever при заблокированном bolt-файле
- **Given** файл `sessions.db` уже открыт другим процессом
- **When** второй процесс вызывает `bolt.Open(path, 0o600, nil)`
- **Then** `bolt.Open` возвращает ошибку через ≤ 1s
- Evidence: тест с lock-файлом и assert на timeout error

### AC-008 Нет утечки time.After в reconnect

- Почему это важно: каждая reconnect-попытка не должна оставлять активный timer в куче
- **Given** `sleepWithContext(ctx, d)` вызвана
- **When** контекст отменён до истечения d
- **Then** timer остановлен и собран GC
- Evidence: проверка через `Timer.Stop()` + GC trace

### AC-009 Ошибки кодирования логируются, а не игнорятся

- Почему это важно: silent data loss при ошибках сериализации
- **Given** любой endpoint с `json.NewEncoder(w).Encode(...)`
- **When** `Encode` возвращает ошибку
- **Then** ошибка записана в `logger.Error()`
- Evidence: `grep` по репозиторию — 0 вхождений `_ = json.NewEncoder`
- **Given** `handshake.EncodeAuthError()` / `frame.Encode()`
- **When** кодирование фрейма не удалось
- **Then** ошибка записана в лог перед return
- Evidence: код-ревью

### AC-010 SessionManager не имеет deadlock в expireIdle/SetCancel

- Почему это важно: expireIdle не должен блокироваться навсегда
- **Given** SessionManager с активными сессиями
- **When** `expireIdle` запущен и одновременно `SetCancel` вызван для той же сессии
- **Then** обе операции завершаются без deadlock
- Evidence: тест с parallel calls и лимитом по времени (3s timeout)

### AC-011 Type assertion заменена на errors.As

- Почему это важно: wrapped errors не ловятся через bare type assertion
- **Given** `if netErr, ok := err.(net.Error)` в `proxy.go:295`
- **When** ошибка обёрнута через `fmt.Errorf("...%w...")`
- **Then** `errors.As(err, &netErr)` корректно находит `net.Error`
- Evidence: юнит-тест с wrapped error

### AC-012 TLS config + backoff parsing вынесены в shared helper

- Почему это важно: изменение TLS/backoff не требует правки в двух местах
- **Given** два файла (`proxy.go`, `tun.go`) в `bootstrap/client/`
- **When** добавляется новый транспортный канал
- **Then** TLS config и backoff конфигурация берутся из одного helper-функции
- Evidence: grep по `bootstrap/client/` на наличие дублирующихся блоков

### AC-013 4KB буферы берутся из sync.Pool

- Почему это важно: снижение GC pressure под нагрузкой
- **Given** hot path в proxy stream и TUN reader
- **When** буфер `make([]byte, 4096)` нужен для чтения
- **Then** буфер получен из `sync.Pool`, а после использования возвращён
- Evidence: тест производительности с `-benchmem` показывает снижение аллокаций

## Допущения

- Существующий `tunReadInterruptible` может быть заменён на постоянный reader с select + close
- Semaphore в proxy listener использует тот же паттерн, что уже есть в `session.go:proxySem`
- BoltDB timeout = 1s достаточно для всех сценариев
- Разделение lock в SessionManager через отдельный `sync.Mutex` для `cancelFuncs`
- В `proxy.go:295` не больше одного места с type assertion на `net.Error`

## Критерии успеха

- SC-001 `go test -race ./...` проходит без data race
- SC-002 `golangci-lint run` без новых предупреждений
- SC-003 Количество горутин стабильно после 10 reconnect-циклов: не более baseline + 5%

## Краевые случаи

- TUN device не поддерживает interruptible Read (блокирующий syscall) — закрывать device на ctx cancel
- Proxy listener на Windows не имеет `net.Error.Timeout()` — запасной путь
- BoltDB первый open (файла нет) — создаётся, не timeout
- context.Background() в DNS refresh domain_matcher — подменяется на корневой контекст при инициализации (AcceptRootCtx или подобный)

## Открытые вопросы

1. Замена `tunReadInterruptible` — закрывать TUN device по ctx cancel или использовать pool reader? Предпочтение: убрать горутину на пакет, сделать один reader с `select` + close device
2. WebUI broadcast — какие именно goroutine? `state.broadcastLogs` и `state.broadcastStatus` — нужно подтверждение что это все
3. `sync.Pool` для 4KB буферов — размер достаточен для всех сценариев? MTU стандартный 1500, но может быть до 9000 (jumbo frames)
