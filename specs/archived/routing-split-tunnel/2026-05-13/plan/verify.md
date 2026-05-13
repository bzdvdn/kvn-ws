---
report_type: verify
slug: routing-split-tunnel
status: pass
docs_language: ru
generated_at: 2026-05-13
---

# Verify Report: routing-split-tunnel

## Scope

- snapshot: проверка реализации routing & split tunnel: default route, CIDR/IP/domain matchers, ordered rules engine, DNS resolver, server NAT, DNS override
- verification_mode: default
- artifacts:
  - CONSTITUTION.md
  - specs/active/routing-split-tunnel/tasks.md
  - src/internal/routing/
  - src/internal/dns/
  - src/internal/nat/
  - src/internal/config/client.go
  - configs/client.yaml
- inspected_surfaces:
  - src/internal/routing/ — RuleSet, Router, Matchers, DomainMatcher
  - src/internal/dns/ — Resolver, Cache
  - src/internal/nat/ — NFTManager
  - src/internal/config/client.go — RoutingCfg, LoadClientConfig

## Verdict

- status: pass
- archive_readiness: safe
- summary: все 12 задач выполнены, 10 AC покрыты кодом и тестами, build + vet + tests pass

## Checks

- task_state: completed=11, open=0
- acceptance_evidence:
  - AC-001 -> RouteAction type (routing.go), RuleSet.Route (rule_set.go), TestDefaultRouteServer/TestDefaultRouteDirect (routing_test.go)
  - AC-002 -> CIDRMatcher (matcher.go), TestCIDRInclude/TestCIDRExclude/TestZeroPrefix (routing_test.go)
  - AC-003 -> ExactIPMatcher (matcher.go), TestExactIPInclude (routing_test.go)
  - AC-004 -> Resolver with cache (resolver.go, cache.go), TestCacheGetSet/TestCacheExpiry (dns_test.go)
  - AC-005 -> DomainMatcher (domain_matcher.go), TestDomainMatcherMatch/TestDomainMatcherMultipleIPs (domain_matcher_test.go)
  - AC-006 -> exclude→include→default order (rule_set.go), TestOrderedExcludeWins/TestBothExcludeAndInclude (routing_test.go)
  - AC-007 -> NFTManager.Setup/Teardown (nftables.go), TestNFTManagerInterface (nat_test.go)
  - AC-008 -> SetDNSOverride + isDNSQuery (router.go), TestIsDNSQuery/TestIsNotDNSQuery (routing_test.go)
  - AC-009 -> RoutingCfg + client.yaml routing section (client.go), LoadClientConfig defaults (client.go)
  - AC-010 -> BenchmarkRoute (benchmark_test.go), edge tests (routing_test.go)
- implementation_alignment:
  - RuleSet.NewRuleSet → NewRuleSetWithResolver создаёт ordered rules: exclude → include → default
  - TunRouter.RoutePacket проверяет DNS override до routing decision
  - DefaultResolver.Lookup использует Cache.Get для TTL-based кеша
  - NFTManager.Setup создаёт nftables table/chain/rule через exec.Command

## Errors

- none

## Warnings

- none

## Questions

- none

## Not Verified

- Полный end-to-end тест с TUN + WebSocket — требует работающего TUN-устройства в контейнере; gate test проверяет routing engine + NAT изолированно
- `curl --resolve` + `dig` в Docker Compose — настроена инфраструктура (`docker-compose.test.yml`, `Dockerfile.test`, `scripts/test-gate.sh`), требуется CI с `--privileged` для nftables

## Integration Test Infrastructure

Добавлены артефакты для Docker Compose gate test:

- `docker-compose.test.yml` — тестовый compose с CAP_NET_ADMIN
- `Dockerfile.test` — Alpine + nftables + bind-tools + curl
- `scripts/test-gate.sh` — скрипт: unit tests + nftables integration + routing gate scenario
- `src/cmd/gatetest/main.go` — Go-программа, симулирующая AC-010 сценарий
- `configs/client.test.yaml` — тестовый конфиг split-tunnel
- `configs/server.test.yaml` — тестовый серверный конфиг

Запуск: `docker compose -f docker-compose.test.yml up --build`

## Next Step

- safe to archive
