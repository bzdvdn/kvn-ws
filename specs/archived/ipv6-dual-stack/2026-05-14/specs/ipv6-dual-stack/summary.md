# IPv6 & Dual-Stack — Summary

**Goal:** Администратор VPN получает возможность развернуть туннель с IPv6-связностью (dual-stack или IPv6-only): клиенту назначается IPv6-адрес из пула fd00::/64, трафик IPv6 маршрутизируется через туннель с MASQUERADE на сервере.

## Acceptance Criteria

| AC | Description | Evidence |
|---|---|---|
| AC-001 | TUN интерфейс с IPv6 адресом | `ip -6 addr show dev kvn` показывает ULA адрес |
| AC-002 | IPv6 адрес назначается из пула | Два клиента получают разные fd00::/64 адреса |
| AC-003 | IPv6 MASQUERADE на сервере | `nft list table ip6 kvn-nat` показывает masquerade |
| AC-004 | ping6 проходит через туннель | `ping6 -c 1` — 0% loss |
| AC-005 | Dual-stack routing policy | IPv4 и IPv6 трафик маршрутизируются независимо |

## Out of Scope

- DNS64/NAT64
- IPv6-only транспорт (WebSocket через IPv6)
- ICMPv6 Path MTU discovery
- DHCPv6 / SLAAC
