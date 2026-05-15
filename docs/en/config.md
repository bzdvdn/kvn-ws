<!-- @sk-task docs-and-release#T3.1: config reference (AC-002) -->

# Configuration Reference

kvn-ws uses YAML configuration files for both server and client.

## Server config (`server.yaml`)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `listen` | string | `:443` | Server listen address and port |
| `tls.cert` | string | `cert.pem` | Path to TLS certificate file |
| `tls.key` | string | `key.pem` | Path to TLS private key file |
| `tls.client_ca_file` | string | `""` | Path to CA cert for mTLS client verification |
| `tls.client_auth` | string | `""` | mTLS mode: `request`, `require`, `verify` (requires `client_ca_file`) |
| `network.pool_ipv4.subnet` | string | `10.10.0.0/24` | IPv4 pool subnet for client IP allocation |
| `network.pool_ipv4.gateway` | string | `10.10.0.1` | IPv4 gateway address |
| `network.pool_ipv4.range_start` | string | subnet+1 | First allocatable IPv4 address |
| `network.pool_ipv4.range_end` | string | broadcast-1 | Last allocatable IPv4 address |
| `network.pool_ipv6.subnet` | string | `fd00::/112` | IPv6 pool subnet for client IP allocation |
| `network.pool_ipv6.gateway` | string | `fd00::1` | IPv6 gateway address |
| `network.pool_ipv6.range_start` | string | subnet+1 | First allocatable IPv6 address |
| `network.pool_ipv6.range_end` | string | broadcast-1 | Last allocatable IPv6 address |
| `session.max_clients` | int | `100` | Maximum concurrent client sessions |
| `session.idle_timeout_sec` | int | `120` | Session idle timeout in seconds (0 = no timeout) |
| `session.expiry.session_ttl_sec` | int | `86400` | Absolute session TTL in seconds |
| `session.expiry.reclaim_interval_sec` | int | `10` | Reclaim loop interval in seconds |
| `auth.mode` | string | `token` | Authentication mode (`token`, `jwt`, `basic`) |
| `auth.tokens[].name` | string | — | Token name for identification |
| `auth.tokens[].secret` | string | — | Token secret value |
| `auth.tokens[].bandwidth_bps` | int | `0` | Bandwidth limit in bps (0 = unlimited) |
| `auth.tokens[].max_sessions` | int | `0` | Max sessions per token (0 = unlimited) |
| `rate_limiting.auth_burst` | int | `5` | Auth rate-limiter burst size per IP |
| `rate_limiting.auth_per_minute` | int | `1` | Auth rate-limiter requests per minute per IP |
| `rate_limiting.packets_per_sec` | int | `1000` | Per-session packet rate limit |
| `origin.whitelist` | []string | `[]` | Allowed Origin/Referer headers (empty = allow all) |
| `origin.allow_empty` | bool | `true` | Allow requests without Origin header |
| `admin.enabled` | bool | `false` | Enable Admin API |
| `admin.listen` | string | `localhost:8443` | Admin API listen address |
| `admin.token` | string | `""` | Admin API authentication token |
| `acl.deny_cidrs` | []string | `[]` | CIDR deny list (denied before allow check) |
| `acl.allow_cidrs` | []string | `[]` | CIDR allow list (empty = allow all) |
| `compression` | bool | `false` | Enable WebSocket per-message compression |
| `multiplex` | bool | `false` | Enable WebSocket multiplexing |
| `mtu` | int | `1400` | TUN interface MTU |
| `crypto.enabled` | bool | `false` | Enable app-layer AES-256-GCM encryption |
| `crypto.key` | string | `""` | 256-bit master key as 64 hex chars (required if enabled) |
| `bolt_db_path` | string | `""` | Path to BoltDB file for IP pool persistence (empty = in-memory only) |
| `logging.level` | string | `info` | Log level (`debug`, `info`, `warn`, `error`) |

