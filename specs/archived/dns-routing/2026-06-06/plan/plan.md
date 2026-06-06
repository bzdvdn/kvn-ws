# Plan: DNS-зависимая маршрутизация (wildcard exclude/include доменов)

## Scope

- Suffix matching (`.ru`, `.ozon.ru`) в `DomainMatcher` без DNS
- `RuleSet.MatchDomain(domain string)` — проверка suffix-доменов
- DNS-интерсептор на TUN: парсинг DNS question, QNAME → suffix match → direct/tunnel
- Proxy mode: проверка домена из `dst` стрима против suffix-правил
- Минимальный DNS question parser (без библиотек)

## Out of scope

- DoH/DoT interception
- CNAME chain resolution
- EDNS0/ECS обработка
- IPv6 DNS (AAAA) — пока только A records
- `dns_intercept` toggle в конфиге — включается автоматически при наличии suffix-доменов

## Changes

### 1. `routing/domain_matcher.go` — suffix support

- Добавить `suffixes []string` поле
- `NewDomainMatcher` разделяет входные домены: `.`-prefix → `suffixes`, иначе → `domains` (старое поведение)
- `MatchDomain(domain string) bool` — `strings.HasSuffix(domain, suffix)` по `suffixes`
- `Match(ip)` без изменений — работает только для exact domains
- `refreshCache` пропускает suffix-домены (не резолвятся)

### 2. `routing/routing.go` — RouteNone

- Добавить `RouteNone RouteAction = 0` для случая "нет совпадения по домену"

### 3. `routing/rule_set.go` — MatchDomain

- Добавить `suffixDomains map[RouteAction][]string` поле в `RuleSet`
- `addRules` заполняет `suffixDomains` для `.`-prefix доменов
- `NewRuleSetWithResolver` не создаёт `DomainMatcher` для suffix-доменов (они не резолвятся)
- `MatchDomain(domain string) RouteAction` — проход по `suffixDomains`, возврат действия или `RouteNone`

### 4. `routing/dns.go` (новый) — DNS question parser

- `ParseDNSQuestion(packet []byte) (string, bool)` — принимает IPv4 UDP пакет, возвращает QNAME
- Парсинг: IP header (IHL) → UDP header (8B) → DNS header (12B) → QNAME (length-prefixed labels)
- Только первый question, только A/AAAA

### 5. `routing/router.go` — DNS intercept

- В `RoutePacket`: при `isDNSQuery(packet)` → `parseAndRouteDNS(packet) RouteAction`
- `parseAndRouteDNS`: парсит QNAME, вызывает `ruleSet.MatchDomain(qname)`
- Если RouteDirect → `sendDirect` (не идёт в туннель)
- Если RouteServer → `sendTunnel` (идёт в туннель)
- Если RouteNone → fallback к `routeByRule` (старое поведение)

### 6. `bootstrap/client/proxy.go` — proxy domain check

- В handler функции, ДО существующего DNS resolve + IP check:
  - Вызвать `routeSet.MatchDomain(host)`
  - Если RouteDirect → сразу direct (без DNS lookup)
  - Если RouteServer/RouteNone → fallback к существующему IP-based routing

### 7. `routing/domain_matcher_test.go` — тесты suffix

- `TestMatchDomainSuffixRu` — `.ru` матчит `hh.ru`, не матчит `google.com`
- `TestMatchDomainSuffixLong` — `.ozon.ru` матчит `api.ozon.ru`, не матчит `ozon.com`
- `TestMatchDomainExactWithoutDot` — `example.com` без точки не матчится через MatchDomain
- `TestMatchDomainDotRuNotMatchBareRu` — `.ru` не матчит голое `ru`

### 8. `routing/routing_test.go` — RuleSet MatchDomain тесты

- `TestRuleSetMatchDomainExclude` — exclude_domains: [.ru] → hh.ru → RouteDirect, google.com → RouteNone
- `TestRuleSetMatchDomainInclude` — include_domains: [.corp] → internal.corp.ru → RouteServer

### 9. `routing/router_test.go` — DNS intercept тесты

- `TestDNSInterceptExclude` — DNS пакет для `.ru` домена → sendDirect
- `TestDNSInterceptNoMatch` — DNS пакет для `.com` домена → fallback к routeByRule
- `TestParseDNSQuestion` — валидный DNS пакет → QNAME
- `TestParseDNSQuestionTruncated` — короткий пакет → false

### 10. `bootstrap/client/proxy.go` — proxy domain тест

- Интеграционный: exclude_domains: [.ru] → запрос к hh.ru:443 → direct (без DNS resolve)

## Dependencies

- `std::strings` (уже импортирован)
- Новых external зависимостей нет
- DNS question parser — руками (без библиотек)

## Migration

- Существующие конфиги без suffix-доменов — поведение не меняется
- `Match(ip)` — unchanged
- `SaveClientConfig` не требует изменений (suffix-домены уже валидны в `ExcludeDomains`/`IncludeDomains`)

## Verification

1. `go test ./src/internal/routing/...` — все тесты проходят
2. `go build ./...` — без ошибок
3. `go vet ./...` — без предупреждений
