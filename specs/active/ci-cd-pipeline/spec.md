# CI/CD Pipeline

## Scope Snapshot

- In scope: GitHub Actions workflow для PR checks, Docker сборки, smoke test, lint, и релизного пайплайна.
- Out of scope: Деплой, perf bench, E2E с реальным TUN.

## Цель

Сейчас CI/CD отсутствует — тесты запускаются только вручную. Цель — автоматизировать проверки на каждый PR и релизный процесс.

## Контекст

- Существующий `.github/workflows/ci.yml` есть, но может не покрывать все сценарии.
- `go test` не запускается автоматически.
- Docker-сборка не проверяется в CI.
- Для smoke test нужен Docker-in-Docker или отдельный runner.

## Требования

- RQ-001 GitHub Actions workflow для PR: checkout, setup Go, `go build ./...`, `go test ./... -race`.
- RQ-002 Docker build step: `docker compose build` без ошибок.
- RQ-003 Smoke test step: `docker compose up -d && sleep 5 && docker compose logs client | grep "handshake complete"`.
- RQ-004 Lint step: `golangci-lint run ./...` с конфигом `.golangci.yml`.
- RQ-005 Release workflow: on tag `v*` → build → GitHub Release.

## Критерии приемки

### AC-001 PR check

- **Given** открыт PR в main/master
- **When** CI запускается
- **Then** `go build ./...` и `go test ./... -race` проходят
- Evidence: зелёный CI check на PR

### AC-002 Docker build

- **Given** PR check запущен
- **When** `docker compose build`
- **Then** образ собирается за <5 мин
- Evidence: CI лог без ошибок сборки

### AC-003 Docker smoke test

- **Given** Docker образ собран
- **When** `docker compose up -d && sleep 5`
- **Then** `docker compose logs client | grep "handshake complete"` находит session
- Evidence: CI лог содержит `handshake complete`

### AC-004 Lint

- **Given** Go код изменён
- **When** `golangci-lint run ./...`
- **Then** 0 ошибок
- Evidence: CI лог без lint errors

### AC-005 Release workflow

- **Given** тэг `v*` запушен
- **When** release workflow запущен
- **Then** GitHub Release создан с changelog
- Evidence: Release appears on GitHub

## Вне scope

- Push в Container Registry (Docker Hub, GHCR)
- Deploy стейджинг/продакшен
- Мониторинг и алертинг
- E2E тесты на физическом TUN

## Допущения

- GitHub-hosted runner имеет Docker и docker compose.
- Для smoke test не нужен реальный TUN — handshake проверяется без I/O.
- `golangci-lint` установлен через `go install` или action.

## Открытые вопросы

1. Использовать `docker compose` или `docker compose` в CI? (зависит от версии runner)
2. Нужен ли matrix по Go-версиям (1.22, 1.25)?
3. Нужна ли кэширование Go modules / Docker layers?
