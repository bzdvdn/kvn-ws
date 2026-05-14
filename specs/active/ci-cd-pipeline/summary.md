## Goal

Настроить GitHub Actions CI/CD: автопроверки на каждый PR, Docker-сборка, smoke test, lint, и автоматический release workflow.

## Acceptance Criteria

| ID | Description | Verification |
|----|-------------|-------------|
| AC-001 | PR check | `go build + go test ./... -race` проходит |
| AC-002 | Docker build | Multi-stage образ собирается без ошибок |
| AC-003 | Docker smoke test | `docker compose up` + grep handshake |
| AC-004 | Lint | `golangci-lint run` без ошибок |
| AC-005 | Release | Пуш тега `v*` triggers build + GitHub Release |

## Out of Scope

- Deploy в cloud (GCR, Docker Hub push)
- Integration/E2E тесты на реальном TUN
- Performance benchmarks
