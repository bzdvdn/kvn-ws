# Foundation — инфраструктура проекта kvn-ws

## Scope Snapshot

- **In scope:** инициализация Go-модуля, скелет структуры пакетов, Docker-сборка, CI-пайплайн, конфигурация (viper) и логирование (zap).
- **Out of scope:** любой логический код TUN, WebSocket, туннелирования, маршрутизации или аутентификации.

## Цель

Разработчик получает готовую к работе репозиторную структуру: проект компилируется, собирается в Docker-образ, CI проверяет каждый PR. Это foundation для всех последующих фич — без этого этапа ни одна строка логики не может быть написана и протестирована.

## Основной сценарий

1. Разработчик клонирует репозиторий, выполняет `go build ./src/...` — сборка успешна.
2. Выполняет `docker compose up` — сервер и клиент запускаются, логируются startup-события.
3. Открывает PR — CI запускает `go test ./...`, `golangci-lint`, сборку; всё зелёное.

## User Stories

- P1: Разработчик хочет быстро начать писать логику, не тратя время на настройку Go-модуля, Docker и CI.

## MVP Slice

Весь этап foundation — это MVP. Любая незавершённая задача делает невозможным старт следующих этапов. Все AC-* обязательны.

## First Deployable Outcome

`go build ./src/...` успешен, `docker compose up` поднимает процесс, CI зелёный на тестовом PR.

## Scope

1. Go-модуль `github.com/bzdvdn/kvn-ws` с `go.mod` / `go.sum`
2. Директории `src/`, `src/cmd/client/`, `src/cmd/server/`, `src/internal/*`, `src/pkg/api/`
3. Dockerfile multi-stage (build + distroless)
4. docker-compose.yml с сервисами `server` и `client`
5. GitHub Actions CI (test, lint, build)
6. Парсинг YAML-конфигов через `spf13/viper` для client и server
7. JSON-логирование через `uber-go/zap`
8. Пустые main.go с graceful startup/shutdown (SIGTERM)
9. Сборка бинарников в `bin/` через `scripts/build.sh`
10. `.gitignore` для артефактов сборки, IDE, OS-файлов

## Контекст

- ОС разработки: Linux (Ubuntu 22.04+). Docker-образы: `golang:1.22-alpine` для build.
- Все Go-зависимости фиксируются в `go.sum`.
- Docker Compose для локальной разработки; production deployment — отдельная ответственность.
- Предположение: у разработчика установлены Go 1.22+, Docker, Docker Compose.

## Требования

### RQ-001 Go-модуль

Система ДОЛЖНА представлять собой Go-модуль `github.com/bzdvdn/kvn-ws`, собираемый командой `go build ./src/...`.

### RQ-002 Структура директорий

Репозиторий ДОЛЖЕН содержать структуру: `src/` (весь код), `src/cmd/client/main.go`, `src/cmd/server/main.go`, `src/internal/<domain>/*.go`, `src/pkg/api/`.

### RQ-003 Docker-сборка

Система ДОЛЖНА собираться в multi-stage Docker-образ с финальным distroless-образом.

### RQ-004 docker-compose

Репозиторий ДОЛЖЕН содержать `docker-compose.yml`, запускающий server (порт 443) и client (зависит от server).

### RQ-005 CI-пайплайн

Репозиторий ДОЛЖЕН содержать GitHub Actions workflow, запускающий `go test ./...`, `golangci-lint`, `go build ./src/...` на каждый push/PR.

### RQ-006 Конфигурация client

Система ДОЛЖНА читать `configs/client.yaml` через viper с поддержкой env-override (префикс `KVN_CLIENT_`).

### RQ-007 Конфигурация server

Система ДОЛЖНА читать `configs/server.yaml` через viper с поддержкой env-override (префикс `KVN_SERVER_`).

### RQ-008 Логирование

Система ДОЛЖНА логировать startup-события в JSON-формате через `uber-go/zap` в stdout.

### RQ-009 Graceful shutdown

`main.go` клиента и сервера ДОЛЖНЫ обрабатывать SIGTERM/SIGINT и завершаться с корректным закрытием ресурсов.

### RQ-010 Сборка бинарников в bin/

Система ДОЛЖНА предоставлять скрипт `scripts/build.sh`, собирающий бинарники `client` и `server` в `bin/`.

### RQ-011 .gitignore

Репозиторий ДОЛЖЕН содержать `.gitignore`, исключающий `bin/`, `*.exe`, `*.test`, `vendor/`, `.idea/`, `.vscode/`, `*.log`, `*.out`, `__pycache__/`, `dist/`, `coverage/`, `tmp/`.

## Вне scope

- Любая логика TUN, WebSocket, туннелирования, фрейминга, handshake, auth, routing.
- Написание тестов (кроме пустого `go test ./...`).
- Production-деплой (k8s, systemd).
- Prometheus метрики, health endpoint.
- Hot-reload конфига (будет в production hardening).
- `bin/` — только output сборки, не commit; `.gitignore` гарантирует это.

## Критерии приемки

### AC-001 Go-модуль инициализирован и собирается

- Почему это важно: без этого ни строчки кода не будет скомпилировано.
- **Given** свежий checkout репозитория с Go 1.22+
- **When** выполняется `go build ./src/...` из корня
- **Then** команда завершается успешно (exit 0), бинарники `client` и `server` собраны
- Evidence: `go build ./src/...` stdout содержит `ok` или `?`

### AC-002 скелет internal-пакетов существует

