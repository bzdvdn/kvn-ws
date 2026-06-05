# Bootstrap Extraction & God-object Elimination

## Scope Snapshot

- In scope: устранение God-объектов в cmd/client и cmd/server путём выноса бутстрапа, data-path и rate-limiter в internal/.
- Out of scope: изменение протокола, data model, CLI-флагов, конфига или поведения системы.

## Цель

Разработчик получает поддерживаемую кодовую базу: cmd/ — тонкие entrypoints (~25-32 строк), вся логика бутстрапа и data-path — в internal/ пакетах с разделёнными файлами по concern-ам. Успех: go build/vet/test проходят, команды работают как раньше.

## Основной сценарий

1. Разработчик открывает cmd/client/main.go и видит 25 строк — вызов client.New().Run(ctx).
2. Вся TUN-логика, reconnect, proxy, kill-switch — в internal/bootstrap/client/.
3. Вся server-логика (handlers, admin API, SIGHUP) — в internal/bootstrap/server/.
4. Rate-limiter извлечён в internal/ratelimit/.
5. Data-path (Session, данные фреймы) — в internal/tunnel/.

## User Stories

- P1 Story: как разработчик, я хочу видеть в cmd/ только entrypoint, чтобы быстро понять, с чего начинается программа.
- P2 Story: как разработчик, я хочу, чтобы client data-path был в одном месте (internal/tunnel), а не копирован между client и server main.go.
- P3 Story: как разработчик, я хочу видеть rate-limiter как отдельный package, чтобы его можно было unit-тестировать независимо.

## MVP Slice

Вынос bootstrap в internal/bootstrap/{client,server} и tunnel Session в internal/tunnel/ — чтобы cmd/ были тонкими.

## First Deployable Outcome

После первого pass: `go build ./...`, `go vet ./...`, `go test ./... -race` проходят, и функциональность не изменилась (gatetest routing pass).

## Scope

- Вынос бутстрапа из cmd/client/main.go → internal/bootstrap/client/ (client.go, tun.go, proxy.go, reconnect.go, killswitch.go)
- Вынос бутстрапа из cmd/server/main.go → internal/bootstrap/server/ (server.go, handler.go)
- Вынос data-path Session → internal/tunnel/session.go
- Вынос rate-limiter → internal/ratelimit/ratelimit.go
- Удаление мёртвого пакета pkg/api/
- Приватизация SessionStreams.M → m
- Удаление глобальной proxySem, перенос в Session
- nil-guards для опциональных полей Session (sm, collectors, proxyStreams)
- Чистка SIGHUP reload (type assertion fix)
- Proxy goroutine nil-guard в FrameTypeProxy

## Контекст

- cmd/client/main.go был ~787 строк, cmd/server/main.go ~632 строк — God-объекты, сложно поддерживать.
- Data-path дублировался между client и server.
- proxySem была глобальной переменной — нарушало конституцию.

## Требования

- RQ-001 cmd/ entrypoints должны быть ≤50 строк каждый.
- RQ-002 internal/bootstrap/client/ должен содержать всю TUN/reconnect/proxy/kill-switch логику.
- RQ-003 internal/tunnel/session.go должен быть единым data-path для client и server.
- RQ-004 Все пакеты должны собираться (`go build ./...`) и проходить `go vet ./...`.
- RQ-005 Все acceptance-тесты и routing-тесты должны проходить без изменений.

## Вне scope

- Изменение CLI-флагов, конфига, протокола, data model.
- Рефакторинг internal/nat, internal/routing, internal/session, internal/acl.
- Добавление новой функциональности.

## Критерии приемки

### AC-001 cmd/ — тонкие entrypoints

- Почему это важно: разработчик сразу видит точку входа без лишнего шума.
- **Given** исходники в cmd/
- **When** разработчик открывает cmd/client/main.go или cmd/server/main.go
- **Then** каждый файл содержит ≤50 строк и только вызов bootstrap-пакета
- Evidence: `wc -l src/cmd/client/main.go` ≤ 50, `wc -l src/cmd/server/main.go` ≤ 50

### AC-002 bootstrap-пакеты существуют

- Почему это важно: вся логика бутстрапа лежит в internal/ с разделением по concern-ам.
- **Given** репозиторий
- **When** разработчик открывает internal/bootstrap/
- **Then** существуют internal/bootstrap/client/ и internal/bootstrap/server/
- Evidence: `ls src/internal/bootstrap/client/` и `src/internal/bootstrap/server/` содержат файлы

### AC-003 Data-path переиспользуется

- Почему это важно: Session (data-path) не дублируется между client и server.
- **Given** репозиторий
- **When** разработчик ищет tunToWS, wsToTun
- **Then** они удалены, весь data-path в internal/tunnel/session.go
- Evidence: grep не находит tunToWS/wsToTun, Session.Run() вызывается из обоих bootstrap-пакетов

### AC-004 Сборка и тесты

- Почему это важно: рефакторинг не сломал функциональность.
- **Given** репозиторий после рефакторинга
- **When** `go build ./...`, `go vet ./...`, `go test -race ./src/...`, `go run src/cmd/gatetest/ --mode routing`
- **Then** все проходят без ошибок
- Evidence: exit code 0

### AC-005 Мёртвый код удалён

- Почему это важно: pkg/api/ больше не используется, не должен сбивать с толку.
- **Given** репозиторий
- **When** разработчик ищет pkg/api/
- **Then** пакет удалён
- Evidence: `ls src/pkg/` не содержит api/

### AC-006 Нет глобального proxySem

- Почему это важно: глобальное состояние нарушает конституцию проекта.
- **Given** репозиторий
- **When** поиск `var proxySem`
- **Then** proxySem нет как глобальной переменной
- Evidence: grep не находит `var proxySem` вне Session struct

### AC-007 SIGHUP reload работает

- Почему это важно: server перезагружает конфиг по SIGHUP.
- **Given** server запущен с конфигом
- **When** послан SIGHUP
- **Then** конфиг перезагружается без panic
- Evidence: нет type assertion panic в startSighupHandler

## Допущения

- Поведение системы не меняется — только структура кода.
- Все существующие тесты покрывают критический функционал.
- Proxy mode и TUN mode продолжают работать одинаково.

## Открытые вопросы

- none
