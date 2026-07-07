---
report_type: verify
slug: android-fakedns-routing
status: pass
docs_language: ru
generated_at: 2026-07-08
---

# Verify Report: android-fakedns-routing

## Scope

- snapshot: fakeDNS domain-based routing (exclude/include suffix) with TCP direct delivery
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - `.speckeep/constitution.summary.md`
  - specs/active/android-fakedns-routing/tasks.md
  - specs/active/android-fakedns-routing/spec.md
- inspected_surfaces:
  - config/AppConfig.kt — routingDomainsEnabled, routingExcludeDomains, routingIncludeDomains
  - dns/FakeIpPool.kt — bitmap allocator 198.18.0.0/15
  - dns/DnsParser.kt — buildQuery, buildResponse, buildEmptyResponse
  - dns/FakeDnsResolver.kt — resolve(), resolveDomain(), suffix matching, isExcluded
  - vpn/KvnVpnService.kt — routePacket(), DirectDeliverer (handleTcpSyn, handleTcpData, handleUdp), buildTcpResponse, buildDnsResponsePacket, rewritePacket, closeTun cleanup
  - dns/LogBuffer.kt — in-memory ring buffer
  - FakeDnsResolverTest.kt — 13 tests
  - FakeIpPoolTest.kt — 11 tests
  - KvnVpnServiceTest.kt — 9 tests

## Verdict

- status: pass
- archive_readiness: safe
- summary: Все 8 задач выполнены, 93 unit-теста проходят, ручное тестирование `curl 2ip.ru` подтверждает AC-001. 7 багфиксов задокументированы в T4.2.

## Checks

- task_state: completed=8, open=0; все задачи T1.1–T4.2 завершены
- acceptance_evidence:
  - AC-001 → T2.1 (FakeDnsResolver.resolvedDomain + isExcluded), T2.2 (FakeDnsResolverTest суффиксный match), T4.2 (manual: `curl 2ip.ru` показывает реальный IP)
  - AC-002 → T3.1 (include matching + fake IP allocation), T4.1 (FakeDnsResolverTest include возвращает fake IP)
  - AC-003 → T2.1 (routing engine проверяет excludedIp set), T2.2 (KvnVpnServiceTest SYN fallback), T3.2 (cleanup не затрагивает CIDR)
  - AC-004 → T2.1 (dot-barrier в `dotBarrier()`), T2.2 (FakeDnsResolverTest dot-barrier cases)
  - AC-005 → T3.2 (closeTun очищает pool/cache/resolver), T4.1 (FakeIpPoolTest clear)
  - AC-006 → T3.1 (rewritePacket — IP + TCP checksum), T4.1 (KvnVpnServiceTest rewrite TCP/UDP)
  - AC-007 → T2.1 (excludedIp set проверяется до domain), T3.2 (edge cases), T2.2 (прямой SYN)
  - AC-008 → T3.1 (non-matched → fake IP pool), T4.1 (FakeDnsResolverTest include no pool = null)
  - AC-009 → T2.1 (routingDomainsEnabled=false → null), T2.2 (resolver test false = null), T4.2 (manual)
- implementation_alignment:
  - Suffix matching: `dotBarrier()` — endsWith + dot before suffix (T2.1)
  - DNS resolution: `resolveDomain()` использует `bindSocket` на физической сети (DEC-006)
  - TCP direct delivery: `handleTcpSyn` — 2s timeout, `handleTcpData` — payload relay, reader job — MTU-40 chunking
  - Oversized packet fix: reader job режет ответ на chunk-и по `mtu-40` байт
  - Data offset fix: `/4` → `ushr 4` для правильного вычисления TCP header length
  - TCP checksum: pseudo-header 4 отдельных слова + `end=20+tcpLen`

## Errors

- none

## Warnings

- `verify-task-state.sh` сообщает WARN о несуществующих путях в Touches (config/AppConfig.kt, dns/*.kt) — ложно-положительные: пути относительны к android source tree, а не к specs/
- WARN о задачах без Touches: поле — поле опционально, все ключевые Touches указаны

## Questions

- none

## Not Verified

- Include domain (`routingIncludeDomains`) не тестировался вручную на реальном устройстве (нет `.corp` инфраструктуры). Unit-тесты (FakeDnsResolverTest, KvnVpnServiceTest) покрывают функциональность.
- AAAA → пустой ответ проверен unit-тестами, не проверен вручную (нет IPv6-only DNS).
- Edge case «exhaustion fake IP pool» покрыт unit-тестом, не проверен вручную.

## Next Step

- safe to archive
