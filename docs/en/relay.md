# Relay Mode

Relay mode allows running an intermediary node that accepts client connections and proxies them to an upstream VPN server. The relay acts as a transparent pipe — it does not decrypt, obfuscate, or inspect tunnel traffic.

## Architecture

```
                    ┌──────────────┐      WebSocket       ┌──────────────┐
  WS client ───────▶│              ├─────────────────────▶│              │
                    │    Relay     │                       │    Server    │
  QUIC client ─────▶│  (mode:relay)│      WebSocket       │  (upstream)  │
                    └──────────────┘                       └──────────────┘
```

- **Relay → Server**: always WebSocket (single upstream connection per client)
- **Client → Relay**: WebSocket (TCP, always on) or QUIC (UDP, optional)
- Both transports share the same connection limit (`max_connections`) and bridge logic

## Transport Details

### WebSocket listener
- HTTP server with TLS, always active
- Path allowlist (`ws_paths`) — requests to non-allowed paths return 404
- Standard WebSocket upgrade (`Upgrade: websocket` header required)

### QUIC listener
- UDP listener on the same port as WS (different L4 protocol)
- Active only when `relay.quic` block is configured
- No path filtering (QUIC has no HTTP concept)
- Same TLS config as WebSocket (TLS 1.3 required)

## Configuration

Minimal relay config:

```yaml
mode: relay
server: wss://vpn.example.com/tunnel
relay:
  listen: 0.0.0.0:8443
tls:
  verify_mode: insecure
```

With QUIC and custom paths:

```yaml
mode: relay
server: wss://vpn.example.com/tunnel
relay:
  listen: 0.0.0.0:8443
  ws_paths:
    - /tunnel
    - /api/v1/events
  max_connections: 200
  quic:
    keep_alive: 7
    idle_timeout: 60
tls:
  verify_mode: insecure
log:
  level: info
```

## Client connection examples

### WebSocket client (via relay)

```yaml
mode: tun
server: wss://relay:8443/tunnel
auth:
  token: your-token
tls:
  verify_mode: insecure
```

### QUIC client (via relay)

```yaml
mode: tun
server: quic://relay:8443
transport: quic
auth:
  token: your-token
tls:
  verify_mode: insecure
```

## Running the example

```bash
cd examples/relay
bash run.sh
```

This starts:
1. **server** — upstream VPN server (port 443)
2. **relay** — relay node with WS + QUIC (port 8443 TCP+UDP)
3. **client** — WS client connecting through relay
4. **quic-client** — QUIC client connecting through relay

Both clients establish a tunnel through the relay to the upstream server.

## Terminator Mode

Terminator is a relay mode where the node acts as a full VPN endpoint: accepts clients, allocates IPs from a pool, sets up TUN, and routes traffic. Unlike bridge mode (transparent pipe), the terminator **decrypts** and **routes** client traffic.

### Architecture

```
                    ┌──────────────┐      Direct CIDR     ┌──────────────┐
                    │              │◀──── ─ ─ ─ ─ ─ ─ ─ ─▶│   Internet   │
  WS client ───────▶│  Terminator  │                       │  (via TUN)   │
                    │ (mode:term)  │      WebSocket       ┌──────────────┐
  QUIC client ─────▶│              ├─────────────────────▶│    Server    │
                    └──────────────┘      (upstream)      │  (upstream)  │
                                                           └──────────────┘
```

- **Direct CIDR** — traffic to specified ranges goes directly through the relay's TUN.
- **Upstream** — remaining traffic is encrypted and sent to the upstream VPN server.
- Relay allocates IPs to clients from its own pool (`relay.network.pool_ipv4`).

### Terminator Configuration

```yaml
mode: relay
server: wss://vpn.example.com/tunnel
relay:
  mode: terminator
  listen: 0.0.0.0:8443
  routing:
    direct_ranges:
      - 10.0.0.0/8
      - 192.168.0.0/16
    direct_domains:
      - .internal.example
  network:
    pool_ipv4:
      subnet: 172.16.0.0/24
      gateway: 172.16.0.1
tls:
  verify_mode: insecure
```

### Requirements

- Relay requires `NET_ADMIN` capability and `/dev/net/tun` (TUN device).
- `relay.network.pool_ipv4` is required for terminator mode.
- For the upstream connection, the relay uses the `KVN_RELAY_AUTH_TOKEN` environment variable.

## Notes

- Relay does not require TUN device or root privileges (no `NET_ADMIN` needed for the relay itself)
- TLS certificate for incoming connections: if not configured, a self-signed cert is generated
- The relay always connects to upstream via WebSocket regardless of client transport
- `idle_timeout` for QUIC is required (must be > 0) to prevent resource leaks
- `keep_alive` defaults to 7 seconds if not set or set to 0
- WS clients connecting through a relay **must disable** `obfuscation.padding`: the relay is an opaque pipe and will forward padded messages upstream, corrupting frame decoding on the server side. QUIC clients have no such restriction — they never use WS padding.
