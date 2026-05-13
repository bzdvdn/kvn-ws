# Foundation — План

## Цель

Создать рабочую инфраструктуру репозитория kvn-ws: Go-модуль, структуру DDD-пакетов, Docker-сборку, CI, конфигурацию и логирование. После этого плана разработчик может писать код в `src/internal/*`, собирать бинарники, запускать через Docker и проходить CI.

## MVP Slice

Весь foundation — это единый MVP. Любая незавершённая часть блокирует старт следующих этапов. Все 12 AC должны быть закрыты.

## First Validation Path

```bash
# 1. Сборка
go build ./src/...
# 2. Бинарники
./scripts/build.sh && ls -la bin/
# 3. Docker
docker compose up -d && docker compose ps
# 4. CI (имитация)
go test ./... && go vet ./... && golangci-lint run ./src/...
```

## Scope

1. Go-модуль и корневая структура (`go.mod`, `src/`, `configs/`, `scripts/`, `docs/`, `examples/`)
2. Все пакеты `src/internal/*` с заглушками (package declaration + пустые файлы)
3. Dockerfile + docker-compose.yml
4. `.github/workflows/ci.yml`
5. `configs/client.yaml`, `configs/server.yaml` + код парсинга в `src/internal/config/`
6. `src/internal/logger/` с zap-инициализацией
7. `src/cmd/client/main.go`, `src/cmd/server/main.go` с graceful shutdown
8. `scripts/build.sh` и `.gitignore`

**Вне scope любые изменения:** `src/internal/tun/`, `transport/*`, `protocol/*`, `routing/`, `nat/`, `session/`, `crypto/`, `metrics/`, `pkg/api/` — только заглушки.

## Implementation Surfaces

| Surface | Файлы | Тип | Зачем |
|---------|-------|-----|-------|
| go-module | `go.mod`, `go.sum` | новый | корень модуля |
| client-main | `src/cmd/client/main.go` | новый | entrypoint клиента |
| server-main | `src/cmd/server/main.go` | новый | entrypoint сервера |
| config-pkg | `src/internal/config/config.go`, `client.go`, `server.go` | новый | парсинг + валидация YAML |
| logger-pkg | `src/internal/logger/logger.go` | новый | zap-инициализация |
| internal-stubs | `src/internal/<domain>/*.go` | новый | 14 пакетов-заглушек |
| docker-build | `Dockerfile` | новый | multi-stage build |
| docker-compose | `docker-compose.yml` | новый | локальный запуск |
| ci-workflow | `.github/workflows/ci.yml` | новый | PR-проверки |
| config-files | `configs/client.yaml`, `configs/server.yaml` | новый | шаблоны конфигов |
| build-script | `scripts/build.sh` | новый | сборка в `bin/` |
| gitignore | `.gitignore` | новый | исключение артефактов |

## Bootstrapping Surfaces

Все surfaces — bootstrapping; foundation не требует предсуществующей структуры.

## Влияние на архитектуру

- Закладывается DDD-структура `src/internal/<domain>` — foundation для всех будущих фич.
- `go.mod` фиксирует зависимости: `gorilla/websocket`, `spf13/viper`, `uber-go/zap`, `prometheus/client_golang`, `golang.zx2c4.com/wireguard/tun` — все сразу в go.mod (без кода использования).
- Никакого влияния на runtime-логику — foundation layer.

## Acceptance Approach

| AC | Подход | Surface | Наблюдение |
|----|--------|---------|------------|
| AC-001 | `go mod init` + `go build ./src/...` | go-module | exit 0 |
| AC-002 | `mkdir -p src/internal/*` | internal-stubs | `ls -d` все пути |
| AC-003 | `docker build` multi-stage | docker-build | образ в `docker images` |
| AC-004 | `docker compose up -d` | docker-compose | `docker compose ps` → Up |
| AC-005 | `.github/workflows/ci.yml` push/PR | ci-workflow | зелёные checks |
| AC-006 | `--config configs/client.yaml` | config-pkg, client-main | лог `config loaded` |
| AC-007 | `--config configs/server.yaml` | config-pkg, server-main | лог `config loaded` |
| AC-008 | zap → stdout JSON | logger-pkg | `{"level":"info",...}` |
| AC-009 | `KVN_CLIENT_SERVER_ADDR=...` | config-pkg | лог с подменённым значением |
| AC-010 | SIGTERM → shutdown | client-main, server-main | лог `shutting down` |
| AC-011 | `scripts/build.sh` → `bin/` | build-script | `file bin/client` → ELF |
| AC-012 | `.gitignore` → `git status` | gitignore | `bin/` не в untracked |

