DEC-001: раздельные IPv4/IPv6 пулы, аллокация без линейного сканирования /64
DEC-002: handshake с length-prefixed IP (4 или 16 байт)
DEC-003: IPv6 NAT через отдельную nftables table `ip6 kvn-nat`
DEC-004: определение семейства пакета по первой ниббле (4/6)
DEC-005: DNS-резолвер с dual-stack (A + AAAA) запросами
Surfaces:
- tun: src/internal/tun/tun.go
- pool: src/internal/session/session.go, session/bolt.go
- handshake: src/internal/protocol/handshake/handshake.go
- nat: src/internal/nat/nftables.go
- router: src/internal/routing/router.go
- dns: src/internal/dns/resolver.go
- config-server: src/internal/config/server.go
- config-client: src/internal/config/client.go
- cmd-server: src/cmd/server/main.go
- cmd-client: src/cmd/client/main.go
- killswitch: src/cmd/client/main.go
