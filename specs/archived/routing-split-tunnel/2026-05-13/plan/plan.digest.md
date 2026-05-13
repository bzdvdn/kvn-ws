DEC-001: exclude→include→default ordered pipeline (first match wins)
DEC-002: CIDR matching via net/netip.Prefix.Contains
DEC-003: DNS resolver — in-memory cache + TTL, stdlib net.DefaultResolver
DEC-004: DNS override — TUN-level UDP 53 interception
DEC-005: Server-side NAT — nftables exec wrapper (exec.Command)
Surfaces:
- config-routing: src/internal/config/client.go
- routing-engine: src/internal/routing/routing.go, src/internal/routing/matcher.go, src/internal/routing/rule.go
- dns-resolver: src/internal/dns/resolver.go, src/internal/dns/cache.go (new package)
- nat-manager: src/internal/nat/nat.go, src/internal/nat/nftables.go
- client-yaml: configs/client.yaml
