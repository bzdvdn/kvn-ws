# Repository Map — kvn-ws

Go stack: `go.mod` at root, all source under `src/`.

## Entry Points
- `src/cmd/client/main.go` — CLI entrypoint: KVN-over-WS client
- `src/cmd/server/main.go` — CLI entrypoint: KVN-over-WS server

## Top-Level Code
- `src/internal/config/` — YAML config parsing (viper + env override)
- `src/internal/logger/` — structured JSON logging (zap)
- `src/internal/tun/` — TUN device abstraction (stub)
- `src/internal/transport/websocket/` — WS dial/accept (stub)
- `src/internal/transport/tls/` — TLS config (stub)
- `src/internal/transport/framing/` — binary frame protocol (stub)
- `src/internal/protocol/handshake/` — Client/Server Hello (stub)
- `src/internal/protocol/auth/` — token/jwt/basic auth (stub)
- `src/internal/protocol/control/` — PING, CLOSE, ROUTE_UPDATE (stub)
- `src/internal/routing/` — packet routing (stub)
- `src/internal/nat/` — MASQUERADE/SNAT (stub)
- `src/internal/session/` — session management + IP pool (stub)
- `src/internal/crypto/` — app-layer encryption (stub)
- `src/internal/metrics/` — Prometheus metrics (stub)
- `src/pkg/api/` — public API package (stub)

## Key Paths
- `configs/client.yaml` — client config template
- `configs/server.yaml` — server config template
- `Dockerfile` — multi-stage Docker build
- `docker-compose.yml` — local dev orchestration
- `scripts/build.sh` — native binary build to `bin/`
- `.github/workflows/ci.yml` — GitHub Actions CI pipeline

## Where To Edit
- Core tunnel logic — `src/internal/tun/`, `src/internal/transport/*`, `src/internal/protocol/*`
- Routing/rules — `src/internal/routing/`, `src/internal/nat/`
- Session/auth — `src/internal/session/`, `src/internal/protocol/auth/`
- Config changes — `src/internal/config/`
- Logging/metrics — `src/internal/logger/`, `src/internal/metrics/`

## Excluded
- `.speckeep/**` — excluded from indexing
- `specs/archived/**` — excluded from indexing
- `bin/` — build output
- `vendor/` — vendored deps (not used)
