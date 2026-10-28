# src/mobile

## Purpose
gomobile AAR boundary for Android v2.

## API
- `Resolve(configJSON) (sessionJSON, error)` — authenticated TUN assignment.
- `Start(configJSON, fd) (*Client, error)` — runs v2 data plane on a VpnService TUN fd.
- `Client.Stop()`, `Client.Error()`.

## Contract
- Kotlin owns VpnService address/routes/DNS; Go owns HTTP/2/QUIC and packet pump.
- `fd` is a detached Android TUN descriptor.
- Exclude-domain DNS/direct routing lives in `internal/client/direct.go`.
