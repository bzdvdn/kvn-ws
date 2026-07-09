---
report_type: verify
slug: win-tun
status: pass
docs_language: ru
generated_at: 2026-07-08
---

# Verify Report: win-tun

## Scope

- snapshot: Windows TUN Device (Wintun + winipcfg + web UI integration) — все 11 AC, 12 задач, 6 фаз
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - specs/active/win-tun/spec.md
  - specs/active/win-tun/plan.md
  - specs/active/win-tun/tasks.md
- inspected_surfaces:
  - src/internal/tun/tun_windows.go (Open, Close, Read, Write, SetIP, SetMTU, SetGateway, RemoveGateway, AddExcludeRoute, RemoveExcludeRoute, CleanupExcludeRoutes, CleanupStaleExcludeRoutes, SaveDefaultRoute, deterministicGUID)
  - src/internal/tun/tun_stub.go (build tag)
  - src/internal/tun/tun_windows_test.go (6 unit tests)
  - src/internal/tun/tun_common.go (routeMeta, TunDevice interface)
  - src/internal/webui/server.go (handlePlatform — tun_supported)
  - src/internal/webui/handler_connect.go (TUN mode unblock)
  - src/internal/webui/frontend/src/types.ts (PlatformResponse)
  - src/internal/webui/frontend/src/context.tsx (tunSupported state)
  - src/internal/webui/frontend/src/App.tsx (prop wiring)
  - src/internal/webui/frontend/src/TabbedForm.tsx (conditional TUN option)
  - scripts/build.sh (windows cross-compile target)

## Verdict

- status: pass
- archive_readiness: safe
- summary: все 12 задач завершены, 11 AC покрыты кодом и/или тестами, cross-compile проходит, traceability полная

## Verification Matrix

| AC-ID | Task IDs | Evidence | Verdict |
|-------|----------|----------|---------|
| AC-001 | T1.1, T2.1 | `tun_windows.go:57` Open() создаёт Wintun adapter; `tun_windows.go:114` SetIP() через winipcfg LUID.SetIPAddressesForFamily | pass |
| AC-002 | T1.1 | `tun_windows.go:86` Read(), `tun_windows.go:96` Write() без virtio headroom | pass |
| AC-003 | T3.1 | `tun_windows.go:146` SetGateway(), `tun_windows.go:163` RemoveGateway() через LUID.AddRoute/DeleteRoute | pass |
| AC-004 | T3.2, T6.1 | `tun_windows.go:179` AddExcludeRoute, `:205` RemoveExcludeRoute, `:225` CleanupExcludeRoutes, `:280` CleanupStaleExcludeRoutes + тесты `TestParseLUIDRoundtrip`, `TestParseLUIDInvalid` | pass |
| AC-005 | T2.1 | `tun_windows.go:132` SetMTU() через LUID.IPInterface().NLMTU | pass |
| AC-006 | T4.1, T6.1 | `tun_windows.go:35` deterministicGUID() UUIDv5 + 4 теста: `TestDeterministicGUIDStability`, `TestDeterministicGUIDDifferentInputs`, `TestDeterministicGUIDVersionBits`, `TestDeterministicGUIDRandom` | pass |
| AC-007 | T4.2 | `tun_windows.go:74` Close() guarded by sync.Once, idempotent | pass |
| AC-008 | T1.2 | `scripts/build.sh:45` case "windows" с GOOS=windows GOARCH=amd64; `GOOS=windows go build ./src/internal/tun/...` — pass | pass |
| AC-009 | T1.1 | `tun_stub.go:1` build tag `!linux && !windows` | pass |
| AC-010 | T3.1 | `tun_windows.go:257` SaveDefaultRoute() через winipcfg.GetIPForwardTable2, выбор best default | pass |
| AC-011 | T5.1, T5.2 | `server.go:111` tun_supported в /api/platform; `handler_connect.go:84` разблокирован Windows; frontend: `types.ts:70` PlatformResponse, `context.tsx:59` tunSupported state, `App.tsx:65` prop wiring, `TabbedForm.tsx:247` conditional option | pass |

## Checks

- task_state: completed=12, open=0
- acceptance_evidence: 11/11 AC — подтверждены кодом, тестами и/или билд-проверками
- implementation_alignment:
  - Все функции TunDevice реализованы в tun_windows.go с корректными build tags
  - Web UI динамически показывает tun_supported на всех платформах
  - Cross-compile GOOS=windows GOARCH=amd64 проходит для tun, web, client/server пакетов
  - GUID детерминирован (UUIDv5, SHA-1, DNS namespace)
- traceability: 28 `@sk-task` / `@sk-test` маркеров в коде, покрывают все 12 задач

## Errors

- none

## Warnings

- Windows-runtime тесты (интеграционные с Wintun) не запускались — требуют хост с Windows + wintun.dll

## Not Verified

- Интеграционный тест Wintun loopback write/read (требует Windows хоста)
- DNS через luid.SetDNS() — отложено как post-MVP (DEC-005)

## Next Step

- safe to archive
