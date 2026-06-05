# Bootstrap Extraction & God-object Elimination План

## Phase Contract

Inputs: spec, конституция, существующая кодовая база.
Outputs: план реализации, data model (не требуется — только перестановка кода).
Stop if: spec не определяет границы каждого извлекаемого пакета.

## Цель

Убрать God-объекты из cmd/, вынеся ~1500 строк бутстрапа и data-path в internal/ пакеты с файловой структурой по concern-ам. Функциональность остаётся идентичной — меняется только расположение кода и устранение глобального состояния.

## MVP Slice

Вынос server-бутстрапа и data-path Session — cmd/server/main.go сокращается с ~632 до ~32 строк, cmd/client/main.go начинает использовать tunnel.NewSession.

## First Validation Path

```sh
go build ./... && go vet ./... && go test -race ./src/... && go run ./src/cmd/gatetest/ --mode routing
```

## Scope

- internal/bootstrap/server/ — Server struct, New, Run, buildMux, startSighupHandler, health handlers, handleTunnel
- internal/bootstrap/client/ — Client struct, New, Run, tun.go (reconnectLoop + runSession), proxy.go (runProxyMode), reconnect.go (backoff), killswitch.go (nftables)
- internal/tunnel/session.go — Session struct извлечён из cmd/client, nil-guards, SetTunRouter, SetInterruptibleRead
- internal/ratelimit/ratelimit.go — IPRateLimiter + SessionPacketLimiter
- internal/proxy/stream.go — SessionStreams.M → m, NewSessionStreams()
- Удаление pkg/api/
- proxySem глобальная → поле Session
- SIGHUP type assertion fix
- Proxy goroutine nil-guard

## Implementation Surfaces

- src/cmd/client/main.go — существующий, урезается
- src/cmd/server/main.go — существующий, урезается
- src/internal/bootstrap/client/ — новый пакет (5 файлов)
- src/internal/bootstrap/server/ — новый пакет (2 файла)
- src/internal/tunnel/session.go — новый пакет/файл
- src/internal/ratelimit/ratelimit.go — новый пакет (извлечение из существующего)
- src/internal/proxy/stream.go — существующий, приватное поле + конструктор
- src/pkg/api/ — удаляется

## Bootstrapping Surfaces

- src/internal/bootstrap/client/ — директория + 5 файлов
- src/internal/bootstrap/server/ — директория + 2 файла
- src/internal/tunnel/ — директория + session.go
- src/internal/ratelimit/ — директория + ratelimit.go

## Влияние на архитектуру

- cmd/ становятся тонкими entrypoints — чистая архитектура
- Session переиспользуется между client и server — DRY
- Глобальное proxySem устранено — без глобального состояния (конституция)
- rate-limiter отдельный package — тестируемый

## Acceptance Approach

- AC-001 (тонкие cmd/) → `wc -l` проверка
- AC-002 (bootstrap пакеты) → `ls` проверка
- AC-003 (data-path) → grep удалённых tunToWS/wsToTun
- AC-004 (сборка/тесты) → go build/vet/test/gatetest
- AC-005 (мёртвый код) → ls pkg/
- AC-006 (глобальный proxySem) → grep
- AC-007 (SIGHUP) → code review

## Данные и контракты

- Data model не меняется.
- API контракты не меняются.
- Структура Session расширяется полем proxySem (вместо глобальной).
- `data-model.md` не требуется — нет изменений модели данных.

## Стратегия реализации

### DEC-001 Извлечение по пакетам, а не по файлам

  Why: каждый concern (client/server/tunnel/ratelimit) становится своим Go package — изоляция зависимостей.
  Tradeoff: больше boilerplate (package declarations, экспорт символов) — но это стандартная Go-практика.
  Affects: все новые internal/ пакеты.
  Validation: go build ./... проходит.

### DEC-001 Session — nil-safe для опциональных полей

  Why: proxy и collectors опциональны для Server-сессий (server не использует proxyStreams), nil-guard предотвращает panic.
  Tradeoff: дополнительная проверка в хот-пате (FrameTypeProxy), но это один if на фрейм.
  Affects: internal/tunnel/session.go, internal/proxy/stream.go.
  Validation: go test -race проходит.

### DEC-003 Файл-per-concern для client bootstrap

  Why: 5 файлов (client, tun, proxy, reconnect, killswitch) вместо одного monolithic файла — навигация.
  Tradeoff: больше файлов, но каждый <200 строк.
  Affects: internal/bootstrap/client/.
  Validation: code review.

## Incremental Delivery

### MVP (server bootstrap + tunnel session)

- internal/bootstrap/server/ создан, cmd/server/main.go → 32 строки.
- internal/tunnel/session.go создан, используется сервером.
- internal/ratelimit/ извлечён.
- Проверка: go build/vet/test.

### Итеративное расширение

- internal/bootstrap/client/ — перенос client-logic (tun, proxy, reconnect, killswitch).
- cmd/client/main.go → 25 строк.
- Приватизация SessionStreams.M, proxySem → Session.
- nil-guards, SIGHUP fix, proxy goroutine nil-guard.

## Порядок реализации

1. server bootstrap (независим, самый простой).
2. tunnel session (data-path, нужен и client, и server).
3. ratelimit extraction (независим).
4. client bootstrap (зависит от tunnel session).
5. Чистки: proxySem, SIGHUP, nil-guards, pkg/api удаление.

## Риски

- Риск 1: сломать data-path при переносе Session.
  Mitigation: все тесты проходят, gatetest routing подтверждает routing-логику.
- Риск 2: пропустить импорт/символ при переносе.
  Mitigation: go build ./... ловит несоответствия.
- Риск 3: race condition в proxySem (была глобальной).
  Mitigation: proxySem теперь поле Session — per-session, не shared.

## Rollout и compatibility

- Полностью backward-compatible — меняется только структура кода.
- CLI-аргументы, конфиг, протокол не меняются.
- Специальных rollout-действий не требуется.

## Проверка

- `go build ./...` — все пакеты собираются.
- `go vet ./...` — статический анализ.
- `go test -race ./src/...` — тесты + race detector.
- `go run ./src/cmd/gatetest/ --mode routing` — routing.
- Code review: grep tunToWS, grep proxySem, wc -l cmd/, ls pkg/api.

## Соответствие конституции

- нет конфликтов. Устранение глобального proxySem прямо соответствует конституции ("без глобального мутабельного состояния").
