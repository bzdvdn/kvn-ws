# Foundation — Задачи

## Phase Contract

Inputs: plan, spec, constitution summary.
Outputs: упорядоченные исполнимые задачи с Touches и AC-покрытием.
Stop if: задачи расплывчаты или AC не покрыты.

## Implementation Context

- **Цель MVP:** Go-модуль, DDD-скелет, Docker, CI, viper-config, zap-logger, бинарники в bin/, .gitignore.
- **Инварианты:**
  - Все пакеты `src/internal/*` — только заглушки (`package tun`, пустой файл).
  - `go.mod` фиксирует: `gorilla/websocket`, `spf13/viper`, `uber-go/zap`, `prometheus/client_golang`, `golang.zx2c4.com/wireguard/tun`.
  - Конфиги строго разделены: `ClientConfig` / `ServerConfig`.
  - Префикс env-override: `KVN_CLIENT_` / `KVN_SERVER_`.
- **Границы scope:**
  - Не пишем код в `tun/`, `transport/*`, `protocol/*`, `routing/`, `nat/`, `session/`, `crypto/`, `metrics/` — только заглушки.
  - Не пишем тесты (оставлены на следующие этапы).
- **Proof signals:**
  - `go build ./src/...` exit 0
  - `docker compose ps` → Up для server + client
  - CI зелёный
  - `file bin/client` → ELF

## Surface Map

| Surface | Tasks |
|---------|-------|
| go-module | T1.1 |
| gitignore | T1.2 |
| internal-stubs | T1.3 |
| build-script | T2.1 |
| config-files | T2.2 |
| config-pkg | T2.3 |
| logger-pkg | T3.1 |
| client-main | T3.2 |
| server-main | T3.2 |
| docker-build | T3.3 |
| docker-compose | T3.4 |
| ci-workflow | T4.1 |

## Фаза 1: Go-модуль и структура

Цель: инициализировать модуль, создать .gitignore, развернуть скелет DDD-пакетов.

- [x] T1.1 Инициализировать Go-модуль `github.com/bzdvdn/kvn-ws` и добавить зависимости. Touches: go.mod, go.sum
- [x] T1.2 Создать `.gitignore` с исключением `bin/`, `*.exe`, `*.test`, `vendor/`, `.idea/`, `.vscode/`, `*.log`, `*.out`, `__pycache__/`, `dist/`, `coverage/`, `tmp/`. Touches: .gitignore
- [x] T1.3 Создать скелет DDD-пакетов `src/internal/*` с `package` и пустыми файлами-заглушками. Touches: src/internal/config/, src/internal/tun/, src/internal/transport/websocket/, src/internal/transport/tls/, src/internal/transport/framing/, src/internal/protocol/handshake/, src/internal/protocol/auth/, src/internal/protocol/control/, src/internal/routing/, src/internal/nat/, src/internal/session/, src/internal/crypto/, src/internal/metrics/, src/internal/logger/, src/pkg/api/

## Фаза 2: Инфраструктура сборки

Цель: скрипт сборки бинарников и конфигурация.

- [x] T2.1 Создать `scripts/build.sh`, собирающий `src/cmd/client` и `src/cmd/server` в `bin/`. Touches: scripts/build.sh
- [x] T2.2 Создать шаблоны `configs/client.yaml` и `configs/server.yaml` с минимальными полями. Touches: configs/client.yaml, configs/server.yaml
- [x] T2.3 Реализовать пакет `src/internal/config/` с парсингом YAML через viper, env-override (`KVN_CLIENT_`/`KVN_SERVER_`), отдельные struct `ClientConfig` / `ServerConfig`, валидацию обязательных полей на старте. Touches: src/internal/config/config.go, src/internal/config/client.go, src/internal/config/server.go

## Фаза 3: Точки входа и Docker

Цель: main.go с graceful shutdown, Docker-сборка, docker-compose.

- [x] T3.1 Реализовать пакет `src/internal/logger/` — инициализация zap-logger с JSON-output и конфигурируемым level. Touches: src/internal/logger/logger.go
- [x] T3.2 Реализовать `src/cmd/client/main.go` и `src/cmd/server/main.go` с загрузкой конфига, инициализацией логгера, graceful shutdown (SIGTERM/SIGINT). Touches: src/cmd/client/main.go, src/cmd/server/main.go
- [x] T3.3 Создать multi-stage Dockerfile: build stage (golang:1.22-alpine) → runtime stage (distroless/static), бинарники `client` и `server` в `/usr/local/bin/`. Touches: Dockerfile
- [x] T3.4 Создать `docker-compose.yml` с сервисами `server` (порт 443, монтирование configs/) и `client` (depends_on: server). Touches: docker-compose.yml

## Фаза 4: CI и проверка

Цель: GitHub Actions pipeline, финальная валидация.

- [x] T4.1 Создать `.github/workflows/ci.yml` с шагами: checkout, setup Go, `go test ./...`, `golangci-lint`, `go build ./src/...`, `scripts/build.sh`. Touches: .github/workflows/ci.yml

## Покрытие критериев приемки

- AC-001 -> T1.1, T3.2, T4.1
- AC-002 -> T1.3
- AC-003 -> T3.3, T4.1
- AC-004 -> T3.4
- AC-005 -> T4.1
- AC-006 -> T2.2, T2.3, T3.2
- AC-007 -> T2.2, T2.3, T3.2
- AC-008 -> T3.1, T3.2
- AC-009 -> T2.3, T3.2
- AC-010 -> T3.2
- AC-011 -> T2.1, T4.1
- AC-012 -> T1.2 |

## Заметки

- Фаза 1 + Фаза 2 можно выполнять параллельно (T1.x не зависят от T2.x).
- Фаза 3 зависит от Фазы 1 и 2 (main.go требует config и logger).
- Фаза 4 зависит от Фазы 1–3 (CI проверяет сборку, Docker, тесты).
- Каждая задача самодостаточна: implement-агент читает только задачи + файлы из Touches.
