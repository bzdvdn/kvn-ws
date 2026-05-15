# Production Issue Fixes — План

## Цель

Минимальными патчами устранить 5 critical/high проблем аудита и 1 gap тестов, не трогая архитектуру и не рефакторя соседний код. Каждый фикс изолирован в своём пакете; единственный cross-cutting change — версия Go.

## MVP Slice

AC-001..AC-005 (все критические проблемы). AC-006 (gap-тесты для config/cmd) — зависимость от AC-003 (Go-версия должна быть зафиксирована до запуска CI-тестов).

## First Validation Path

```bash
go mod edit -go 1.22 && go build ./src/... && go test -race ./src/...
```

После каждого фикса — `go test -race ./src/internal/<пакет>/...`.

## Scope

1. `dns/cache.go` — замена RLock на Lock в expired-path
2. `routing/domain_matcher.go` — вставка dns-кеша перед реальным lookup
3. Go-версия: `go.mod`, `Dockerfile`, `Dockerfile.test`, `.github/workflows/ci.yml` → 1.22
4. `cmd/server/main.go` — `context.Background()` → `r.Context()` (или derived)
5. `src/internal/crypto/` — удалить файл stub (пакет не импортируется нигде)
6. Тесты: `config_test.go` (smoke load/save round-trip) + `main_test.go` (flflag parse)

## Implementation Surfaces

### AC-001 — DNS cache data race

- **Surfaces:** `src/internal/dns/cache.go` (существующий)
- **Изменение:** в `Get()` — при обнаружении expired entry не делать `delete` под RLock. Вместо этого вернуть `nil, false` и позволить `Set()` перезаписать. Либо апгрейдить RLock → Lock перед delete.
- **Решение:** второй вариант (RLock → unlock → Lock) — минимальное изменение без логики гонки.
- **Тест:** добавить concurrent Get/Set в `dns_test.go` с `-race`.

### AC-002 — Domain matcher per-packet DNS

- **Surfaces:** `src/internal/routing/domain_matcher.go` (существующий)
- **Изменение:** перед вызовом `r.resolver.LookupNetIP()` проверить `r.resolver.Lookup()` (кеш). Если кеш есть — использовать его, не делать сетевой lookup.
- **Тест:** mock resolver с counter в `domain_matcher_test.go`.

### AC-003 — Go-версии

- **Surfaces:** `go.mod`, `Dockerfile`, `Dockerfile.test`, `.github/workflows/ci.yml`
- **Изменение:** все файлы приводятся к Go 1.25 консистентно. Зависимости (`golang.org/x/sync`, `golang.zx2c4.com/wireguard`) требуют >=1.25, поэтому downgrade невозможен.
- **Тест:** `grep -rn '1\.25' Dockerfile Dockerfile.test .github/workflows/ci.yml` — все три содержат 1.25; CI на `go-version: "1.25"`.

### AC-004 — Session context

- **Surfaces:** `src/cmd/server/main.go`
- **Изменение:** передать `ctx` из `r.Context()` в `errgroup`/goroutine вместо `context.Background()`.
- **Тест:** существующие WS-тесты (dataframe_test.go) проходят без изменений — это доказывает, что сессия корректно завершается.

### AC-005 — Crypto stub

- **Surfaces:** `src/internal/crypto/crypto.go`
- **Изменение:** удалить файл. Проверить `grep -r '"github.com/bzdvdn/kvn-ws/src/internal/crypto"' src/` — импортов нет.
- **Тест:** `go build ./src/...` проходит.

### AC-006 — Gap-тесты

- **Surfaces:** `src/internal/config/config_test.go`, `src/cmd/server/main_test.go`, `src/cmd/client/main_test.go` (новые)
- **config_test.go:** smoke — LoadServerConfig/LoadClientConfig с реальным YAML, проверка полей по умолчанию
- **main_test.go:** флаги парсятся, config загружается (без реального TUN/TLS — mock)

## Bootstrapping Surfaces

`none` — все изменения в существующих файлах. Новые файлы: только `*_test.go`.

## Влияние на архитектуру

- Локальное: правим до 5 строк в каждом целевом файле.
- Нет миграций, нет изменения контрактов, нет rollout-последствий.

## Acceptance Approach

| AC | Подход | Surfaces | Наблюдение |
|----|--------|----------|-----------|
| AC-001 | конкурентный тест с `go test -race` | `dns/cache.go`, `dns/dns_test.go` | 0 races |
| AC-002 | mock resolver + counter | `routing/domain_matcher.go`, `domain_matcher_test.go` | вызовов resolver = число доменов, не число пакетов |
| AC-003 | grep + билд | `go.mod`, `Dockerfile*`, `ci.yml` | нигде нет `1.25` |
| AC-004 | code review + тесты WS | `cmd/server/main.go` | нет `context.Background()` в handleTunnel |
| AC-005 | grep, build | `src/internal/crypto/` | файл удалён, билд ок |
| AC-006 | `go test ./src/...` | новые `*_test.go` | все тесты + новые проходят |