### Server example

```yaml
listen: :443
tls:
  cert: /etc/kvn-ws/cert.pem
  key: /etc/kvn-ws/key.pem
  min_version: "1.3"
network:
  pool_ipv4:
    subnet: 10.10.0.0/24
    gateway: 10.10.0.1
session:
  max_clients: 100
  idle_timeout_sec: 120
auth:
  mode: token
  tokens:
    - name: default
      secret: your-token-here
logging:
  level: info
```

## Client config (`client.yaml`)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `server` | string | — | WebSocket server URL (e.g. `wss://example.com/tunnel`) |
| `mode` | string | `tun` | Client mode: `tun` (VPN tunnel) or `proxy` (local SOCKS5/HTTP proxy) |
| `auth.token` | string | — | Authentication token matching server config |
| `tls.verify_mode` | string | `verify` | TLS verification mode: `verify`, `skip` |
| `tls.ca_file` | string | `""` | Custom CA certificate file (optional) |
| `tls.server_name` | string | `""` | TLS SNI server name (optional) |
| `mtu` | int | `1400` | TUN interface MTU |
| `ipv6` | bool | `false` | Enable IPv6 support |
| `auto_reconnect` | bool | `true` | Automatically reconnect on disconnect |
| `compression` | bool | `false` | Enable WebSocket per-message compression |
| `multiplex` | bool | `false` | Enable WebSocket multiplexing |
| `crypto.enabled` | bool | `false` | Enable app-layer AES-256-GCM encryption |
| `crypto.key` | string | `""` | 256-bit master key as 64 hex chars (must match server) |
| `proxy_listen` | string | `127.0.0.1:2310` | SOCKS5/HTTP proxy listen address (proxy mode only) |
| `proxy_auth.username` | string | `""` | Proxy authentication username (optional) |
| `proxy_auth.password` | string | `""` | Proxy authentication password (optional) |
| `routing.default_route` | string | `server` | Default routing mode (`server`, `direct`) |
| `routing.include_ranges` | []string | `[]` | CIDR ranges to route through VPN |
| `routing.exclude_ranges` | []string | `[]` | CIDR ranges to exclude from VPN routing |
| `routing.include_ips` | []string | `[]` | Specific IPs to route through VPN |
| `routing.exclude_ips` | []string | `[]` | Specific IPs to exclude from VPN routing |
| `routing.include_domains` | []string | `[]` | Domains to route through VPN |
| `routing.exclude_domains` | []string | `[]` | Domains to exclude from VPN routing |
| `kill_switch.enabled` | bool | `false` | Block all non-tunnel traffic on disconnect (nftables) |
| `reconnect.min_backoff_sec` | int | `1` | Minimum reconnect backoff in seconds |
| `reconnect.max_backoff_sec` | int | `30` | Maximum reconnect backoff in seconds |
| `log.level` | string | `info` | Log level (`debug`, `info`, `warn`, `error`) |

### Client example

```yaml
server: wss://vpn.example.com/tunnel
auth:
  token: your-token-here
tls:
  verify_mode: verify
mtu: 1400
ipv6: false
auto_reconnect: true
log:
  level: info
routing:
  default_route: server
  include_ranges:
    - 10.0.0.0/8
  exclude_domains:
    - example.com
```

## Environment variables

All config keys can be overridden by environment variables with the `KVN_SERVER_` / `KVN_CLIENT_` prefix and dots replaced by underscores:

```bash
KVN_SERVER_LISTEN=:8443
KVN_SERVER_CRYPTO_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
KVN_CLIENT_AUTH_TOKEN=my-secret-token
KVN_CLIENT_LOG_LEVEL=debug
```

For auth tokens in production, use a JSON array via environment variable:

```bash
KVN_SERVER_AUTH_TOKENS_JSON='[{"name":"admin","secret":"a1b2c3"},{"name":"guest","secret":"x9y8z7"}]'
```
