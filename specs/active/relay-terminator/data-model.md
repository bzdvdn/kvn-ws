# Relay-Terminator: Data Model

## Status

- wire protocols: **no-change** (frame format, handshake — без изменений)
- config: **extended** (новый `RelayConfig`)
- session/IP pool: **no-change** (переиспользуем существующие из `internal/session`)

## RelayConfig (новый)

```yaml
mode: relay
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
    direct_ranges:                    # CIDR
      - 10.0.0.0/8
      - 172.16.0.0/12
    direct_domains:                   # domain suffix (.ru, .local) или exact (internal.corp)
      - .ru
      - .local
      - internal.corp
  network:
    pool_ipv4:
      subnet: 10.10.0.0/24
      gateway: 10.10.0.1

auth:
  tokens:
    - name: default
      secret: your-token-here
      bandwidth_bps: 0
      max_sessions: 0

server: wss://upstream-vpn.example.com/tunnel
transport: quic                     # transport для upstream tunnel
obfuscation:                        # obfuscation для upstream tunnel
  enabled: true
  padding:
    enabled: true
    size: 512
crypto:
  key: relay-master-key             # для расшифровки фреймов клиента

tls:
  cert: /etc/kvn-ws/certs/relay.pem
  key: /etc/kvn-ws/certs/relay-key.pem
  verify_mode: insecure             # для upstream

log:
  level: info
```

## Структуры Go

```go
type RelayConfig struct {
    Mode        string              // "relay"
    Relay       RelayTermCfg        // relay-specific
    Server      string              // upstream server URL
    Transport   string              // upstream transport
    Obfuscation *ObfuscationCfg     // для upstream
    Crypto      CryptoCfg           // для расшифровки фреймов клиента
    TLS         ClientTLSCfg        // для upstream + для accept
    Auth        ServerAuth          // токены для аутентификации клиентов
    Log         LogConfig
}

type RelayTermCfg struct {
    Mode           string        // "bridge" | "terminator"
    Listen         string
    WSPaths        []string
    MaxConnections int
    TLS            *RelayTLSCfg  // cert/key для accept (отдельный от ClientTLSCfg)
    Quic           *RelayQuicCfg
    Routing        *RelayRoutingCfg
    Network        *NetworkCfg   // IP pool для клиентов
}

type RelayRoutingCfg struct {
    DirectRanges  []string  // CIDR list
    DirectDomains []string  // domain suffix (.ru) or exact (internal.corp)
}
```
