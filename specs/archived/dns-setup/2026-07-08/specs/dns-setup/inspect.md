---
report_type: inspect
slug: dns-setup
status: pass
docs_language: ru
generated_at: 2026-07-08
---

# Inspect Report: dns-setup

## Scope

- snapshot: DNS registration for Windows + macOS TUN mode — SetDNS on interface, dnsproxy refactoring, bootstrap integration
- artifacts:
  - CONSTITUTION.md
  - specs/active/dns-setup/spec.md

## Verdict

- status: pass
- summary: spec coherent, no conflicts with constitution, 9 AC well-formed, open questions resolved

## Constitution Check

- ✅ No conflicts with constitution (Go 1.22+, traceability, DDD, docs ru/en)
- ✅ Traceability markers (`@sk-task`/`@sk-test`) required — noted for implementation phase
- ✅ Windows/macOS DNS registration does not introduce Cgo beyond existing constraints

## AC Quality Check

- AC-001 — AC-009: all have Given/When/Then with observable evidence
- AC-002: "ИЛИ восстановлен" — clean formulation for Wintun interface removal semantics
- AC-005: evidence specifies both Linux compilation AND cross-compile success
- AC-008/009: CleanupStaleDNS on Windows/macOS — platform-specific evidence is clear
- No placeholders or `[NEEDS CLARIFICATION]`

## Scope Check

- Single feature: DNS setup on Windows + macOS TUN. No scope creep.
- `Вне scope` correctly excludes SOCKS5/HTTP transparent proxy DNS, IPv6, DoT/DoH
- `Допущения` cover SIP, root requirement, full-tunnel DNS override deferred

## Open Questions Resolution

| Question | Resolution |
|----------|-----------|
| `SetDNSOverride(true)` — отдельная фича? | ✅ Оставлено post-MVP, записано в Допущениях |
| `DNSSettingsBackup` interface vs saveDNS/restoreDNS? | ✅ `saveDNS()`/`restoreDNS()` в bootstrap, без interface |
| macOS service name: hardwarePorts vs utunX? | ✅ `-listallhardwareports` primary → `utunX` fallback → оставлен открытый вопрос для macOS < Ventura |

## Edge Cases

- DNS routing disabled / no suffix domains → dnsproxy not started (consistent with Linux behavior)
- macOS DNS restoration with empty `-setdnsservers` → resets to DHCP (documented)
- Wintun adapter missing on CleanupStaleDNS → silent skip

## Errors

- none

## Warnings

- macOS fallback `-setdnsservers utunX <ip>` on macOS < Ventura is untested — may require cgo/SystemConfiguration
- `saveDNS()`/`restoreDNS()` platform files add 3 new build-tagged files in bootstrap — verify they don't conflict with existing Linux-only resolver exclude route logic

## Suggestions

- Consider extracting the DNS bootstrap block (tun.go:313-436) into a shared `setupDNS()` that dispatches to platform `saveDNS()`/`restoreDNS()`, keeping the common dnsproxy start/route logic in one place
- Add `@sk-task` markers on all new functions in implementation phase per constitution requirements

## Next Step

- safe to continue to plan

Готово к: /spk.plan dns-setup
