# src/internal/transport

## Purpose
v2 HTTP/2 stream transport + optional QUIC datagram side channel.

## Key Files
- `client.go` — shared HTTP/2 client, uTLS dialer, session request, POST/PUT streams.
- `server.go` — TCP/UDP stream handlers, fallback, lease touch.
- `session.go` — `GET /` TUN assignment JSON.
- `padding.go` — adaptive frame/datagram padding.
- `quic.go` — shared QUIC UDP channel.
- `const.go` — legacy path/header compatibility.

## Contract
- New clients use `/`, target in `:authority`; legacy `/api/v2/stream` + `X-KVN-Target` accepted.
- Session: `GET /` + `Accept: application/json` + Bearer + `X-Request-ID`.
- Server pool is `internal/tunpool`.
