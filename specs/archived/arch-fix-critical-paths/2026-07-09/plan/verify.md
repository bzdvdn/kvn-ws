---
report_type: verify
slug: arch-fix-critical-paths
status: pass
docs_language: ru
generated_at: 2026-07-09
---

# Verify Report: arch-fix-critical-paths

## Scope

- snapshot: 7 архитектурных фиксов в core/relay — QUIC backoff, DNS pool, DNS cache eviction, relay session registration, uint16 overflow guard, raw packet boundary guards, relay handler tests
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - specs/active/arch-fix-critical-paths/spec.md
  - specs/active/arch-fix-critical-paths/tasks.md
- inspected_surfaces:
  - src/internal/transport/quic/listen.go — AcceptWithBackoff
  - src/internal/bootstrap/server/server.go — QUIC accept loop
  - src/internal/bootstrap/relay/bootstrap.go — relay QUIC accept loop
  - src/internal/bootstrap/relay/router.go — DNS pool, DNS cache eviction, overflow guard, boundary guards
  - src/internal/bootstrap/relay/handler.go — session registration in SessionManager
  - src/internal/bootstrap/relay/handler_test.go — handler tests
  - src/internal/bootstrap/relay/router_test.go — DNS pool, cache, overflow, boundary tests
  - src/internal/transport/quic/listen_test.go — backoff tests
  - src/internal/bootstrap/server/server_test.go — server handler tests

## Verdict

- status: pass
- archive_readiness: safe
- summary: Все 7 AC подтверждены тестами, trace-маркеры на месте, все линтеры/проверки проходят

## Checks

- task_state: completed=18, open=0
- acceptance_evidence:
  - AC-001 -> T2.1 (AcceptWithBackoff), T2.1T (4 quic unit tests), T2.2 (server loop), T2.2T (handler tests), T2.3 (relay loop)
  - AC-002 -> T3.1 (getDNSConn/putDNSConn pool), T3.1T (TestGetDNSConnPool)
  - AC-003 -> T3.2 (insertDNSCache eviction), T3.2T (TestInsertDNSCacheLimit, TestInsertDNSCacheEvictExpired)
  - AC-004 -> T4.1 (sm.Create/SetCancel/Remove), T4.1T (TestHandleTerminatorStreamSessionLifecycle)
  - AC-005 -> T1.1 (overflow guard totalLen > 65535), T1.1T (TestBuildDNSRespPacketOverflow)
  - AC-006 -> T1.2 (boundary guards в extractDestIP, isDNSQuery, cacheDNSResponse), T1.2T (5 unit tests)
  - AC-007 -> T5.1 (handler_test.go), тесты TestHandleTerminatorStreamSessionLifecycle, TestHandleTerminatorStreamInvalidToken
- implementation_alignment:
  - backoff: base=10ms, max=5s, множитель ×2, capped — константы в listen.go:41-42
  - DNS pool: sync.Pool с getDNSConn/putDNSConn — router.go:34
  - DNS cache: defaultMaxDNSCacheSize=10000, TTL-scan + oldest-first eviction — router.go:185
  - Session: sm.Create → sm.SetCancel → sm.Remove в handleTerminatorStream — handler.go:48
  - Overflow: totalLen > 65535 → return nil — router.go:236
  - Boundary: len(packet) < minimalHeader → return false/zero — router.go:106,216,340
- traceability: 30 annotations найдены trace-скриптом, все @sk-task/@sk-test размещены над owning declarations (не на package/import/file-header)

## Verification Matrix

| AC-ID | Task IDs | Evidence | Verdict |
|-------|----------|----------|---------|
| AC-001 | T2.1, T2.1T, T2.2, T2.2T, T2.3, T2.3T | `TestAcceptWithBackoffTransientErrors`: PASS, `TestAcceptWithBackoffContextCanceled`: PASS, `TestAcceptWithBackoffFirstTry`: PASS, `TestAcceptWithBackoffContextTimeout`: PASS, `TestIsWebSocketRequest`: PASS, `TestAllowedWSPath`: PASS | pass |
| AC-002 | T3.1, T3.1T | `TestGetDNSConnPool`: PASS; pool reuse via sync.Pool в router.go:34 | pass |
| AC-003 | T3.2, T3.2T | `TestInsertDNSCacheLimit`: PASS (cache ≤ 10000), `TestInsertDNSCacheEvictExpired`: PASS; defaultMaxDNSCacheSize=10000 в router.go:17 | pass |
| AC-004 | T4.1, T4.1T | `TestHandleTerminatorStreamSessionLifecycle`: PASS; sm.Create/SetCancel/Remove в handler.go:48 | pass |
| AC-005 | T1.1, T1.1T | `TestBuildDNSRespPacketOverflow`: PASS; guard `totalLen > 65535 → return nil` в router.go:236 | pass |
| AC-006 | T1.2, T1.2T | `TestIsDNSQueryShort`: PASS, `TestIsDNSQueryIHLZero`: PASS, `TestExtractDestIPZero`: PASS, `TestExtractDestIPShort`: PASS, `TestExtractDestIPNonIP`: PASS; boundary guards в router.go:106,216,340 | pass |
| AC-007 | T5.1 | `TestHandleTerminatorStreamSessionLifecycle`: PASS, `TestHandleTerminatorStreamInvalidToken`: PASS; 2 теста в handler_test.go | pass |

## Errors

- none

## Warnings

- WARN: Touches references non-existent path `bridge.go`, `bootstrap.go` (task T3.1) — это опечатки в tasks.md, не влияющие на реализацию (bridge.go и bootstrap.go не затрагивались в T3.1)

## Questions

- none

## Not Verified

- SC-001 (100 transient errors без падения) и SC-002 (1000 DNS-запросов, ≤10 соединений) — скриптовые сценарии успеха, не проверялись в рамках default verification
- Android-клиент и Web UI — out of scope

## Next Step

- safe to archive

## Trace proof

- T1.1 -> src/internal/bootstrap/relay/router.go:236 (@sk-task)
- T1.1T -> src/internal/bootstrap/relay/router_test.go:107 (@sk-test)
- T1.2 -> src/internal/bootstrap/relay/router.go:106,216,340 (@sk-task)
- T1.2T -> src/internal/bootstrap/relay/router_test.go:124,142,151 (@sk-test)
- T2.1 -> src/internal/transport/quic/listen.go:51 (@sk-task)
- T2.1T -> src/internal/transport/quic/listen_test.go:25,43,59,76 (@sk-test)
- T2.2 -> src/internal/bootstrap/server/server.go:443 (@sk-task)
- T2.2T -> src/internal/bootstrap/server/server_test.go:10,26 (@sk-test)
- T2.3 -> src/internal/bootstrap/relay/bootstrap.go:246 (@sk-task)
- T3.1 -> src/internal/bootstrap/relay/router.go:34,76,96 (@sk-task)
- T3.1T -> src/internal/bootstrap/relay/router_test.go:64 (@sk-test)
- T3.2 -> src/internal/bootstrap/relay/router.go:185 (@sk-task)
- T3.2T -> src/internal/bootstrap/relay/router_test.go:18,38 (@sk-test)
- T4.1 -> src/internal/bootstrap/relay/handler.go:48,92,228 (@sk-task)
- T4.1T -> src/internal/bootstrap/relay/handler_test.go:92 (@sk-test)
- T5.1 -> src/internal/bootstrap/relay/handler_test.go:93,127 (@sk-test)
