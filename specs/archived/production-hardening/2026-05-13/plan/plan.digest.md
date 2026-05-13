DEC-001: reconnect — exponential backoff loop around websocket.Dial
DEC-002: keepalive — gorilla/websocket native PING/PONG + timeout
DEC-003: kill-switch — nftables reject rule on disconnect, remove on reconnect
DEC-004: rate limiter — token bucket per IP, middleware in HTTP handler
DEC-005: session expiry — background goroutine + idle/TTL checks
DEC-006: BoltDB persistence — bolt store for IPPool allocations
DEC-007: Prometheus metrics — promhttp.Handler + custom collectors
DEC-008: health endpoint — separate /health handler on server mux
DEC-009: SIGHUP reload — signal.Notify + atomic config swap
DEC-010: structured audit — zap.With zap.Fields for session/ip/reason
DEC-011: CLI/env — pflag + viper precedence: CLI > env > YAML
Surfaces:
- control-ping: src/internal/protocol/control/control.go
- ws-transport: src/internal/transport/websocket/websocket.go
- client-main: src/cmd/client/main.go
- server-main: src/cmd/server/main.go
- session-store: src/internal/session/session.go, src/internal/session/store.go
- session-bolt: src/internal/session/bolt.go (new file)
- metrics: src/internal/metrics/metrics.go
- config-cli: src/internal/config/server.go, src/internal/config/client.go
- config-reload: src/internal/config/config.go
- logger: src/internal/logger/logger.go
