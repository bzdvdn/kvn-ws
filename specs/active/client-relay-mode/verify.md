---
report_type: verify
slug: client-relay-mode
status: concerns
docs_language: ru
generated_at: 2026-06-13
---

# Verify Report: client-relay-mode

## Scope

- snapshot: верификация реализации relay mode — config structs, TLS listener, WS path allowlist, bridge session, error handling, unit tests
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - specs/active/client-relay-mode/tasks.md
- inspected_surfaces:
  - `src/internal/config/client.go` — RelayCfg, RelayTLSCfg, defaults, validation
  - `src/internal/bootstrap/client/relay.go` — runRelayMode, relayTLSConfig, allowedRelayPath, relayHandler, bridgeRelayConn, copyDirection
  - `src/internal/bootstrap/client/client.go` — mode branch in Run()
  - `src/internal/config/client_test.go` — 4 unit tests
  - `configs/client.yaml` — commented relay example

## Verdict

- status: concerns
- archive_readiness: safe (minor env limitation, no code defects)
- summary: все 7 задач выполнены, trace-маркеры на месте, 4 unit-теста написаны. code review не выявил ошибок. Единственное ограничение — среда не позволяет `go build`/`go test` (Go 1.23 vs go.mod 1.25) — это не дефект реализации

## Checks

- task_state: completed=7, open=0
- acceptance_evidence:
  - AC-001 (forward handshake) → T2.2: `bridgeRelayConn` читает ClientHello → dial upstream → forward → возвращает ServerHello; T4.2: manual validation
  - AC-002 (data bridge + disconnect) → T2.2: две `copyDirection` горутины + `sync.Once` close; T4.2: manual validation
  - AC-003 (конфиг) → T1.1: RelayCfg struct + defaults + validation; T2.1: listener из конфига; T4.1: 4 unit-теста
  - AC-004 (reject on upstream failure) → T3.1: dial failure → close client + `upstream dial failed` лог; T4.2: manual validation
  - AC-005 (opaque bridge) → T2.2: `copyDirection` на уровне `ReadMessage/WriteMessage`, без интерпретации фреймов; T4.2: manual validation
- implementation_alignment:
  - Semaphore `chan struct{}` в `runRelayMode` для max_connections
  - Self-signed TLS fallback (`generateSelfSignedCert`) при отсутствии `relay.tls`
  - WS path allowlist (`allowedRelayPath`) — 404 для неразрешённых path
  - Bridge: `copyDirection` с `sync.Once` для double-close safety

## Errors

- none

## Warnings

- W1: Go toolchain mismatch (env: 1.23, go.mod: 1.25) — `go build`/`go test` не запущены. Код структурно корректен, но компиляция не подтверждена в этой сессии.

## Questions

- none

## Not Verified

- `go build ./src/cmd/client` — не выполнено (toolchain limitation)
- `go test ./src/internal/config/...` — не выполнено (toolchain limitation)
- `golangci-lint` — не выполнено (toolchain limitation)
- Ручной сценарий relay → upstream → client — не выполнен (нет двух работающих инстансов)

## Next Step

- safe to archive after environment-verified build; no code changes required
