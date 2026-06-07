# Repository Map — kvn-ws

Go stack: `go.mod` at root, all source under `src/`.

## Entry Points
- `src/cmd/client/main.go` — CLI entrypoint: KVN-over-WS client
- `src/cmd/server/main.go` — CLI entrypoint: KVN-over-WS server
- `src/cmd/gatetest/main.go` — routing gate test program (AC-010 simulation)
- `src/cmd/stability/main.go` — stability gate / soak test program (AC-012)
- `src/cmd/web/main.go` — Web UI entrypoint: browser-based tunnel client manager

## Top-Level Code
- `src/internal/config/` — YAML config parsing (viper + env override)
- `src/internal/logger/` — structured JSON logging (zap)
- `src/internal/tun/` — TUN device (WireGuard tun, ip-link)
- `src/internal/transparent/` — iptables/pf transparent proxy rules (DEC-001)
- `src/internal/transport/` — transport abstraction (StreamConn interface)
- `src/internal/transport/websocket/` — WS dial/accept, per-conn config
- `src/internal/transport/quic/` — QUIC dial/listen, ObfuscatedQUICConn (8B nonce + XOR)
- `src/internal/transport/tls/` — TLS config (mTLS, CA, verify mode)
- `src/internal/transport/framing/` — binary frame protocol (encode/decode, buffer pool)
- `src/internal/protocol/handshake/` — Client/Server Hello, MTU negotiation
- `src/internal/protocol/auth/` — token/jwt/basic auth, session binding
- `src/internal/protocol/control/` — PING/PONG keepalive, session control
- `src/internal/routing/` — packet routing engine (RuleSet, CIDR/IP/domain matchers, ordered rules)
- `src/internal/dns/` — DNS resolver with in-memory TTL cache
- `src/internal/dnsproxy/` — in-client DNS proxy server for transparent proxy mode
- `src/internal/nat/` — nftables MASQUERADE (server-side NAT)
- `src/internal/session/` — session management, IP pool, expiry/reclaim, BoltDB persistence
- `src/internal/crypto/` — app-layer encryption (AES-256-GCM, per-session key derivation)
- `src/internal/metrics/` — Prometheus metrics (active_sessions, throughput, errors)
- `src/internal/proxy/` — SOCKS5 + HTTP CONNECT proxy listener for local proxy mode
- `src/internal/acl/` — CIDR-based ACL (radix tree matcher, allow/deny lists)
- `src/internal/admin/` — Admin API server (chi router, session mgmt, pprof)
- `src/internal/bootstrap/client/` — Client bootstrap (dial, reconnect, kill switch, proxy, TUN)
- `src/internal/bootstrap/server/` — Server bootstrap (handler, lifecycle, graceful shutdown)
- `src/internal/ratelimit/` — IP-based rate limiter (token bucket per IP/session)
- `src/internal/systemproxy/` — Platform-specific system proxy manager (Linux/macOS/Windows env)
- `src/internal/tunnel/` — Tunnel session logic (proxy buffers, stream bridge, transport glue)
- `src/internal/webui/` — Web UI server (embed React SPA, REST API + SSE, AppState)
- `src/pkg/api/` — public API package (stub)
- `src/integration/` — integration tests (tunnel_integration_test.go)

## Key Paths
- `configs/client.yaml` — client config template
- `configs/client.test.yaml` — test config for split tunnel
- `configs/server.yaml` — server config template
- `configs/server.test.yaml` — test server config
- `configs/loadtest.yaml` — load test config
- `Dockerfile` — multi-stage Docker build
- `Dockerfile.test` — gate test image (alpine + nftables)
- `docker-compose.yml` — local dev orchestration
- `docker-compose.test.yml` — gate test compose
- `scripts/build.sh` — native binary build to `bin/`
- `scripts/build-web.sh` — web UI frontend build script
- `scripts/test-gate.sh` — gate test script
- `scripts/test-proxy.sh` — proxy mode test script
- `scripts/test-security.sh` — security/ACL test script
- `scripts/test-stability.sh` — stability/soak test script
- `scripts/install-server.sh` — server install script (systemd, TLS, config gen)
- `scripts/install-client.sh` — Linux client install script (binary + config + systemd)
- `scripts/install-client.ps1` — Windows client install script (binary + config + scheduled task)
- `scripts/install-web.sh` — Linux/macOS install script for kvn-web
- `scripts/install-web.ps1` — Windows install script for kvn-web
- `scripts/kvn-web.service` — systemd unit for kvn-web
- `scripts/kvn-web.plist` — macOS launchd plist for kvn-web
- `.github/workflows/ci.yml` — GitHub Actions CI pipeline
- `examples/docker-compose.yml` — standalone docker-compose example
- `examples/client.yaml` — standalone client config example
- `examples/server.yaml` — standalone server config example
- `examples/run.sh` — TLS gen + docker compose up script
- `README.md` — root readme with badges, quickstart, doc links
- `CHANGELOG.md` — version history (Keep a Changelog)
- `docs/en/` — English documentation
- `docs/ru/` — Russian documentation (full translation)
- `docs/openapi.yaml` — OpenAPI specification for admin API

## Where To Edit
- Core tunnel logic — `src/internal/tun/`, `src/internal/tunnel/`, `src/internal/transport/{websocket,quic,framing}/*`, `src/internal/protocol/*`
- Routing/rules — `src/internal/routing/`, `src/internal/nat/`, `src/internal/dns/`, `src/internal/acl/`
- Session/auth — `src/internal/session/`, `src/internal/protocol/auth/`, `src/internal/logger/`
- Client bootstrap — `src/internal/bootstrap/client/`
- Server bootstrap — `src/internal/bootstrap/server/`
- Admin API — `src/internal/admin/`
- Config changes — `src/internal/config/`
- Logging/metrics — `src/internal/logger/`, `src/internal/metrics/`
- Rate limiting — `src/internal/ratelimit/`
- System proxy — `src/internal/systemproxy/`
- Transparent proxy — `src/internal/transparent/`, `src/internal/dnsproxy/`
- Web UI — `src/internal/webui/`, `src/cmd/web/`, `src/internal/webui/frontend/`
- Integration tests — `src/integration/`
- Documentation — `docs/en/`, `docs/ru/`, `docs/openapi.yaml`
- Examples — `examples/`
- Build/CI/install — `scripts/`, `.github/workflows/`
- Load testing — `configs/loadtest.yaml`
- Release — `CHANGELOG.md`, `README.md`

## Excluded
- `.speckeep/**` — excluded from indexing
- `specs/archived/**` — excluded from indexing
- `specs/active/` — spec artifacts (read via spec files)
- `bin/` — build output
- `vendor/` — vendored deps (not used)
- `src/internal/webui/frontend/node_modules/` — JS deps
- `src/internal/webui/frontend/dist/` — frontend build output
