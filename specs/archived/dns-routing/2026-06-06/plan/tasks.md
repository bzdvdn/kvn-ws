# DNS-зависимая маршрутизация — Задачи

## Phase Contract

Inputs: `specs/active/dns-routing/spec.md`, `specs/active/dns-routing/plan.md`.
Outputs: задачи с покрытием AC-001, AC-002, AC-003, AC-004, AC-005, AC-060.
Stop if: нет.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/routing/domain_matcher.go` | T1.1 |
| `src/internal/routing/routing.go` | T2.1 |
| `src/internal/routing/rule_set.go` | T3.1 |
| `src/internal/routing/dns.go` | T4.1 |
| `src/internal/routing/router.go` | T5.1 |
| `src/internal/bootstrap/client/proxy.go` | T6.1 |
| `src/internal/routing/domain_matcher_test.go` | T7.1 |
| `src/internal/routing/routing_test.go` | T7.2 |
| `src/internal/routing/dns_test.go` | T7.3 |

## Implementation Context

- Цель MVP: suffix matching (`.ru`, `.ozon.ru`) для exclude/include доменов в TUN + proxy mode
- Границы приемки: AC-001, AC-002, AC-003, AC-004, AC-005, AC-060
- Ключевые правила:
  - trace-маркеры `@sk-task` обязательны на изменённых объявлениях
  - Suffix-домены с `.`-префиксом не резолвятся через DNS
  - Точные домены без точки (`example.com`) продолжают работать через DNS resolve + IP match
  - DNS-интерсептор только UDP/53 A/AAAA, не TCP/DoH/DoT
- Инварианты: `DomainMatcher.Match(ip)` не меняется; `TunRouter.RoutePacket` fallback не меняется
- Контракты: `.ru` матчит `hh.ru`, `mail.ru`, `sub.hh.ru`; не матчит голое `ru`
- Proof signals: `go build ./...`, `go test ./src/internal/routing/... -v`
- Вне scope: DoH/DoT interception, CNAME chain resolution, EDNS0/ECS, IPv6 DNS intercept

## Фаза 1: Основа

Осознанно пропущена — `DomainMatcher`, `RuleSet`, `TunRouter` уже существуют.

## Фаза 2: Реализация

- [x] T1.1 Добавить suffix support в `DomainMatcher`.
  Touches: `src/internal/routing/domain_matcher.go`
  - Добавить `suffixes []string` поле в структуру
  - `NewDomainMatcher` разделяет входные домены: `.`-prefix → `suffixes`, иначе → `domains`
  - `MatchDomain(domain string) bool` — `strings.HasSuffix(domain, suffix)` по `suffixes`
  - `refreshCache` пропускает suffix-домены (не резолвятся)
  - AC-001, AC-002

- [x] T2.1 Добавить `RouteNone` sentinel.
  Touches: `src/internal/routing/routing.go`
  - `RouteNone RouteAction = 0` для случая "нет совпадения по домену"
  - RouteServer=1, RouteDirect=2 не меняются
  - AC-001

- [x] T3.1 Добавить suffix domain matching в `RuleSet`.
  Touches: `src/internal/routing/rule_set.go`
  - Добавить `suffixDomains map[RouteAction][]string` поле
  - `addRules` извлекает `.`-prefix домены в `suffixDomains[action]`
  - `MatchDomain(domain string) RouteAction` — проход по `suffixDomains`, возврат `RouteDirect`/`RouteServer`/`RouteNone`
  - AC-001, AC-002

- [x] T4.1 Реализовать DNS question parser.
  Touches: `src/internal/routing/dns.go`
  - `ParseDNSQuestion(packet []byte) (string, bool)` — принимает IPv4 UDP DNS пакет, возвращает QNAME
  - Парсинг: IP header (IHL) → UDP header (8B) → DNS header (12B) → QNAME (length-prefixed labels)
  - Только первый question
  - AC-001, AC-002

- [x] T5.1 Добавить domain-based DNS intercept в `TunRouter`.
  Touches: `src/internal/routing/router.go`
  - В `RoutePacket`: при `isDNSQuery(packet)` → парсинг QNAME → `ruleSet.MatchDomain(qname)`
  - RouteDirect → `sendDirect`, RouteServer → `sendTunnel`, RouteNone → fallback к `routeByRule`
  - DNS override (все DNS через туннель) продолжает работать с приоритетом
  - AC-001, AC-002, AC-004

- [x] T6.1 Добавить domain check в proxy mode handler.
  Touches: `src/internal/bootstrap/client/proxy.go`
  - В handler: `routeSet.MatchDomain(host)` до существующего DNS resolve + IP check
  - RouteDirect → сразу direct (без DNS lookup)
  - RouteServer/RouteNone → fallthrough к IP-based routing
  - AC-060

- [x] T7.1 Написать unit тесты для `MatchDomain`.
  Touches: `src/internal/routing/domain_matcher_test.go`
  - `TestMatchDomainSuffixRu` — `.ru` матчит `hh.ru`, не матчит `google.com`
  - `TestMatchDomainSuffixOzon` — `.ozon.ru` матчит `api.ozon.ru`, не матчит `ozon.com`
  - `TestMatchDomainBareRu` — `.ru` не матчит голое `ru`
  - `TestMatchDomainExactWithoutDot` — `example.com` без точки не матчится через MatchDomain
  - `TestNewDomainMatcherSplit` — проверка разделения exact/suffix
  - AC-001, AC-002, AC-003

- [x] T7.2 Написать unit тесты для `RuleSet.MatchDomain`.
  Touches: `src/internal/routing/routing_test.go`
  - `TestRuleSetMatchDomainExclude` — exclude_domains: [.ru, .ozon.ru] → hh.ru → RouteDirect
  - `TestRuleSetMatchDomainInclude` — include_domains: [.corp] → internal.corp → RouteServer
  - `TestRuleSetMatchDomainEmpty` — без suffix domains → RouteNone
  - AC-001, AC-002

- [x] T7.3 Написать unit тесты для `ParseDNSQuestion`.
  Touches: `src/internal/routing/dns_test.go`
  - `TestParseDNSQuestionValid` — валидный A query для hh.ru
  - `TestParseDNSQuestionMultiLabel` — multi-label api.ozon.ru
  - `TestParseDNSQuestionTruncated` — короткий пакет → false
  - `TestParseDNSQuestionNoQuestion` — пакет без DNS question → false
  - AC-001, AC-003

## Фаза 3: Проверка

Осознанно пропущена — покрыта тестами в T7.1–T7.3

## Покрытие критериев приемки

- AC-001 -> T1.1, T2.1, T3.1, T4.1, T5.1, T7.1, T7.2, T7.3
- AC-002 -> T1.1, T3.1, T4.1, T5.1, T7.1, T7.2
- AC-003 -> T7.1, T7.3
- AC-004 -> T5.1
- AC-005 -> T5.1, T6.1
- AC-060 -> T6.1

## Заметки

- trace-маркеры `@sk-task dns-routing#T1.1`, `@sk-task dns-routing#T2.1` и т.д. на изменённых объявлениях
- Nil resolver в `NewRuleSet` не влияет на suffix domains — они не требуют DNS
- DNS-override (dnsOverride) имеет приоритет над domain-based routing
