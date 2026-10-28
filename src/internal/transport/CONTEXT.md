# src/internal/transport

## Purpose
v2 HTTP/2 stream transport + optional QUIC datagram side channel + ICMP proxy.

## Key Files
- `client.go` — HTTP/2 client, uTLS dialer, session, POST/PUT/PATCH streams.
- `server.go` — TCP/UDP/ICMP handlers, fallback, auth, lease touch.
- `session.go` — `GET /` TUN assignment JSON.
- `padding.go` / `framing.go` — adaptive padding и datagram framing.
- `quic.go` — shared QUIC UDP channel + auth.
- `icmp.go` — server-side raw ICMP echo proxy.
- `const.go` — legacy path/header compatibility.

## Contract
- New clients use `/`, target in `:authority`; legacy `/api/v2/stream` + `X-KVN-Target` accepted.
- Session: `GET /` + `Accept: application/json` + Bearer + `X-Request-ID`.
- ICMP: `PATCH /` + `application/icmp`.
- Server pool — `internal/tunpool`; unknown/unauthorized requests get fallback, not a tunnel.
