# DNS Upstreams List — Data Model

## Status: changes

## Изменения

### `config.DNSProxyCfg`

```go
type DNSProxyCfg struct {
    Listen   string   `json:"listen" mapstructure:"listen"`
    Upstream string   `json:"upstream" mapstructure:"upstream"`   // ← deprecated, остаётся для backward compat
    Upstreams []string `json:"upstreams" mapstructure:"upstreams"` // ← новое поле
}
```

- Custom `UnmarshalYAML`/`UnmarshalJSON`: если указан `upstreams` — используем его (и warning если `upstream` тоже указан). Если только `upstream` — мигрируем в `Upstreams[0]`. Если ничего — дефолты.
- Сериализация: пишем только `upstreams`.

### `config.RelayDNSCfg`

```go
type RelayDNSCfg struct {
    Upstream    string `json:"upstream" mapstructure:"upstream"`     // ← deprecated
    Upstreams   []string `json:"upstreams" mapstructure:"upstreams"` // ← новое
    CacheTTL    int    `json:"cache_ttl" mapstructure:"cache_ttl"`
    Transparent bool   `json:"transparent" mapstructure:"transparent"`
}
```

- Аналогичная backward compat логика.

### `config.ServerConfig`

```go
type ServerConfig struct {
    // ... existing fields ...
    DNSUpstreams []string `mapstructure:"dns_upstreams"` // ← новое поле
}
```

### `dnsproxy.Server`

```go
type Server struct {
    listenAddr string
    upstreams  []string  // ← было upstream string
    // ... rest unchanged ...
}
```

### `tunnel.Session`

```go
type Session struct {
    // ... existing fields ...
    dnsUpstreams []string  // ← новое поле
}
```

### `config.DefaultDNSUpstreams`

Новая переменная:

```go
var DefaultDNSUpstreams = []string{"1.1.1.1:53", "8.8.8.8:53"}
```

## Без изменений

- `framing.FrameTypeDNS`, протокол фреймов — не меняется
- `routing`, `dns.Tracker`, `nat`, `transport` — не меняются
