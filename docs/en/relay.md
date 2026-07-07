# Relay Mode — Split-Tunnel Terminator

The relay operates as a **terminator**: a full VPN endpoint that accepts client connections, decrypts traffic, routes packets (direct vs upstream), and manages a TUN device. It acts as a split-tunnel gateway — direct CIDR/domain traffic goes through the relay's own network, the rest is forwarded to an upstream VPN server.

## Architecture

```
                    ┌──────────────┐      Direct CIDR
                    │              │◀──── ─ ─ ─ ─ ─ ─ ─    Internet
  WS client ───────▶│  Terminator  │      (via relay TUN)
                    │              │
  QUIC client ─────▶│  (mode:term) │      WebSocket/QUIC   ┌──────────────┐
                    │              ├────────────────────────▶    Server    │
                    │   TUN + NAT  │       (upstream)       │  (upstream)  │
                    └──────────────┘                        └──────────────┘
```

- **Direct CIDR** — traffic to matching IP ranges goes through the relay's TUN directly to the internet.
- **Upstream** — remaining traffic is encrypted and forwarded to the upstream VPN server via a separate tunnel.
- **NAT** — userspace SNAT/DNAT (`nat.go`): client packets to direct IPs get source-NAT'd to the relay's TUN IP; response packets are reverse-NAT'd back to the client.

## Transport Details

### Client → Relay

Clients connect via WebSocket (TCP, always on) or QUIC (UDP, requires `relay.quic` block). Both share the same TLS config and connection limit.

- **WebSocket listener**: HTTP server with TLS, path allowlist (`ws_paths`), standard WS upgrade.
- **QUIC listener**: UDP on the same port, requires TLS 1.3, no path filtering.

### Relay → Server (Upstream)

The relay opens a single upstream tunnel to the configured `server`. The upstream transport is independent of the client transport — set via `transport: quic` or `transport: tcp` (default). QUIC→TCP fallback on dial failure.

When `obfuscation.enabled: true`, the upstream QUIC connection is wrapped in `ObfuscatedQUICConn` (XOR via TLS keying material). The server must also have `obfuscation.enabled: true` for this to work.

## Configuration

Full terminator config:

```yaml
mode: relay
server: wss://vpn.example.com/tunnel
transport: quic               # upstream transport: tcp or quic
upstream_token: your-token    # upstream auth (or env KVN_RELAY_AUTH_TOKEN)

relay:
  mode: terminator
  listen: 0.0.0.0:8443
  ws_paths:
    - /tunnel
  max_connections: 200
  quic:
    keep_alive: 7
    idle_timeout: 60
  routing:
    direct_ranges:
      - 10.0.0.0/8
      - 192.168.0.0/16
    direct_domains:
      - .internal.example
      - .local
    dns:
      upstream: "1.1.1.1:53"
      cache_ttl: 60
      transparent: false
  network:
    pool_ipv4:
      subnet: 172.16.0.0/24
      gateway: 172.16.0.1

obfuscation:
  enabled: true
  padding:
    enabled: true
    size: 512
crypto:
  key: relay-master-key

auth:
  tokens:
    - name: default
      secret: your-token-here
tls:
  verify_mode: insecure
log:
  level: info
```

### DNS Interception

The terminator intercepts **all** DNS queries from clients (not just direct-domain ones):

1. Client sends a DNS query (UDP port 53) through the tunnel.
2. Terminator parses the domain name via `routing.ParseDNSQuestion`.
3. If the domain matches a direct rule → forwards to `dns.upstreams` (default `["1.1.1.1:53"]`), caches resolved IPs, returns response directly.
4. If the domain is NOT direct → resolved locally, response sent directly, **no caching** (so subsequent packets fall through to upstream routing).
5. Cached IPs bypass upstream routing for the cache TTL.

This ensures direct-domain traffic never touches the upstream tunnel.

### Routing Decision

For every outgoing packet, `routeOutgoing()` in `handler.go`:

1. **DNS check** — port 53 → resolve locally (see above).
2. **Cache check** — destination IP matches a cached direct-domain IP → direct.
3. **CIDR/domain RuleSet** — `ruleSet.Route(ip)` → direct or server.
4. **Direct** → SNAT + write to TUN.
5. **Server** → SNAT + `upstream.Send()`.

### Userspace NAT

All NAT is done in userspace (`nat.go`), avoiding iptables:

- **SNAT** in `routeOutgoing`: rewrites source IP from client's pool IP to the relay's TUN gateway IP, assigns a random port, stores the mapping.
- **DNAT** in `receiveLoop`: on incoming responses from TUN, looks up the original client IP:port and reverses the mapping.
- Supports TCP, UDP, and ICMP.

### Upstream Reconnect

If the upstream connection dies (`isClosed()` or `Send` error), the relay triggers an async reconnect via `reconnectUpstream()`. The reconnect is serialised by `upstreamMu` to prevent thundering herd. While reconnecting, direct traffic continues to work; upstream traffic is dropped with a warning.

### Lazy Connect

The relay does not crash if the upstream server is unavailable at startup. It logs a warning and retries when the first client arrives. Clients can connect and use direct routing immediately; upstream traffic waits for the tunnel to be established.

### IPv6 Support

The relay supports IPv6 if configured with `relay.network.pool_ipv6`. Both client and relay must support IPv6 for it to work:

```yaml
relay:
  network:
    pool_ipv4:
      subnet: 172.16.0.0/24
      gateway: 172.16.0.1
    pool_ipv6:
      subnet: "fd00::/112"
      gateway: "fd00::1"
```

Clients request IPv6 by setting `ipv6: true` in their config. If the relay has no IPv6 pool, the client runs IPv4-only.

**Note:** If a client does not need IPv6, set `ipv6: false` to prevent IPv6-first connection attempts (e.g. curl trying `2a00:1450:4010:c0f::65:80` before the IPv4 address) from failing with "network unreachable".

## Client Connection Examples

### WebSocket client (via relay)

```yaml
mode: tun
server: wss://relay:8443/tunnel
auth:
  token: your-token-here
tls:
  verify_mode: insecure
```

### QUIC client (via relay)

```yaml
mode: tun
server: quic://relay:8443
transport: quic
auth:
  token: your-token-here
tls:
  verify_mode: insecure
```

## Running the Example

```bash
cd examples/relay-terminator
bash run.sh
```

This starts:
1. **server** — upstream VPN server (port 443, QUIC-ready)
2. **relay** — terminator with WS + QUIC (port 8443 TCP+UDP)
3. **client** — WS client through relay
4. **quic-client** — QUIC client through relay

## Bridge Mode (Legacy)

A legacy **bridge** mode (`relay.mode: bridge`) exists as an opaque pipe — it forwards encrypted frames without decryption. It requires no TUN or NET_ADMIN. Bridge mode persists in `cmd/relay` for backward compatibility but is not actively developed. Use `terminator` for new deployments.

## Requirements

- `NET_ADMIN` capability and `/dev/net/tun` (TUN device).
- `relay.network.pool_ipv4` is required for terminator mode.
- For upstream auth, set `upstream_token` or `KVN_RELAY_AUTH_TOKEN` env.
- Server must have `obfuscation.enabled: true` if the relay uses QUIC upstream with obfuscation.
- `net.ipv4.ip_forward=1` is auto-enabled by the relay for response DNAT (kernel must forward TUN→public).
