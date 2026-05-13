# Security & ACL — Data Model

Status: changes (расширение YAML-конфига; BoltDB-схема не меняется)

## Изменения конфига (config/server.go)

### Новая секция ACL

```yaml
acl:
  deny_cidrs:
    - "10.0.0.0/8"
    - "172.16.0.0/12"
  allow_cidrs: []  # пусто = все разрешены (кроме deny)
```

Go-структура:
```go
type ACLCfg struct {
    DenyCIDRs  []string `mapstructure:"deny_cidrs"`
    AllowCIDRs []string `mapstructure:"allow_cidrs"`
}
```

### Новая структура токена (вместо []string)

```yaml
auth:
  mode: token
  tokens:
    - name: user1
      secret: tok1-hash
      bandwidth_bps: 102400    # 0 = unlimited
      max_sessions: 2          # 0 = unlimited
```

Go-структура:
```go
type TokenCfg struct {
    Name          string `mapstructure:"name"`
    Secret        string `mapstructure:"secret"`
    BandwidthBPS  int    `mapstructure:"bandwidth_bps"`
    MaxSessions   int    `mapstructure:"max_sessions"`
}
```

### Новая секция Origin

```yaml
origin:
  whitelist:
    - "https://example.com"
    - "https://*.example.com"
  allow_empty: false
```

Go-структура:
```go
type OriginCfg struct {
    Whitelist  []string `mapstructure:"whitelist"`
    AllowEmpty bool     `mapstructure:"allow_empty"`
}
```

### Новая секция Admin API

```yaml
admin:
  enabled: false
  listen: "localhost:8443"
  token: "admin-secret-token"
```

Go-структура:
```go
type AdminCfg struct {
    Enabled bool   `mapstructure:"enabled"`
    Listen  string `mapstructure:"listen"`
    Token   string `mapstructure:"token"`
}
```

### mTLS (расширение TLS)

```yaml
tls:
  client_ca_file: ""       # пусто = mTLS выключен
  client_auth: "require"   # "require" | "verify" | ""
```

Go-структура (новые поля):
```go
type TLSCfg struct {
    Cert         string `mapstructure:"cert"`
    Key          string `mapstructure:"key"`
    MinVersion   string `mapstructure:"min_version"`
    ClientCAFile string `mapstructure:"client_ca_file"`
    ClientAuth   string `mapstructure:"client_auth"`
}
```

## BoltDB

Схема не меняется. Сессии хранятся как `sessionID → IP`. max_sessions проверяется в памяти SessionManager, не в Bolt.

## Без изменений

- PoolCfg, SessionCfg, RateLimitCfg, LogConfig — без изменений
- Protocol frames (framing, handshake) — без изменений
- BoltDB buckets и ключи — без изменений