## Данные и контракты

- Data model не меняется. `data-model.md` — no-change stub.
- API/event contracts не меняются.

## Стратегия реализации

### DEC-001 Минимальные патчи, без рефакторинга

- **Why:** каждый фикс касается отдельного бага; рефакторинг вокруг увеличивает риск регрессии без добавления ценности.
- **Tradeoff:** код остаётся неидеальным (напр. в SOCKS5 alloc, BoltDB double-open), но становится безопасным для прода.
- **Affects:** 5-6 файлов, 1-5 строк изменений каждый.
- **Validation:** `go test -race ./src/...` pass.

### DEC-002 DNS cache — RLock → Lock при expired

- **Why:** альтернатива (вернуть miss без delete) приводит к тому, что expired entry висит в мапе до перезаписи — потенциальная утечка памяти. Lock при обнаружении expired — атомарно и безопасно.
- **Tradeoff:** чуть больше contention на записи, но DNS cache — холодный path (редко инвалидируется).
- **Affects:** `dns/cache.go`.
- **Validation:** `go test -race ./src/internal/dns/...`.

### DEC-003 Domain matcher — использовать существующий DNS resolver cache

- **Why:** `dns.Resolver` уже имеет TTL-кеш; domain matcher просто не обращается к нему перед реальным Lookup. Добавить один вызов `r.resolver.Lookup()` (который попадает в кеш) перед `LookupNetIP()`.
- **Tradeoff:** дополнительный `RLock` на каждый пакет — наносекунды, незначительно.
- **Affects:** `routing/domain_matcher.go`.
- **Validation:** counter-based test + benchmark.

### DEC-004 Go 1.22 как единая версия

- **Why:** 1.22 — последняя стабильная; 1.25 не существует. В `go.mod` уже стоит `go 1.22`-совместимый код (нет 1.23-only features).
- **Tradeoff:** нет — Go 1.25 уже июль 2025, стабильный релиз. Все deps его поддерживают.
- **Affects:** `go.mod`, `Dockerfile`, `Dockerfile.test`, `ci.yml`.
- **Validation:** `go build ./src/...` на Go 1.25.

## Incremental Delivery

### MVP — AC-001..AC-005 (все critical-баги)

- AC-003 (Go-версия) — **первым**, от него зависит билд.
- Затем AC-001, AC-002, AC-004, AC-005 — в любом порядке (независимые пакеты).
- Критерий: `go build ./src/... && go test -race ./src/...` проходит.

### Итеративное расширение — AC-006 (gap-тесты)

- После фикса всех багов.
- Не блокирует deploy, но повышает уверенность в config и entry points.

## Порядок реализации

1. **AC-003** — Go-версия (иначе ничего не собирается).
2. **AC-001** + **AC-002** + **AC-004** + **AC-005** — параллельно (разные файлы/пакеты).
3. **AC-006** — gap-тесты, последовательно (config, потом cmd).

## Риски

- **Риск:** AC-002 (domain matcher) может потребовать больше изменений, если `dns.Resolver` не injected в matcher.
  **Mitigation:** у domain matcher уже есть поле `resolver` типа `dns.Resolver` — кеш доступен через него.
- **Риск:** CI на Go 1.25 может быть недоступен на ubuntu-latest.
  **Mitigation:** `actions/setup-go@v5` поддерживает `go-version: "1.25"` с Aug 2025. Резерв — `"1.24"` с downgrade deps.
- **Риск:** AC-004 — контекст запроса может быть отменён до завершения хендшейка.
  **Mitigation:** создать derived context с `context.WithCancel`, который родитель — `r.Context()`. Отмена родителя каскадируется; дочерний можно отменить отдельно.

## Rollout и compatibility

Специальных rollout-действий не требуется. Изменения обратно совместимы:
- DNS cache: семантика `Get()` не меняется (возвращает `nil, false` для expired).
- Domain matcher: логика Match() не меняется — только добавлен кеш перед lookup.
- Go-версия: код не использует 1.23+ features.
- Context: сессия завершается не медленнее, чем раньше.
- Crypto stub: удаление не ломает импорты (их нет).

## Проверка

| Шаг | Команда | Проверяет AC |
|-----|---------|-------------|
| 1 | `go build ./src/...` | AC-003, AC-005 |
| 2 | `go test -race ./src/internal/dns/...` | AC-001 |
| 3 | `go test -race ./src/internal/routing/...` | AC-002 |
| 4 | `go test -race ./src/cmd/... 2>&1 \| grep -c "context.Background"` | AC-004 (0 вхождений) |
| 5 | `go test -count=1 ./src/...` | AC-006 |
| 6 | `grep -rn '1\.25' go.mod Dockerfile Dockerfile.test .github/workflows/ci.yml; echo $?` | AC-003 (exit 1 = нет совпадений) |

## Соответствие конституции

Нет конфликтов. Все изменения следуют Clean Architecture (фиксы внутри своих пакетов, без cross-boundary dependencies).
