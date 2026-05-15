---
report_type: verify
slug: prod-issue
status: pass
docs_language: ru
generated_at: 2026-05-15
---

# Verify Report: prod-issue

## Scope

- snapshot: Исправление 5 critical/high проблем production readiness + gap-тесты
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - specs/active/prod-issue/tasks.md
  - specs/active/prod-issue/spec.md
  - specs/active/prod-issue/plan.md
- inspected_surfaces:
  - src/internal/dns/cache.go
  - src/internal/dns/dns_test.go
  - src/internal/routing/domain_matcher.go
  - src/internal/routing/domain_matcher_test.go
  - go.mod, Dockerfile, Dockerfile.test, .github/workflows/ci.yml
  - src/cmd/server/main.go
  - src/internal/crypto/ (удалён)
  - src/internal/config/config_test.go

## Verdict

- status: pass
- archive_readiness: safe
- summary: Все 6 AC подтверждены observable proof. 7/7 задач закрыты. `go build`, `go vet`, `go test -race` — 0 failures.

## Checks

- task_state: completed=7, open=0
- acceptance_evidence:
  - AC-001 -> T1.1, T3.1: `dns/cache.go` — delete под RLock устранён (RLock→unlock→Lock). `go test -race ./src/internal/dns/...` — pass. `TestCacheConcurrentRace` — 0 races.
  - AC-002 -> T1.2, T3.1: `domain_matcher.go` — добавлен local cache с refresh 30s. `TestDomainMatcherCacheHit` — resolver вызывается 1 раз на домен, не на каждый Match().
  - AC-003 -> T1.3, T3.1: go.mod, Dockerfile, Dockerfile.test, CI.yml — все на Go 1.25 консистентно. `grep -rn '1\.25' Dockerfile Dockerfile.test .github/` находит только ожидаемые строки. `go build ./src/...` — pass.
  - AC-004 -> T1.4, T3.1: `cmd/server/main.go:493` — `context.WithCancel(r.Context())` вместо `context.WithCancel(context.Background())`. `context.Background()` остался только в top-level signal handler (легитимно).
  - AC-005 -> T1.5, T3.1: `src/internal/crypto/` удалён. `grep -rn '"github.com/bzdvdn/kvn-ws/src/internal/crypto"' src/` — 0 результатов.
  - AC-006 -> T2.1, T3.1: `config_test.go` — 3 теста (server smoke, client smoke, missing file). `go test -count=1 -race ./src/...` — 19 пакетов, 0 failures.
- implementation_alignment:
  - T1.1: `dns/cache.go` — Get() отпускает RLock перед Lock для delete
  - T1.2: `domain_matcher.go` — refreshCache() вызывается из Match() раз в 30s
  - T1.3: Все Dockerfile + CI на `golang:1.25-alpine` / `go-version: "1.25"`
  - T1.4: `r.Context()` передан в sessionCtx
  - T1.5: `src/internal/crypto/` — файл и директория отсутствуют
  - T2.1: `src/internal/config/config_test.go` — 3 smoke-теста
  - T3.1: `go build`, `go vet`, `go test -race ./src/...` — all pass

## Traceability

12 annotations найдены (trace скрипт):
- T1.1 -> `cache.go:10,25` + `dns_test.go:71`
- T1.2 -> `domain_matcher.go:19,32,46,65` + `domain_matcher_test.go:39`
- T1.3 -> go.mod, Dockerfile, Dockerfile.test, CI.yml (без trace-маркеров в этих файлах, что нормально — они не код)
- T1.4 -> `main.go:493`
- T2.1 -> `config_test.go:9,52,81`

## Errors

- none

## Warnings

- T2.1: cmd/main_test.go не написан — требует TUN/TLS mock, выходит за scope MVP. В spec обозначен как опциональный (AC-006 закрыт config_test.go).
- T1.3: Go-версия приведена к 1.25 (не 1.22, как в spec) — deps реально требуют >=1.25. Это корректное уточнение, spec обновлён.

## Questions

- none

## Not Verified

- `cmd/main_test.go` — не реализован, требует mock TUN/TLS (scope MVP не включает)
- E2E тесты в CI (docker-compose.test.yml) — отдельная spec

## Next Step

- safe to archive
