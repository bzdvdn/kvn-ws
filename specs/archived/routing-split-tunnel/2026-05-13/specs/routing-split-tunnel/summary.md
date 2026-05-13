# Routing & Split Tunnel — Summary

**Goal:** Администратор управляет маршрутизацией клиента: server/direct default, split tunnel по CIDR/IP/доменам, ordered rules engine, DNS resolver с кешем, серверный NAT и DNS override в full-tunnel режиме.

| AC | Описание | Приоритет |
|----|----------|-----------|
| AC-001 | Default route mode — server / direct | P1 |
| AC-002 | Split tunnel по CIDR (include_ranges / exclude_ranges) | P1 |
| AC-003 | Routing по отдельным IP (include_ips / exclude_ips) | P1 |
| AC-004 | DNS resolver с кешем и TTL | P1 |
| AC-005 | Routing по доменам (include_domains / exclude_domains) | P1 |
| AC-006 | Ordered rules engine (exclude → include → default) | P1 |
| AC-007 | Server-side NAT (nftables MASQUERADE) | P1 |
| AC-008 | DNS override для full-tunnel | P2 |
| AC-009 | Конфигурация маршрутизации в client.yaml | P1 |
| AC-010 | Gate: YouTube напрямую, корп. ресурсы через туннель | P1 |

**Out of Scope:** IPv6, балансировка, policy-based routing, PAC, Windows NAT.

**MVP:** default_route + CIDR split tunnel + ordered rules engine (AC-001, AC-002, AC-006, AC-009).
