# Data Model: IPv6 & Dual-Stack

## Изменения модели

### PoolCfg (server config)
```go
type PoolCfg struct {
    Subnet     string  // unchanged
    Gateway    string  // unchanged
    RangeStart string  // unchanged
    RangeEnd   string  // unchanged
}
```
Новое поле `PoolIPv6 PoolCfg` в `NetworkCfg` — переиспользует существующую структуру. IPv6-пул не использует RangeStart/RangeEnd (весь /64 доступен), но поля остаются для консистентности.

### IPPool — два пула
`SessionManager` содержит два `*IPPool` вместо одного. IPv6-пул использует `/112` вместо `/64` для практичной аллокации (65536 подсетей /112 в /64, каждая /112 = 65534 адреса без gateway).

### Session
```go
type Session struct {
    ID           string
    TokenName    string
    AssignedIP   net.IP    // IPv4 (unchanged)
    AssignedIPv6 net.IP    // новая: IPv6
    RemoteAddr   string
    ConnectedAt  time.Time
    LastActivity time.Time
}
```

### ServerHello
Расширение формата: `SessionID(16) + FamilyByte(1) + IPLen(1) + IPBytes(n)` — сначала IPv4, потом IPv6 (если назначен). FamilyByte = 4 или 6.

### BoltDB
Ключ `session:{id}:ipv6` для хранения IPv6 аллокации. IPv4 запись (`session:{id}`) остаётся без изменений.

## Неизменяемые части
- `Matcher` / `CIDRMatcher` / `ExactIPMatcher` / `DomainMatcher` — работают с `netip.Addr`, IPv6-ready
- `RuleSet.Route()` — уже принимает `netip.Addr`
- `BoltStore` — `net.IP` сериализуется и для 4, и для 16 байт
- `ClientConfig.IPv6 bool` — поле уже существует (dead), используется по назначению
