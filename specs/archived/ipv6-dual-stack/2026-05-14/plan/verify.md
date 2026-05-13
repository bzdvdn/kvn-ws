---
report_type: verify
slug: ipv6-dual-stack
status: pass
docs_language: ru
generated_at: 2026-05-14
---

# Verify Report: ipv6-dual-stack

## Scope

- snapshot: верификация реализации IPv6 & Dual-Stack — pool, handshake, TUN, NAT, routing, DNS, kill-switch
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - specs/active/ipv6-dual-stack/tasks.md
- inspected_surfaces:
  - src/internal/config/server.go (PoolIPv6)
  - src/internal/session/{session,bolt}.go (IPv6 pool, AssignedIPv6)
  - src/internal/protocol/handshake/handshake.go (length-prefixed ServerHello)
  - src/internal/nat/nftables.go (Setup6/Teardown6)
  - src/internal/routing/router.go (parseDstIP6, dual-stack dispatch)
  - src/internal/dns/resolver.go (dual "ip" network)
  - src/cmd/server/main.go (pool6, TUN v6, NAT wiring)
  - src/cmd/client/main.go (IPv6 TUN, kill-switch IPv6)
  - test files for handshake, session, routing

## Verdict

- status: pass
- archive_readiness: safe
- summary: все 10 задач завершены, 5 AC покрыты кодом и тестами, traceability 29/29 маркеров, тесты проходят

## Checks

- task_state: completed=10, open=0
- acceptance_evidence:
  - AC-001 -> T1.2, T2.2, T4.1: `SetIP()` with v6 CIDR, client `ip -6 addr add`, server gateway v6
  - AC-002 -> T1.2, T4.1: `NewIPPool6` allocates unique fd00:: addresses, `TestIPv6PoolAllocate` ✅
  - AC-003 -> T2.3, T4.1: `NFTManager.Setup6/Teardown6` creates `ip6 kvn-nat` table
  - AC-004 -> T2.1, T2.2, T2.3, T4.2: handshake + pool + TUN + NAT fully wired
  - AC-005 -> T3.1, T3.2, T4.1: `parseDstIP6`, `"ip"` DNS, IPv6 kill-switch, routing dispatch
- implementation_alignment:
  - Handshake wire format: `SessionID(16) + Count(1) + [Family(1)+Len(1)+Addr]*Count`
  - Pool: `/112` subnet with random-offset allocation (no linear /64 scan)
  - NAT: separate `ip6 kvn-nat` table, not `inet` — no risk to IPv4 NAT
  - Routing: dispatch by packet version nibble (4=v4, 6=v6)
  - DNS: `net.DefaultResolver.LookupNetIP(ctx, "ip", domain)` for A+AAAA

## Errors

- none

## Warnings

- none

## Questions

- none

## Not Verified

- none (all items tested)

## Next Step

- safe to archive
