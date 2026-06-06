---
report_type: verify
slug: fix-critical-leaks
status: pass
docs_language: ru
generated_at: 2026-06-07
---

# Verify Report: fix-critical-leaks

## Scope

- snapshot: проверка всех 16 задач по устранению критических утечек горутин, контекстов, deadlock'ов, проглоченных ошибок и проблем ресурсной безопасности в KVN tunnel
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - specs/active/fix-critical-leaks/tasks.md
  - specs/active/fix-critical-leaks/plan.md
- inspected_surfaces:
  - session/bolt.go (T1.1)
  - bootstrap/client/reconnect.go (T1.2)
  - bootstrap/server/server.go (T1.3)
  - admin/admin.go (T1.3)
  - tunnel/session.go (T1.3, T3.1, T5.1)
  - bootstrap/client/proxy.go (T1.4, T2.1, T2.2, T3.3)
  - transport/quic/dial.go (T2.1)
  - bootstrap/client/tun.go (T2.2)
  - routing/domain_matcher.go (T2.2)
  - webui/server.go (T2.3)
  - proxy/listener.go (T3.2)
  - session/session.go (T4.1)
  - bootstrap/client/helpers.go (T4.2)
  - config/config.go (T4.3)
  - transport/framing/framing.go (T5.1)
  - тунельные тесты: session, tunnel, bootstrap/client, proxy, transport/quic, routing, webui

## Verdict

- status: pass
- archive_readiness: safe
- summary: все 16 задач выполнены, 13 AC закрыты, `go build ./...` + `go test -race ./...` + `go vet ./...` проходят, traceability подтверждена (44 @sk-task + 14 @sk-test)

## Checks

- task_state: completed=16, open=0
- acceptance_evidence:
  - AC-001 (TUN goroutine leak) -> T3.1 (tunnel/session.go:96,302) + T6.1 (TestTunGoroutineLeak)
  - AC-002 (Proxy semaphore) -> T3.2 (proxy/listener.go:85,97) + T6.1 (TestProxySemaphore)
  - AC-003 (RouteDirect lifecycle) -> T3.3 (bootstrap/client/proxy.go:190,235) + T6.1 (TestRouteDirectLifecycle)
  - AC-004 (QUIC ctx cancel) -> T2.1 (transport/quic/dial.go:14) + T6.1 (TestQUICDialContextCancel)
  - AC-005 (DNS ctx propagation) -> T2.2 (bootstrap/client/proxy.go:220, routing/domain_matcher.go:91) + T6.1 (TestDNSContextPropagation)
  - AC-006 (WebUI broadcast shutdown) -> T2.3 (webui/server.go:69) + T6.1 (TestWebUIBroadcastShutdown)
  - AC-007 (BoltDB timeout) -> T1.1 (session/bolt.go:20,44) + T6.1 (TestBoltDBTimeout)
  - AC-008 (time.After leak) -> T1.2 (bootstrap/client/reconnect.go:23) + T6.1 (TestSleepWithContextTimerLeak)
  - AC-009 (swallowed errors) -> T1.3 (admin/admin.go:102,121; server/server.go:333,344; tunnel/session.go:214) + T6.2 (grep: 0 matches)
  - AC-010 (lock ordering) -> T4.1 (session/session.go:282,302,321,373,390) + T6.1 (TestSessionManagerLockOrdering)
  - AC-011 (errors.As) -> T1.4 (bootstrap/client/proxy.go:302) + T6.1 (TestTypeAssertionErrorsAs)
  - AC-012 (helpers.go) -> T4.2 (bootstrap/client/helpers.go:11,27) + T6.2 (trace verified)
  - AC-013 (sync.Pool) -> T5.1 (tunnel/session.go:27, framing/framing.go:37) + T6.2 (trace verified)
- implementation_alignment:
  - T3.1: `tunReadInterruptible` удалён, постоянная reader-горутина с каналом в `startTunReader`
  - T3.2: `Listener.sem` (chan struct{}) с capacity 1000, acquire в `AcceptLoop`, release в `handleClient`
  - T3.3: оба RouteDirect-блока используют `errgroup.WithContext(ctx)` + cleanup goroutine закрывает conn при gctx.Done()
  - T4.1: `cancelFuncsMu sync.Mutex` в SessionManager, двухфазный подход (сбор ID под mu, отмена после unlock)
  - T4.2: `clientTLSConfig()` + `parseBackoff()` в `helpers.go`, дубликаты удалены из proxy.go/tun.go
  - T4.3: `envPrefixForWarning` удалён, `secretFromEnv`/`warnSecretInFile` принимают prefix параметром
  - T5.1: `proxyBufPool sync.Pool` для 4KB буферов, `framing.GetBuffer` экспортирован, оба используются в proxy goroutine

## Errors

- none

## Warnings

- `golangci-lint` не запущен — устаревшая версия (v1.64.8) несовместима с Go 1.25
- `scripts/test-gate.sh` требует Docker-окружения (`/app`), пропущен

## Questions

- none

## Not Verified

- `golangci-lint run` — environment limitation, не влияет на корректность изменений
- `scripts/test-gate.sh` — Docker-зависимый скрипт, не относится к scope fix-critical-leaks

## Next Step

- safe to archive
