# Routing & Split Tunnel — Задачи

## Phase Contract

Inputs: spec, plan, data-model, plan.digest, spec.digest
Outputs: tasks.md с фазами, Surface Map, покрытие AC

## Implementation Context

- **Цель MVP:** default_route server/direct + CIDR split tunnel + ordered rules engine + server-side NAT (AC-001, AC-002, AC-006, AC-007, AC-009).
- **Инварианты:**
  - exclude → include → default, first match wins
  - default_route: `server` | `direct`, default `"server"` при отсутствии routing-секции
  - CIDR/IP валидируются при загрузке конфига (`netip.ParsePrefix` / `netip.ParseAddr`)
  - `RuleSet` immutable после создания
  - NAT — только nftables (exec.Command); если nft отсутствует — ошибка при старте
  - DNS override — TUN-level interception UDP 53 (не iptables REDIRECT)
- **Ошибки:**
  - invalid CIDR/IP в конфиге → load error, клиент не стартует
  - nftables не найден → log + NAT disabled
  - DNS timeout → fallback на default_route, log
- **Границы scope:** нет IPv6, нет hot-reload, нет iptables-legacy, нет Windows NAT
- **Proof signals:** `go test` pass, `curl --resolve` через туннель/direct, `tcpdump` подтверждает DNS override
- **References:** DEC-001–DEC-005, DM-001–DM-004

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/config/client.go` | T1.1, T2.1 |
| `configs/client.yaml` | T2.1 |
| `src/internal/routing/routing.go` | T1.1, T2.2 |
| `src/internal/routing/rule.go` | T2.2 |
| `src/internal/routing/matcher.go` | T2.2 |
| `src/internal/routing/rule_set.go` | T2.2 |
| `src/internal/nat/nat.go` | T1.1, T2.3 |
| `src/internal/nat/nftables.go` | T2.3 |
| `src/internal/dns/resolver.go` | T3.1 |
| `src/internal/dns/cache.go` | T3.1 |
| `src/internal/tun/` | T2.4 |
| `src/internal/routing/routing_test.go` | T2.5, T4.2 |
| `src/internal/dns/dns_test.go` | T3.2 |
| `src/internal/nat/nat_test.go` | T2.5 |

## Фаза 1: Основа

Цель: подготовить RoutingCfg, пустые пакеты и shared типы, от которых зависят все последующие фазы.

- [x] T1.1 Добавить RoutingCfg (DM-001) в ClientConfig, создать пакет dns/, обновить stub в nat/ и routing/
  Touches: src/internal/config/client.go, src/internal/dns/resolver.go, src/internal/dns/cache.go, src/internal/routing/routing.go, src/internal/nat/nat.go
  Details:
  - RoutingCfg struct с DefaultRoute + списками (DM-001)
  - DefaultRoute default `"server"` при отсутствии в YAML
  - RouteAction type (`server` | `direct`)
  - Пакет `dns/` — создать директорию + resolver.go с Resolver interface
  - routing/routing.go — обновить: `package routing`, экспорт RouteAction
  - nat/nat.go — обновить: `package nat`, Manager interface с Setup/Teardown

## Фаза 2: MVP Slice

Цель: config → engine → NAT → TUN integration, end-to-end рабочий flow default_route + CIDR split tunnel.

- [x] T2.1 Развернуть routing-секцию в client.yaml и интеграцию с LoadClientConfig
- [x] T2.2 Реализовать RuleSet, Matcher interface, CIDR/ExactIP matchers, ordered pipeline
- [x] T2.3 Реализовать NAT Manager с nftables MASQUERADE (DEC-005)
- [x] T2.4 Интегрировать routing engine в TUN write path (outbound packets)
- [x] T2.5 Unit-тесты routing engine + NAT
  Touches: src/internal/routing/routing_test.go, src/internal/nat/nat_test.go
  Details:
  - RuleSet: default route (AC-001), CIDR include/exclude (AC-002), exact IP (AC-003), ordering exclude>include (AC-006)
  - Edge cases: пустые списки, CIDR /0, excluded IP в included range
  - NAT: unit test на exec wrapper (mock exec.Command)

## Фаза 3: Основная реализация

Цель: DNS resolver + domain routing + DNS override.

- [x] T3.1 Реализовать DNS resolver с in-memory cache (DEC-003) + DomainMatcher
- [x] T3.2 Unit-тесты DNS resolver + domain matcher
- [x] T3.3 Реализовать DNS override для full-tunnel (DEC-004)
  Touches: src/internal/dns/resolver.go, src/internal/tun/*.go
  Details:
  - В full-tunnel режиме (default_route: server, все exclude-списки пусты) — перехватывать UDP 53 пакеты в TUN read path
  - Форвардить DNS-запросы через туннель на DNS-сервер (конфигурируемый, default 1.1.1.1)
  - Ответ от сервера возвращается через туннель обратно клиенту
  - AC-008 proof: tcpdump на клиенте не показывает UDP 53 вне tun0

## Фаза 4: Проверка

Цель: gate test, benchmark, финальная валидация.

- [x] T4.1 Gate integration test (AC-010) + performance benchmark
  Touches: (integration test в docker-compose окружении)
  Details:
  - Docker Compose сценарий: клиент c routing конфигом (default_route: direct + include_domains: [corp.example.com] + include_ranges: [10.0.0.0/8])
  - `dig +short youtube.com` + `curl --resolve` → direct (matched_rule=default)
  - `dig +short corp.example.com` + `curl --resolve` → tunnel (matched_rule=include_domain)
  - `go test -bench=. ./src/internal/routing/...` — <1µs per packet

- [x] T4.2 Backfill edge-case тесты и финальная проверка
  Touches: src/internal/routing/routing_test.go
  Details:
  - Edge: пустой конфиг, CIDR /0 эквивалент default, DNS timeout fallback, exclude + include conflict
  - `@sk-test` маркеры на тестах (constitution traceability)
  - `@sk-task` маркеры на exported функциях/структурах кода

## Покрытие критериев приемки

- AC-001 -> T2.2, T2.4, T2.5
- AC-002 -> T2.2, T2.4, T2.5
- AC-003 -> T2.2, T2.5
- AC-004 -> T3.1, T3.2
- AC-005 -> T3.1, T3.2
- AC-006 -> T2.2, T2.5
- AC-007 -> T2.3, T2.5
- AC-008 -> T3.3, T4.1
- AC-009 -> T1.1, T2.1
- AC-010 -> T4.1, T4.2
