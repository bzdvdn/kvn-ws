# Relay-Terminator: Data Model (as-built)

## Status

- wire protocols: **no-change** (frame format, handshake — без изменений)
- config: **extended** (RelayConfig)
- session/IP pool: **no-change** (переиспользованы из `internal/session`)

## RelayConfig (as-built)

```yaml
mode: relay                        # всегда "relay"
relay:
  mode: terminator                  # bridge | terminator
  listen: 0.0.0.0:8443
  ws_paths:
    - /tunnel
  max_connections: 200
  quic:
    keep_alive: 7
    idle_timeout: 60
  routing:
    direct_ranges:                  # CIDR для direct-маршрутизации
      - 10.0.0.0/8
      - 172.16.0.0/12
    direct_domains:                 # domain suffix (.local) или exact
      - .local
      - internal.corp
    dns:
      upstream: "1.1.1.1:53"       # DNS upstream для direct-доменов
      cache_ttl: 60                 # TTL кэша resolved IP (секунды)
      transparent: false            # перехватывать DNS на любом порту
  network:
    pool_ipv4:
      subnet: 172.16.0.0/24
      gateway: 172.16.0.1

upstream_token: your-upstream-token  # токен для upstream (или KVN_RELAY_AUTH_TOKEN env)
obfuscation:                         # обфускация для upstream tunnel
  enabled: true
  padding:
    enabled: true
    size: 512
crypto:
  key: relay-master-key              # для handshake с клиентами

auth:
  tokens:
    - name: default
      secret: your-token-here
      bandwidth_bps: 0
      max_sessions: 0

server: wss://upstream-vpn.example.com/tunnel
transport: quic                      # tcp | quic; upstream transport

tls:
  cert: /etc/kvn-ws/certs/relay.pem
  key: /etc/kvn-ws/certs/relay-key.pem
  verify_mode: insecure

log:
  level: info
```

## Структуры Go (as-built)

```go
type RelayConfig struct {
    Mode          string              // "relay"
    Relay         RelayTermCfg
    Server        string              // upstream server URL
    Transport     string              // "tcp" | "quic"
    Obfuscation   *ObfuscationCfg     // для upstream
    Crypto        CryptoCfg           // для handshake с клиентами
    TLS           ClientTLSCfg        // для accept + upstream
    Auth          ServerAuth          // токены клиентов
    UpstreamToken string              // токен для upstream
    Log           LogConfig
}

type RelayTermCfg struct {
    Mode           string             // "bridge" | "terminator"
    Listen         string
    WSPaths        []string
    MaxConnections int
    TLS            *RelayTLSCfg       // cert/key для accept
    Quic           *RelayQuicCfg
    Routing        *RelayRoutingCfg
    Network        *NetworkCfg        // IP pool для клиентов
}

type RelayDNSCfg struct {
    Upstream    string                // "1.1.1.1:53"
    CacheTTL    int                   // секунды
    Transparent bool
}

type RelayRoutingCfg struct {
    DirectRanges  []string
    DirectDomains []string
    DNS           *RelayDNSCfg
}
```
