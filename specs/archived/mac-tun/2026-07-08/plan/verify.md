---
report_type: verify
slug: mac-tun
status: pass
docs_language: ru
generated_at: 2026-07-08
---

# Verify Report: mac-tun

## Scope

- snapshot: macOS TUN Device (utun + ifconfig/route + Web UI + LaunchDaemon) — все 11 AC, 9 задач, 6 фаз
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - specs/active/mac-tun/spec.md
  - specs/active/mac-tun/plan.md
  - specs/active/mac-tun/tasks.md
- inspected_surfaces:
  - src/internal/tun/tun_darwin.go (Open, Close, Read, Write, SetIP, SetMTU, SetGateway, RemoveGateway, AddExcludeRoute, RemoveExcludeRoute, CleanupExcludeRoutes, CleanupStaleExcludeRoutes, SaveDefaultRoute, parseRouteGetOutput, parseField)
  - src/internal/tun/tun_stub.go (build tag)
  - src/internal/tun/tun_darwin_test.go (5 unit tests)
  - src/internal/tun/tun_common.go (routeMeta, TunDevice interface — no change)
  - src/internal/webui/server.go (handlePlatform — tun_supported + darwin)
  - scripts/build.sh (darwin cross-compile target)
  - scripts/com.kvn.tun.plist (LaunchDaemon)
  - scripts/com.kvn.web.plist (LaunchAgent)
  - docs/en/deployment.md, docs/ru/deployment.md (macOS TUN section)

## Verdict

- status: pass
- archive_readiness: safe
- summary: все 9 задач завершены, 11 AC покрыты кодом и/или тестами, cross-compile проходит, traceability полная

## Verification Matrix

| AC-ID | Task IDs | Evidence | Verdict |
|-------|----------|----------|---------|
| AC-001 | T1.1 | `tun_darwin.go:32` Open() создаёт utun через `tun.CreateTUN()` | pass |
| AC-002 | T1.1 | `tun_darwin.go:57` Read(), `:67` Write() без virtio headroom | pass |
| AC-003 | T2.1 | `tun_darwin.go:76` SetIP() через `ifconfig`, `:86` SetMTU() через `ifconfig` | pass |
| AC-004 | T3.1 | `tun_darwin.go:96` SetGateway(), `:106` RemoveGateway() через `route` | pass |
| AC-005 | T3.2 | `tun_darwin.go:116` AddExcludeRoute, `:136` RemoveExcludeRoute, `:146` CleanupExcludeRoutes, `:189` CleanupStaleExcludeRoutes | pass |
| AC-006 | T3.2 | `tun_darwin.go:48` Close() вызывает CleanupExcludeRoutes() | pass |
| AC-007 | T5.1 | `scripts/com.kvn.tun.plist`, `scripts/com.kvn.web.plist` | pass |
| AC-008 | T1.2 | `scripts/build.sh:56` darwin target; `GOOS=darwin go build ./src/internal/...` — pass | pass |
| AC-009 | T1.1 | `tun_stub.go:1` build tag `!linux && !windows && !darwin` | pass |
| AC-010 | T3.1, T6.1 | `tun_darwin.go:162` SaveDefaultRoute через `route -n get default` + 5 тестов: `TestParseRouteGetOutput`, `TestParseRouteGetOutputNoGateway`, `TestParseRouteGetOutputPartial`, `TestParseField`, `TestParseFieldEmpty` | pass |
| AC-011 | T4.1 | `server.go:115` tun_supported включает darwin | pass |

## Checks

- task_state: completed=9, open=0
- acceptance_evidence: 11/11 AC — подтверждены кодом, тестами и/или билд-проверками
- implementation_alignment:
  - Все функции TunDevice реализованы в tun_darwin.go с `//go:build darwin`
  - SetIP/SetMTU/маршруты — через `exec.Command("ifconfig"/"route")`
  - SaveDefaultRoute парсит stdout `route -n get default`
  - Web UI: tun_supported = true на darwin
  - Build: `GOOS=darwin GOARCH=amd64` проходит для tun, web, client/server пакетов
  - Close() вызывает CleanupExcludeRoutes() как на Linux/Windows
- traceability: 23 `@sk-task` / `@sk-test` маркеров в коде, покрывают все 9 задач

## Errors

- none

## Warnings

- Интеграционные тесты (utun loopback write/read) требуют macOS хоста с root
- LaunchDaemon/LaunchAgent plist не проверялись — требуется macOS для загрузки через launchctl

## Not Verified

- Интеграционный тест utun loopback write/read (требует macOS + root)
- LaunchDaemon/LaunchAgent загрузка (требует macOS + root)
- DNS через networksetup — отложено как post-MVP (DEC-005)

## Next Step

- safe to archive
