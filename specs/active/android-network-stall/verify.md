---
report_type: verify
slug: android-network-stall
status: pass
docs_language: ru
generated_at: 2026-11-01
---

# Verify Report: android-network-stall

## Scope

- snapshot: проверены исправления залипаний сети Android-клиента (forward lifecycle, keepalive/health, async direct DNS, bounded TUN stop, ctx propagation).
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - specs/active/android-network-stall/spec.md
  - specs/active/android-network-stall/tasks.md
- inspected_surfaces:
  - src/internal/tcpstack/stack.go
  - src/internal/transport/client.go
  - src/internal/tlsprofile/profile.go
  - src/internal/client/client.go
  - src/internal/client/direct.go
  - src/mobile/mobile.go
  - src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt

## Verdict

- status: pass
- archive_readiness: safe
- summary: все 6 AC подтверждены кодом и автоматическими тестами; прогоны `go test -race`, `go vet` и `compileDebugKotlin` зелёные.

## Checks

- task_state: completed=12, open=0; `verify-task-state.sh android-network-stall` — OK.
- trace: `trace android-network-stall --files` — все изменённые файлы размечены, `Unmapped` нет; `--tests` нашёл 4 test-proof.
- acceptance_evidence:
  - AC-001 -> `TestForwardRequestsReleased` (5 соединений при лимите 2, `ForwardMaxInFlightDrop=0`).
  - AC-002 -> keepalive `ReadIdleTimeout=15s`/`PingTimeout=10s` + `TestClientHealthBoundedOnBlackhole` (bounded failure).
  - AC-003 -> `TestClientHealthReusesConnection` (сервер видит 1 соединение на Session+Health).
  - AC-004 -> `TestHandleDirectDNSEscapesReadLoop` (обработчик возвращается <100 мс при блокирующемся резолве).
  - AC-005 -> `TestFDDeviceReadStopsOnClose` (чтение завершается после Close за ≤500 мс; poll-таймаут 100 мс) + порядок stop→close fd в `stopClientAndTun`.
  - AC-006 -> `TestStackDialContextCanceled` (ctx dial отменяется вместе с контекстом стека).
- implementation_alignment:
  - `req.Complete(false)` вызывается сразу после `CreateEndpoint`; слот forwarder освобождается.
  - Android health использует `active.health()` на живом `transport.Client`.
  - Резолв excluded-доменов вынесен в пул из 8 воркеров, lookup инъектируемый.
  - TUN fd переведён в non-blocking с `poll` и stop-сигналом; Go не закрывает чужой fd.

## Errors

- none

## Warnings

- none

## Questions

- none

## Not Verified

- Реальный NAT/Doze-blackhole и длительный soak на физическом Android-устройстве не воспроизводились; проверено конфигурацией keepalive, bounded health и unit/integration-тестами.
- Полный `assembleDebug`/on-device smoke не запускался; выполнены `gomobile bind` (AAR пересобран) и `:app:compileDebugKotlin`.

## Next Step

- safe to archive
