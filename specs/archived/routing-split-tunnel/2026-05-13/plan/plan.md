# Routing & Split Tunnel — План

## Phase Contract

Inputs: spec routing-split-tunnel, inspect (pass), repo surfaces (config, routing stub, nat stub)
Outputs: plan.md, plan.digest.md, data-model.md

## Цель

Добавить клиентскую маршрутизацию IP-пакетов: default_route server/direct, split tunnel по CIDR/IP/доменам, ordered rules engine. На сервере — nftables MASQUERADE. DNS resolver для доменных правил. DNS override для full-tunnel.

## MVP Slice

default_route server/direct + CIDR split tunnel + ordered rules engine + server-side NAT. Закрывает AC-001, AC-002, AC-006, AC-007, AC-009.

*AC-007 явно добавлен в MVP: без MASQUERADE пакеты «через туннель» не выходят в интернет.*

## First Validation Path

1. `go test ./src/internal/routing/...` — unit-тесты engine + matchers
2. Развернуть сервер + клиент в Docker Compose
3. `curl --resolve example.com:80:10.10.0.2 http://example.com` — пакет идёт через туннель (tcpdump на сервере)
4. `curl --resolve example.com:80:8.8.8.8 http://example.com` — пакет идёт напрямую (default_route: direct)

## Scope

- `src/internal/routing/` — engine, matchers, rules
- `src/internal/nat/` — nftables MASQUERADE setup/teardown
- `src/internal/config/` — routing-секция ClientConfig
- `src/internal/dns/` — новый пакет: resolver + cache (in-memory)
- `configs/client.yaml` — routing-блок

## Implementation Surfaces

- **config-routing** (`src/internal/config/client.go`) — расширение ClientConfig: `Routing RoutingCfg`. Новая структура.
- **routing-engine** (`src/internal/routing/`) — существующий stub. Полная замена: `RuleSet`, `Matcher` interface, CIDR/IP/Domain matchers.
- **dns-resolver** (`src/internal/dns/`) — новый пакет. `Resolver` (cache + upstream lookup), `Cache` (TTL-keys).
- **nat-manager** (`src/internal/nat/`) — существующий stub. `Manager` с методами Setup/Teardown, exec nft.
- **client-yaml** (`configs/client.yaml`) — добавление секции `routing`.

## Bootstrapping Surfaces

- `src/internal/dns/` — создать пакет (не существует)
- `src/internal/routing/rule.go`, `src/internal/routing/matcher.go` — новые файлы для разделения логики

## Влияние на архитектуру

- ClientConfig расширяется без breaking changes (опциональное поле Routing)
- TUN-устройство вызывает routing engine при получении пакета из стека ОС (outbound)
- NAT — серверная часть, независима от клиентской маршрутизации
- DNS resolver — новая зависимость на клиенте, in-memory, без persist

## Acceptance Approach

- AC-001 (default route): конфиг → engine → routing decision → integration test с curl
- AC-002 (CIDR): matcher CIDR → engine exclude/include → integration test
- AC-003 (IP): matcher exact IP → engine (аналогично CIDR)
- AC-004 (DNS cache): unit test с fake DNS upstream, проверка TTL
- AC-005 (domain routing): domain → DNS resolve → IP → CIDR matcher → engine
- AC-006 (ordered rules): unit test pipeline exclude→include→default
- AC-007 (NAT): server start → nft ruleset check → `tcpdump` + `curl`
- AC-008 (DNS override): full-tunnel config → tcpdump на клиенте → все UDP 53 через tun0
- AC-009 (config): unit test LoadClientConfig с/без routing-секции
- AC-010 (gate): ручной прогон `dig` + `curl --resolve` в Docker Compose

Все surfaces: routing-engine, config-routing, dns-resolver, nat-manager, client-yaml.

## Данные и контракты

См. `data-model.md`.

## Стратегия реализации

### DEC-001: exclude→include→default ordered pipeline

- Why: spec (RQ-002) требует exclude→include→default. Простейший sequential match. Не надо priority/weight system.
- Tradeoff: порядок фиксирован, не конфигурируется.
- Affects: routing-engine.
- Validation: AC-006 unit test.

### DEC-002: CIDR matching через net/netip