## Данные и контракты

Foundation не добавляет persisted entities, не меняет API/event contracts. Data model — `no-change`.

## Стратегия реализации

### DEC-001: scripts/build.sh (bash), не Makefile

- **Why:** bash-скрипт не требует установки make, работает везде где есть Go. Минимум внешних зависимостей.
- **Tradeoff:** нет dependency tracking (всегда полная пересборка) — для foundation это приемлемо.
- **Affects:** `scripts/build.sh`, CI workflow.
- **Validation:** `scripts/build.sh && bin/client --help` exit 0.

### DEC-002: Docker multi-stage: golang:1.22-alpine → distroless/static

- **Why:** минимальный размер образа (~15MB vs ~300MB для alpine). Никаких shell-атак.
- **Tradeoff:** distroless не имеет shell — отладка через `docker exec` невозможна. В production это плюс.
- **Affects:** `Dockerfile`.
- **Validation:** `docker images kvn-ws:test` размер <50MB.

### DEC-003: GitHub Actions + golangci-lint

- **Why:** бесплатно для публичных репозиториев, стандарт индустрии.
- **Tradeoff:** vendor-lock на GitHub — при смене платформы CI придётся переписывать.
- **Affects:** `.github/workflows/ci.yml`.
- **Validation:** PR с любым изменением → зелёный статус.

### DEC-004: ClientConfig / ServerConfig — отдельные struct

- **Why:** каждый компонент имеет свою конфигурацию. Единая struct Config порождает лишние поля и путаницу.
- **Tradeoff:** дублирование общих полей (log level, log format) — решается композицией.
- **Affects:** `src/internal/config/client.go`, `src/internal/config/server.go`, `src/internal/config/config.go`.
- **Validation:** `client --config configs/client.yaml` парсит только client-поля.

## Incremental Delivery

### MVP (единый инкремент)

Все 12 AC закрываются одним инкрементом. Разделение на подзадачи — ортогонально:
1. Go-модуль + структура (AC-001, AC-002)
2. .gitignore + scripts/build.sh (AC-011, AC-012)
3. Dockerfile + docker-compose.yml (AC-003, AC-004)
4. config + viper (AC-006, AC-007, AC-009)
5. logger + zap (AC-008)
6. main.go + graceful shutdown (AC-010)
7. CI workflow (AC-005)

## Порядок реализации

1. **Go-модуль + .gitignore + структура** — самый базовый слой, без него ничего не собирается.
2. **scripts/build.sh** — возможность собирать и проверять.
3. **configs + logger** — infrastructure-слой, нужен main.go.
4. **main.go + graceful shutdown** — точка входа, логирует конфиг.
5. **Dockerfile + docker-compose** — контейнеризация.
6. **CI workflow** — завершающий штрих.

Параллельно: 1+2, 3+4, 5+6.

## Риски

- **golangci-lint не установлен в CI:** mitigation — `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` в workflow.
- **distroless не содержит ca-certificates:** mitigation — `COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/` в Dockerfile.
- **viper найдёт ключ из соседнего config struct:** mitigation — `viper.SetEnvPrefix("KVN_CLIENT")` / `KVN_SERVER`.

## Rollout и compatibility

Foundation не имеет rollout — это чистый bootstrap. После merge ветки `feature/foundation` в main любой разработчик получает готовую инфраструктуру.

## Проверка

- Automated: `go test ./...` (пустые тесты в каждом пакете), `go vet ./...`
- Manual: `docker compose up -d && docker compose ps && docker compose logs`
- CI: `.github/workflows/ci.yml` — полный pipeline
- Все AC-* и DEC-* подтверждаются automated checks или однократным manual run.

## Соответствие конституции

- нет конфликтов: foundation полностью соответствует конституции (DDD-структура, Docker, Go, Clean Architecture).
