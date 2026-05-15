# Repository Map — kvn-ws

Go stack: `go.mod` at root, all source under `src/`.

## Entry Points
- `src/cmd/client/main.go` — CLI entrypoint: KVN-over-WS client
- `src/cmd/server/main.go` — CLI entrypoint: KVN-over-WS server
- `src/cmd/gatetest/main.go` — routing gate test program (AC-010 simulation)
- `src/cmd/stability/main.go` — stability gate / soak test program (AC-012)

## Top-Level Code
- `src/internal/config/` — YAML config parsing (viper + env override)
- `src/internal/logger/` — structured JSON logging (zap)
- `src/internal/tun/` — TUN device (WireGuard tun, ip-link)
- `src/internal/transport/websocket/` — WS dial/accept, per-conn config
- `src/internal/transport/tls/` — TLS config (mTLS, CA, verify mode)
- `src/internal/transport/framing/` — binary frame protocol (encode/decode, buffer pool)
- `src/internal/protocol/handshake/` — Client/Server Hello, MTU negotiation
- `src/internal/protocol/auth/` — token/jwt/basic auth, session binding
- `src/internal/protocol/control/` — PING/PONG keepalive, session control
- `src/internal/routing/` — packet routing engine (RuleSet, CIDR/IP/domain matchers, ordered rules)
- `src/internal/dns/` — DNS resolver with in-memory TTL cache
- `src/internal/nat/` — nftables MASQUERADE (server-side NAT)
- `src/internal/session/` — session management, IP pool, expiry/reclaim, BoltDB persistence
- `src/internal/crypto/` — app-layer encryption (AES-256-GCM, per-session key derivation)
- `src/internal/metrics/` — Prometheus metrics (active_sessions, throughput, errors)
- `src/pkg/api/` — public API package (stub)
- `src/internal/proxy/` — SOCKS5 + HTTP CONNECT proxy listener for local proxy mode

## Key Paths
- `configs/client.yaml` — client config template
- `configs/client.test.yaml` — test config for split tunnel
- `configs/server.yaml` — server config template
- `configs/server.test.yaml` — test server config
- `Dockerfile` — multi-stage Docker build
- `Dockerfile.test` — gate test image (alpine + nftables)
- `docker-compose.yml` — local dev orchestration
- `docker-compose.test.yml` — gate test compose
- `scripts/build.sh` — native binary build to `bin/`
- `scripts/test-gate.sh` — gate test script
- `.github/workflows/ci.yml` — GitHub Actions CI pipeline
- `examples/docker-compose.yml` — standalone docker-compose example
- `examples/client.yaml` — standalone client config example
- `examples/server.yaml` — standalone server config example
- `examples/run.sh` — TLS gen + docker compose up script
- `README.md` — root readme with badges, quickstart, doc links
- `CHANGELOG.md` — version history (Keep a Changelog)
- `docs/en/` — English documentation
- `docs/ru/` — Russian documentation (full translation)

## Where To Edit
- Core tunnel logic — `src/internal/tun/`, `src/internal/transport/*`, `src/internal/protocol/*`
- Routing/rules — `src/internal/routing/`, `src/internal/nat/`, `src/internal/dns/`
- Session/auth — `src/internal/session/`, `src/internal/protocol/auth/`, `src/internal/logger/`
- Config changes — `src/internal/config/`
- Logging/metrics — `src/internal/logger/`, `src/internal/metrics/`
- Documentation — `docs/en/`, `docs/ru/`
- Examples — `examples/`
- Release — `CHANGELOG.md`, `README.md`

## Excluded
- `.speckeep/**` — excluded from indexing
- `specs/archived/**` — excluded from indexing
- `bin/` — build output
- `vendor/` — vendored deps (not used)
