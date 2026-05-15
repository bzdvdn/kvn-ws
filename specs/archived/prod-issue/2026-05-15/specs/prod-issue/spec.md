# Production Issue Fixes

## Scope Snapshot

- In scope: Исправление критических и высоких проблем production readiness, выявленных в ходе аудита codebase.
- Out of scope: Архитектурные изменения новых фич; тесты и документация — только в объёме, необходимом для доказательства исправления.

## Цель

Разработчик получает production-ready код без data race, катастрофической деградации DNS-маршрутизации, утечки горутин и неработающего билда. Успех измеряется прохождением `go test -race ./src/...`, билдом на Go 1.22/1.23, и отсутствием регрессий в существующих тестах.

## Основной сценарий

1. Разработчик видит 5 critical/high-проблем, зафиксированных в аудите.
2. Для каждой проблемы вносится минимальное целевое изменение, устраняющее root cause.
3. Все существующие тесты проходят, `go test -race` не падает, билд собирается.
4. Data race в DNS кеше воспроизводится и фиксится; domain matcher не делает DNS lookup на каждый пакет.

## MVP Slice

Закрыть AC-001..AC-005 (все критические проблемы). AC-006 (gap-тесты) — опционально в этом спеке.

## First Deployable Outcome

После реализации — `go build ./src/... && go test -race ./src/...` проходит на Go 1.22. CI зелёный.

## Scope

1. Fix data race в `dns/cache.go` — delete под RLock
2. Fix per-packet DNS resolution в `routing/domain_matcher.go` — добавить DNS cache lookup перед обращением к DNS
3. Fix Go-версии во всём проекте — привести go.mod, Dockerfile, CI к консистентной версии (1.22)
4. Fix context.Background() — заменить на родительский контекст в session goroutine
5. Fix crypto — убрать stub-пакет или добавить noop-реализацию с panic на вызов (чтобы не было ложного чувства защиты)
6. Написать gap-тесты на config, cmd/main (end-to-end smoke)

## Контекст

- Репозиторий требует Go 1.22+; 1.25 — несуществующая версия
- `dns/cache.go` использует `sync.RWMutex`, delete мутирует мапу под RLock — это data race по Go memory model
- `domain_matcher.Match()` вызывается на каждый пакет из `RoutePacket` — DNS lookup на каждый пакет делает фичу непригодной для реального использования
- `cmd/server/main.go` создаёт горутины с `context.Background()` — если сессия не закрывается корректно, горутины не завершаются

## Требования

- RQ-001 Система ДОЛЖНА проходить `go test -race ./src/...` без data race errors
- RQ-002 Система ДОЛЖНА собираться на Go 1.25.x (требование зависимостей)
- RQ-003 DNS cache ДОЛЖЕН корректно обрабатывать expired entries без data race
- RQ-004 Domain matcher НЕ ДОЛЖЕН выполнять DNS resolution на каждый вызов Match() — должен использовать кеш
- RQ-005 Session goroutines ДОЛЖНЫ наследовать родительский контекст для корректного завершения
- RQ-006 Go-версии в go.mod, Dockerfile, CI и CONSTITUTION ДОЛЖНЫ быть консистентны
- RQ-007 Stub-пакет crypto ДОЛЖЕН быть либо реализован, либо помечен как неиспользуемый

## Вне scope

- Реализация app-layer encryption (crypto) — заменяется маркировкой stub'а как неиспользуемого
- Performance-оптимизации (sync.Pool для SOCKS5, buffer allocations)
- Docker HEALTHCHECK и non-root user
- Fuzz-тесты для протокола
- Полное покрытие тестами всех пакетов — только gap-тесты для доказательства исправления
- CI-пайплайн (docker build, e2e, coverage) — отдельная spec

## Критерии приемки

### AC-001 DNS cache data race устранён

- Почему это важно: data race — undefined behavior; под нагрузкой приводит к panic/corruption
- **Given** DNS cache с expired entry
- **When** горутина A вызывает `Get()` и обнаруживает expired entry, одновременно горутина B вызывает `Set()`
- **Then** ни одна горутина не падает с race condition, `go test -race` проходит
- Evidence: `go test -race ./src/internal/dns/...` — 0 failures

### AC-002 Domain matcher не делает DNS lookup на каждый пакет

- Почему это важно: per-packet DNS resolution убивает пропускную способность (секунды на пакет)
- **Given** domain matcher с 3 доменами
- **When** вызывается `Match()` 100 раз подряд
- **Then** DNS resolver вызывается не более 3 раз (а не 300)
- Evidence: тест с mock resolver и counter вызовов; benchmark

### AC-003 Go-версии консистентны

- Почему это важно: иначе билд падает в CI и Docker
- **Given** файлы: go.mod, Dockerfile, Dockerfile.test, .github/workflows/ci.yml
- **When** выполняется `grep 'golang:1\.25\|go 1\.25'` по файлам
- **Then** все файлы консистентно используют Go 1.25 (ни один не указывает другую версию)
- Evidence: `grep -rn '1\.25' Dockerfile Dockerfile.test .github/workflows/ci.yml` находит только ожидаемые строки; `go build ./src/...` успешен

### AC-004 Session goroutines наследуют родительский контекст

- Почему это важно: context.Background() создаёт неуправляемые горутины → утечка
- **Given** `handleTunnel()` в `cmd/server/main.go`
- **When** устанавливается новое WS-соединение
- **Then** горутины forward-цикла используют контекст запроса (или derived), не `context.Background()`
- Evidence: код не содержит `context.Background()`; код содержит `ctx` от `r.Context()` или `errgroup`

### AC-005 Stub-пакет crypto не вводит в заблуждение

- Почему это важно: пустой package crypto создаёт ложное чувство защищённости
- **Given** пакет `src/internal/crypto/`
- **When** код импортирует или вызывает crypto
- **Then** либо: (a) crypto полностью удалён, либо (b) все экспортируемые функции валидны, либо (c) пакет пуст и не импортируется нигде
- Evidence: `go vet ./src/...` не находит мёртвый код; grep по импортам не показывает crypto

### AC-006 Существующие тесты не регрессированы

- Почему это важно: изменения не должны ломать работающую функциональность
- **Given** вся тестовая база (17 test files)
- **When** `go test ./src/...` после всех изменений
- **Then** все тесты проходят
- Evidence: `go test -count=1 ./src/...` — exit code 0

## Допущения

- Все изменения делаются минимальными патчами, без рефакторинга соседнего кода
- Go 1.22.x — целевая версия (последняя стабильная на момент фикса)
- Тесты AC-001..AC-005 пишутся в рамках доработки, если их нет
- Для domain matcher используется существующий `dns.Resolver` с TTL-кешем

## Краевые случаи

- **Пустой DNS кеш**: Get на пустой мапе не должен паниковать
- **Domain matcher без доменов**: Match всегда false, без DNS вызовов
- **Session без контекста запроса**: если контекст недоступен — использовать `context.TODO()`, не `Background()`
- **Crypto не импортируется**: удалить файл; если импортируется — понять цепочку и разорвать

## Открытые вопросы

- `none`
