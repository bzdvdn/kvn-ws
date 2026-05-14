<!-- @sk-task docs-and-release#T3.1: config reference (AC-002) -->

# Configuration Reference

kvn-ws uses YAML configuration files for both server and client.

## Server config (`server.yaml`)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `listen` | string | `:443` | Server listen address and port |
| `tls.cert` | string | `cert.pem` | Path to TLS certificate file |
| `tls.key` | string | `key.pem` | Path to TLS private key file |
| `tls.min_version` | string | `"1.3"` | Minimum TLS version |
| `network.pool_ipv4.subnet` | string | `10.10.0.0/24` | IPv4 pool subnet for client IP allocation |
| `network.pool_ipv4.gateway` | string | `10.10.0.1` | IPv4 gateway address |
| `network.pool_ipv6.subnet` | string | `fd00::/112` | IPv6 pool subnet for client IP allocation |
| `network.pool_ipv6.gateway` | string | `fd00::1` | IPv6 gateway address |
| `session.max_clients` | int | `100` | Maximum concurrent client sessions |
| `session.idle_timeout_sec` | int | `120` | Session idle timeout in seconds |
| `auth.mode` | string | `token` | Authentication mode (`token`, `jwt`, `basic`) |
| `auth.tokens[].name` | string | — | Token name for identification |
| `auth.tokens[].secret` | string | — | Token secret value |
| `auth.tokens[].bandwidth_bps` | int | `0` | Bandwidth limit in bps (0 = unlimited) |
| `auth.tokens[].max_sessions` | int | `0` | Max sessions per token (0 = unlimited) |
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
| `server` | string | — | WebSocket server URL (e.g. `https://example.com/ws`) |
| `auth.token` | string | — | Authentication token matching server config |
| `mtu` | int | `1400` | TUN interface MTU |
| `ipv6` | bool | `false` | Enable IPv6 support |
| `auto_reconnect` | bool | `true` | Automatically reconnect on disconnect |
| `log.level` | string | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `routing.default_route` | string | `server` | Default routing mode (`server`, `direct`) |
| `routing.include_ranges` | []string | `[]` | CIDR ranges to route through VPN |
| `routing.exclude_ranges` | []string | `[]` | CIDR ranges to exclude from VPN routing |
| `routing.include_ips` | []string | `[]` | Specific IPs to route through VPN |
| `routing.exclude_ips` | []string | `[]` | Specific IPs to exclude from VPN routing |
| `routing.include_domains` | []string | `[]` | Domains to route through VPN |
| `routing.exclude_domains` | []string | `[]` | Domains to exclude from VPN routing |

### Client example

```yaml
server: https://vpn.example.com/ws
auth:
  token: your-token-here
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

All config keys can be overridden by environment variables with the `KVN_` prefix and dots replaced by underscores:

```bash
KVN_LISTEN=:8443
KVN_AUTH_TOKEN=my-secret-token
KVN_LOG_LEVEL=debug
```
