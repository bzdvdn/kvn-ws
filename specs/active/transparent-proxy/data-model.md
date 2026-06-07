# Transparent Proxy — Data Model

status: change

## ClientConfig — новые поля

```go
type ClientConfig struct {
    // ... existing fields ...

    Transparent bool      `json:"transparent" mapstructure:"transparent"`
    DNSProxy     DNSProxyCfg `json:"dns_proxy" mapstructure:"dns_proxy"`
}

type DNSProxyCfg struct {
    Listen string `json:"listen" mapstructure:"listen"` // default: "127.0.0.53:53"
}
```

## Defaults

- `Transparent: false` — существующий `proxy mode` без изменений.
- `DNSProxyCfg.Listen: "127.0.0.53:53"` — стандартный адрес systemd-resolved.

## No-change areas

- Routing config (`RoutingCfg`) — exclude CIDR/домены используются и для transparent (iptables exclude rules) без изменения структуры.
- ProxyAuth, Crypto, TLS, Transport — без изменений.
- Protocol frames, API contracts — без изменений.
