DEC-001: scripts/build.sh (bash) для сборки в bin/
DEC-002: Docker multi-stage: golang:1.22-alpine → distroless/static
DEC-003: GitHub Actions + golangci-lint для CI
DEC-004: ClientConfig/ServerConfig — отдельные struct на компонент
Surfaces:
- go-module: go.mod, go.sum
- client-main: src/cmd/client/main.go
- server-main: src/cmd/server/main.go
- config-pkg: src/internal/config/*.go
- logger-pkg: src/internal/logger/*.go
- internal-stubs: src/internal/tun/, src/internal/transport/*, src/internal/protocol/*, src/internal/routing/, src/internal/nat/, src/internal/session/, src/internal/crypto/, src/internal/metrics/
- docker-build: Dockerfile
- docker-compose: docker-compose.yml
- ci-workflow: .github/workflows/ci.yml
- config-files: configs/client.yaml, configs/server.yaml
- build-script: scripts/build.sh
- gitignore: .gitignore
