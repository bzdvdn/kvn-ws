# src/internal/client

## Purpose
v2 client data plane (desktop TUN/proxy and mobile external TUN).

## Entry Points
- `New`/`Run` — proxy or self-owned TUN.
- `ResolveTUN` — fetch server-assigned lease.
- `RunTUNDevice` — external TUN (Android).

## Gotchas
- Server assigns unique TUN lease; no local TUN fallback.
- `X-Request-ID` (client_id) is sent on session and streams; streams renew leases.
- Direct/exclude DNS (`direct.go`) resolves outside v2 and direct-dials TCP/UDP.
- QUIC is optional; HTTP/2 UDP is fallback.
