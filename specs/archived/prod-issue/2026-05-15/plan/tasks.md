# Production Issue Fixes — Задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/dns/cache.go` | T1.1 |
| `src/internal/dns/dns_test.go` | T1.1 |
| `src/internal/routing/domain_matcher.go` | T1.2 |
| `src/internal/routing/domain_matcher_test.go` | T1.2 |
| `go.mod` | T1.3 |
| `Dockerfile` | T1.3 |
| `Dockerfile.test` | T1.3 |
| `.github/workflows/ci.yml` | T1.3 |
| `src/cmd/server/main.go` | T1.4 |
| `src/internal/crypto/crypto.go` | T1.5 |
| `src/internal/config/config_test.go` | T2.1 |
| `src/cmd/server/main_test.go` | T2.1 |
| `src/cmd/client/main_test.go` | T2.1 |
| весь проект | T3.1 |

## Implementation Context

- **Цель MVP:** AC-001..AC-005 (5 critical багов). AC-006 (gap-тесты) — фаза 2.
- **Инварианты/семантика:**
  - `dns/cache.go`: `Get()` под RLock не вызывает `delete()` — только чтение; expired entry удаляется при апгрейде до Lock
  - `domain_matcher.go`: перед `LookupNetIP()` проверять `resolver.Lookup()` (in-memory cache); если кеш не пуст — не делать сетевой вызов
  - Go-версия: `go 1.22` везде (go.mod, Dockerfile, CI)
  - session goroutine: наследует `r.Context()`, а не `context.Background()`
  - crypto stub: файл удаляется (не импортируется нигде)
- **Контракты/протокол:** не меняются. Data model — no-change.
- **Границы scope:** не делаем SOCKS5 alloc-оптимизацию, BoltDB double-open, HEALTHCHECK, non-root user, fuzz-тесты.
- **Proof signals:** `go build ./src/... && go test -race ./src/...` — exit 0.
- **References:** DEC-001..DEC-004.

## Фаза 1: MVP — Critical-баги

Цель: устранить 5 critical/high проблем, чтобы код был безопасен для прода.

- [x] T1.1 **Исправить data race в DNS cache** — в `dns/cache.go:Get()` при обнаружении expired entry не делать `delete` под RLock. Апгрейднуть до Lock перед удалением, либо вернуть miss. Добавить concurrent-тест с `-race`. Touches: `src/internal/dns/cache.go`, `src/internal/dns/dns_test.go`. AC-001.

- [x] T1.2 **Убрать per-packet DNS resolution в domain matcher** — в `domain_matcher.go:Match()` перед `LookupNetIP()` вызвать `resolver.Lookup()` (in-memory cache). Если кеш есть — использовать его, не делать сетевой lookup. Добавить тест с mock resolver и counter. Touches: `src/internal/routing/domain_matcher.go`, `src/internal/routing/domain_matcher_test.go`. AC-002.

- [x] T1.3 **Привести Go-версии к 1.25 (консистентно)** — все файлы (`go.mod`, `Dockerfile`, `Dockerfile.test`, `.github/workflows/ci.yml`) используют Go 1.25. Зависимости (`golang.org/x/sync`, `golang.zx2c4.com/wireguard`) требуют >=1.25. Touches: `go.mod`, `Dockerfile`, `Dockerfile.test`, `.github/workflows/ci.yml`. AC-003.

- [x] T1.4 **Заменить context.Background() на родительский контекст** — в `cmd/server/main.go:handleTunnel()` передать derived context от `r.Context()` в errgroup/goroutine вместо `context.Background()`. Touches: `src/cmd/server/main.go`. AC-004.

- [x] T1.5 **Удалить stub-пакет crypto** — удалить `src/internal/crypto/crypto.go`. Проверить grep-ом, что пакет нигде не импортируется. Touches: `src/internal/crypto/crypto.go`. AC-005.

## Фаза 2: Gap-тесты

Цель: повысить уверенность в config и entry points.

- [x] T2.1 **Добавить gap-тесты для config** — написать `config_test.go` (smoke-загрузка YAML, проверка defaults) + race test для DNS cache. cmd main_test.go отложен — требует TUN/TLS mock, не входит в MVP. Touches: `src/internal/config/config_test.go`, `src/internal/dns/dns_test.go`. AC-006.

## Фаза 3: Верификация

Цель: доказать, что все фиксы работают и нет регрессий.

- [x] T3.1 **Выполнить полную проверку** — `go build ./src/... && go test -count=1 -race ./src/...`. Убедиться что grep `1.25` не даёт совпадений, grep `context.Background()` не даёт совпадений в `cmd/server/main.go`, `crypto/` не существует. Touches: весь проект. AC-001..AC-006.

## Покрытие критериев приемки

- AC-001 → T1.1, T3.1
- AC-002 → T1.2, T3.1
- AC-003 → T1.3, T3.1
- AC-004 → T1.4, T3.1
- AC-005 → T1.5, T3.1
- AC-006 → T2.1, T3.1

## Заметки

- T1.1..T1.5 — независимы, можно параллелить. T1.3 (Go-версия) рекомендуется первым, от него зависит билд остальных.
- После T1.5 удалить директорию `src/internal/crypto/` если пуста.
- T2.1 не блокирует T3.1, но желателен для полноты.
