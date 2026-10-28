# src/internal/client

## Purpose
v2 client data plane для desktop TUN/proxy и Android external TUN.

## Entry Points
- `New`/`Run` — proxy или self-owned TUN.
- `ResolveTUN` — получить server-assigned lease.
- `RunTUNDevice` — внешний TUN (Android).
- `Metrics` — RX/TX, latency, uptime, reconnects.

## Behavior
- TCP: gVisor split → отдельный HTTP/2 stream на flow.
- UDP: shared QUIC datagram channel, если `quic_enabled`; иначе HTTP/2 UDP stream.
- ICMP echo: `ping.go` → `PATCH /` stream, сервер делает raw ICMP.
- Direct: `direct.go` резолвит exclude-домены и dial-ит TCP/UDP напрямую.
- `client_id` передаётся в `X-Request-ID`; streams продлевают lease.

## Gotchas
- Server assigns unique TUN lease; no local TUN fallback.
- QUIC failure is non-fatal: fallback на HTTP/2 UDP.
- TUN fd на Android принадлежит Kotlin; Go не закрывает чужой fd.
- Realtime UDP зависит от QUIC; HTTP/2 fallback — TCP-in-TCP.
