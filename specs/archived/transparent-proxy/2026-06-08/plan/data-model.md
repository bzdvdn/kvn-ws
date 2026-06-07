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
    Listen   string `json:"listen" mapstructure:"listen"`   // default: "127.0.0.54:53"
    Upstream string `json:"upstream" mapstructure:"upstream"` // default: "1.1.1.1:53"
}
```

## Defaults

- `Transparent: false` — существующий `proxy mode` без изменений.
- `DNSProxyCfg.Listen: "127.0.0.54:53"` — изменено с `127.0.0.53:53` (конфликт с systemd-resolved).
- `DNSProxyCfg.Upstream: "1.1.1.1:53"` — upstream для fallback (когда нет туннеля).

## DNS proxy — domain routing

DNS proxy не хранит routing-правила напрямую. Через `SetRouteFunc(fn func(domain string) bool)` передаётся функция, которая вызывает `routeSet.MatchDomain()` из существующего `RoutingCfg`. Excluded домены резолвятся локально через оригинальные nameserver-ы (из `ResolvConfBackup.Nameservers()`), остальные — через туннель как раньше.

## No-change areas

- Routing config (`RoutingCfg`) — exclude CIDR/домены используются и для transparent (iptables exclude rules) без изменения структуры.
- ProxyAuth, Crypto, TLS, Transport — без изменений.
- Protocol frames, API contracts — без изменений.