- Why: Go 1.22+ stdlib `net/netip.Prefix.Contains` — zero dep, эффективно.
- Tradeoff: IPv4-only до выхода из scope.
- Affects: routing-engine.
- Validation: AC-002, AC-003 unit tests.

### DEC-003: DNS resolver — in-memory cache + sync.Mutex

- Why: in-memory — достаточно, persist не нужен (Assumptions). `net.DefaultResolver` для upstream.
- Tradeoff: не thread-safe без mutex (используем `sync.RWMutex`).
- Affects: dns-resolver.
- Validation: AC-004 unit test.

### DEC-004: DNS override — TUN-level interception

- Why: перехват UDP 53 в TUN read path, а не iptables REDIRECT — кросс-платформенно (Linux/macOS/WS).
- Tradeoff: DNS на localhost (127.0.0.1:53) не перехватывается; only через TUN.
- Affects: routing-engine, dns-resolver, tun (чтение пакетов).
- Validation: AC-008 integration test.

### DEC-005: Server-side NAT — nftables exec wrapper

- Why: `exec.Command("nft", ...)` — простейшая интеграция. Нет внешней Go-зависимости.
- Tradeoff: требует nftables на сервере; error при старте если нет.
- Affects: nat-manager.
- Validation: AC-007 (nft list ruleset).

## Incremental Delivery

### MVP (Первая ценность)

- Config: RoutingCfg + client.yaml
- Engine: exclude→include→default, CIDR matcher, exact IP matcher
- NAT: nftables MASQUERADE setup
- Coverage: AC-009, AC-001, AC-006, AC-002, AC-003, AC-007

**Критерий:** `curl --resolve example.com:80:10.10.0.2 http://example.com` через туннель, `curl --resolve example.com:80:8.8.8.8 http://example.com` напрямую.

### Итеративное расширение

**Iteration 2 (DNS + domains):** DNS resolver + domain matcher. AC-004, AC-005.
- Валидация: `curl --resolve internal.corp.ru:80:10.10.10.10 http://internal.corp.ru` через туннель.

**Iteration 3 (DNS override + gate):** DNS override for full-tunnel. AC-008, AC-010.
- Валидация: `tcpdump` не показывает DNS вне туннеля; gate test с YouTube/corp.

## Порядок реализации

1. **Config (AC-009):** RoutingCfg + defaults. Независимо.
2. **Engine (AC-001, AC-002, AC-003, AC-006):** RuleSet + matchers + pipeline. Unit-тесты.
3. **NAT (AC-007):** nftables wrapper. Интеграционный тест.
4. **Интеграция engine + tun:** routing engine вызывается в TUN write path.
5. **DNS resolver + domain rules (AC-004, AC-005):** зависит от engine.
6. **DNS override (AC-008):** зависит от DNS resolver.
7. **Gate test (AC-010):** последним, всё остальное должно работать.

Параллельно: Config (1) и NAT (3) — независимы. Engine (2) зависит от Config.

## Риски

- **Риск 1: nftables нет на сервере** — ошибка при старте, graceful fallback с логом. Mitigation: проверка `exec.Command("nft", "--version")` в Setup.
- **Риск 2: Производительность routing (SC: <1µs)** — матчинг на каждый пакет. Mitigation: benchmark в CI, при превышении — trie-оптимизация (отложено).
- **Риск 3: DNS resolver timeout** — блокировка пакета. Mitigation: timeout + fallback на default_route.

## Rollout и compatibility

- Routing-секция в конфиге опциональна → backward compatible
- NAT — новый код на сервере, не влияет на существующие сессии
- DNS resolver — клиентский, не влияет на сервер
- Feature flag не требуется

## Проверка

- `go test ./src/internal/routing/...` — unit (AC-001–AC-006, AC-009)
- `go test ./src/internal/dns/...` — unit (AC-004)
- `go test ./src/internal/nat/...` — unit (AC-007)
- Docker Compose integration: `curl --resolve` checks (AC-010)
- `go test -bench=. ./src/internal/routing/...` — performance SC
- `tcpdump` + `nft list ruleset` — manual validation (AC-007, AC-008)

## Соответствие конституции

- нет конфликтов: Go, Clean Architecture, traceability (`@sk-task`), Docker multi-stage
- Новый пакет `src/internal/dns/` соответствует подходу domain/infra разделения
