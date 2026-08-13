---
report_type: verify
slug: dual-ws-channel
status: pass
docs_language: ru
generated_at: 2026-08-13
---

# Verify Report: dual-ws-channel

## Scope

- snapshot: второй WS-канал для UDP-трафика (VoIP-медиа) отдельно от TCP; классификация по IP-протоколу на обеих ролях; привязка secondary по `session_id`+токен; буфер до primary-ready; фоллбэк на одиночный канал; backward compat.
- verification_mode: default
- artifacts:
  - CONSTITUTION.md (через `.speckeep/constitution.summary.md`)
  - specs/active/dual-ws-channel/tasks.md
  - specs/active/dual-ws-channel/spec.md (AC-001…AC-008)
  - specs/active/dual-ws-channel/plan.md (DEC-001…DEC-006)
- inspected_surfaces:
  - `src/internal/tunnel/session.go` — `SetSecondary`, `secondaryToTun`, `tunToWS` классификация, буфер 300 мс/1024
  - `src/internal/tunnel/demux.go` — `parseIPProto`
  - `src/internal/bootstrap/server/handler.go` — secondary-ветка, `handleSecondaryStream`
  - `src/internal/bootstrap/server/server.go` — `tunnelSessRefs`
  - `src/internal/bootstrap/client/tun.go` — `dialSecondaryChannel`, фоллбэк
  - `src/internal/bootstrap/client/dial.go` — общий `dialStream` (один WSConfig)
  - `src/internal/config/client.go` — флаг `MultiChannel` (`multi_channel`)
  - `src/internal/protocol/handshake/handshake.go` — encode/decode ChannelTag/SessionTag
  - `src/integration/tunnel_integration_test.go`, `src/internal/tunnel/session_test.go`, `src/internal/protocol/handshake/handshake_test.go`

## Verdict

- status: pass
- archive_readiness: safe
- summary: все 9 задач закрыты с Proof, все 8 AC подтверждены тестами и code inspection; `check-ready verify`, `verify-task-state`, `go vet` и `go test -race ./src/...` — чистые.

## Checks

- task_state: completed=9, open=0; TASK_IDS=40, AC_COVERAGE_LINES=8; `./.speckeep/scripts/verify-task-state.sh dual-ws-channel` EXIT=0.
- acceptance_evidence:
  - AC-001 -> T3.1, T4.1 — `handleSecondaryStream` сверяет `sess.TokenName != tokenName` → reject; `TestTunnelDualChannelForeignTokenAndPrimarySurvives` PASS (foreign token → FrameTypeAuth).
  - AC-002 -> T2.2, T4.1 — `tunToWS` (`session.go:864`): `parseIPProto(payload)` → target=secondary; `TestTunnelDualChannelRoundtrip` PASS (TCP→primaryGot proto 6, UDP→secondaryGot proto 17).
  - AC-003 -> T2.2, T4.1 — серверный `tunToWS` шлёт UDP-ответ по secondary; `TestTunnelDualChannelRoundtrip` PASS (`reply:udp-payload` доставлен в TUN клиента).
  - AC-004 -> T3.2, T4.1 — `tun.go:399-400`: ошибка `dialSecondaryChannel` → warn + работа на primary; `TestTunnelDualChannelForeignTokenAndPrimarySurvives` PASS (primaryGot proto 17 после reject).
  - AC-005 -> T3.3, T4.2 — `secondaryToTun` (`session.go:793-810`): буфер до `primaryReady`, лимит 300 мс/1024, иначе drop; `TestSecondaryBufferAccumulatesThenFlushes`, `TestSecondaryBufferDropsOnOverflow` PASS.
  - AC-006 -> T3.2, T4.2 — общий `sessionCipher` для обоих каналов (primary/secondary decrypt на одном cipher); оба канала через единый `dialStream(cfg)` (один WSConfig/uTLS/padding); `TestSecondaryCryptoCrossChannel` PASS.
  - AC-007 -> T1.2, T4.2 — `DecodeClientHello` пропускает неизвестные теги, отсутствие тегов = primary; `TestClientHelloNoTagsBackwardCompat` PASS.
  - AC-008 -> T4.1, T4.2 — `go test -race -count=1 ./src/internal/... ./src/integration/...` EXIT=0; `go vet ./src/internal/... ./src/integration/...` EXIT=0.
- implementation_alignment:
  - Primary/secondary share one `tunnel.Session` (`SetSecondary`), secondary loop отдельный от errgroup (`startSecondaryLoop`).
  - Server registry `tunnelSessRefs` keyed by session_id, store before `Run`, delete in defer (`handler.go:206-208`).
  - Backward compat: `handshake.go` encode/decode тегов условно; `types_gen.go` regenerated (неправка вручную отсутствует).

## Errors

- none

## Warnings

- none

## Questions

- none

## Not Verified

- Ручной сценарий звонка через kvn-web в TUN с `multi_channel: true` — не выполнялся (требует живого клиента/сервера); подтверждается только интеграционным тестом.

## Next Step

- safe to archive