- Почему это важно: foundation для DDD-декомпозиции.
- **Given** структура `src/internal/`
- **When** выполняется `ls -d src/internal/*/`
- **Then** существуют директории: `config`, `tun`, `transport/websocket`, `transport/tls`, `transport/framing`, `protocol/handshake`, `protocol/auth`, `protocol/control`, `routing`, `nat`, `session`, `crypto`, `metrics`, `logger`
- Evidence: `ls` выводит все перечисленные пути

### AC-003 Docker multi-stage образ собирается

- Почему это важно: основной способ поставки.
- **Given** Dockerfile в корне репозитория
- **When** выполняется `docker build -t kvn-ws:test .`
- **Then** сборка успешна, образ содержит бинарники `client` и `server`
- Evidence: `docker images kvn-ws:test` показывает образ

### AC-004 docker-compose поднимает оба процесса

- Почему это важно: проверка docker-compose orchestration.
- **Given** docker-compose.yml в корне
- **When** выполняется `docker compose up -d`
- **Then** сервисы `server` и `client` запущены, сервер слушает порт 443
- Evidence: `docker compose ps` показывает `Up` для обоих сервисов

### AC-005 CI пайплайн проходит test/lint/build

- Почему это важно: гарантия качества на каждый PR.
- **Given** GitHub Actions workflow `.github/workflows/ci.yml`
- **When** создаётся PR в main
- **Then** workflow запускает `go test ./...`, `golangci-lint`, `go build ./src/...` и все проходят
- Evidence: зелёный статус checks на PR

### AC-006 конфиг client.yaml парсится и валидируется

- Почему это важно: клиент не может работать без конфигурации.
- **Given** файл `configs/client.yaml` с минимальным контентом
- **When** клиент стартует с `--config configs/client.yaml`
- **Then** конфиг загружен, поля доступны через viper
- Evidence: startup-лог содержит `config loaded: client.yaml`

### AC-007 конфиг server.yaml парсится и валидируется

- Почему это важно: сервер не может работать без конфигурации.
- **Given** файл `configs/server.yaml` с минимальным контентом
- **When** сервер стартует с `--config configs/server.yaml`
- **Then** конфиг загружен, поля доступны через viper
- Evidence: startup-лог содержит `config loaded: server.yaml`

### AC-008 zap-логгер пишет JSON-логи

- Почему это важно: структурированные логи нужны для observability.
- **Given** zap-логгер инициализирован
- **When** вызывается `logger.Info("hello")`
- **Then** stdout получает JSON-строку с полями `level`, `ts`, `msg`
- Evidence: запуск клиента/сервера показывает строки вида `{"level":"info","ts":...,"msg":"..."}`

### AC-009 env override работает для конфига

- Почему это важно: 12-factor app compliance.
- **Given** переменная окружения `KVN_CLIENT_SERVER_ADDR=example.com:443`
- **When** клиент стартует без `server` поля в YAML
- **Then** `config.ServerAddr` равен `example.com:443`
- Evidence: startup-лог показывает `server: example.com:443`

### AC-010 skeleton main.go запускается и завершается

- Почему это важно: проверка graceful shutdown pipeline.
- **Given** собранный бинарник `server`
- **When** запускается `./server & sleep 1 && kill $!`
- **Then** процесс пишет startup-лог, получает SIGTERM, пишет shutdown-лог и завершается exit 0
- Evidence: лог содержит `starting server`, `shutting down`

### AC-011 бинарники собираются в bin/

- Почему это важно: нужен способ собрать нативные бинарники для тестов и дистрибуции вне Docker.
- **Given** `scripts/build.sh` (или `make build`)
- **When** скрипт выполняется
- **Then** `bin/client` и `bin/server` существуют и являются исполняемыми ELF
- Evidence: `file bin/client` выводит `ELF 64-bit LSB executable`

### AC-012 .gitignore исключает артефакты сборки

- Почему это важно: `bin/` и прочие артефакты не должны попадать в репозиторий.
- **Given** файл `.gitignore` в корне
- **When** выполняется `git status` после `scripts/build.sh`
- **Then** `bin/` не отображается в списке отслеживаемых изменений
- Evidence: `git status` не содержит `bin/client` или `bin/server` в секции untracked

## Допущения

- Разработчик использует Linux (Ubuntu 22.04+).
- Go 1.22+ установлен и доступен в PATH.
- Docker Engine 24+ и Docker Compose v2 установлены.
- GitHub репозиторий настроен и имеет Actions enabled.
- Все Go-зависимости (viper, zap) устанавливаются через `go get`.
- YAML-конфиги валидны; неверный YAML — ошибка на старте.

## Критерии успеха

- SC-001: `go build ./src/...` выполняется за <5s.
- SC-002: Docker build завершается за <60s.
- SC-003: CI pipeline полный цикл <3 min.

## Краевые случаи

- **Отсутствие config файла:** main.go ДОЛЖЕН завершаться с fatal-логом.
- **Невалидный YAML:** main.go ДОЛЖЕН завершаться с понятным сообщением об ошибке.
- **Повторный запуск docker-compose:** container already exists — handled by compose.
- **CI запуск без изменений в Go-файлах:** pipeline всё равно проходит (go test ./... для пустых тестов ok).
- **Отсутствие .gitignore:** стандартный `.gitignore` исключает `bin/` и типовые артефакты, но git всё ещё может показывать их при `git add -A` — это ожидаемо, проверка только через `git status`.

## Открытые вопросы

- `none` — foundation не требует уточнений, все решения приняты в конституции и roadmap.
