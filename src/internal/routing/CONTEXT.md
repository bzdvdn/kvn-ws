# Context — src/internal/routing

## Purpose

Движок правил split-tunnel: решает, куда отправить пакет или DNS-запрос — в туннель (`RouteServer`), напрямую (`RouteDirect`) или «решать дальше» (`RouteNone`). Также разворачивает geoip/geosite/url-источники в плоские списки CIDR и доменов.

## Responsibilities

- Владеет: типами `RouteAction`/`Rule`/`Matcher`, построением `RuleSet` из `config.RoutingCfg`, матчерами (CIDR, exact IP, домены, DNS-tracker), побайтовым `TunRouter` и `Resolver`'ом источников с кэшем.
- Не владеет: чтением YAML-конфига (`src/internal/config`), самим DNS-сервером/кэшем (`src/internal/dns`, `src/internal/dnsproxy`), применением маршрутов в ОС (`src/internal/tun`).

## How it works

- `NewRuleSet(cfg, logger)` → `NewRuleSetWithResolver`: сначала добавляются **exclude**-правила (`RouteDirect`), затем **include** (`RouteServer`), затем фиксируется `defaultAction` из `default_route` (`server`/`direct`, иначе ошибка).
- Внутри `addRules` порядок матчеров: `CIDRMatcher` → `ExactIPMatcher` → домены (exact/suffix-карты + опциональный `DomainMatcher` с DNS-резолвом).
- `Route(ip)`: сперва `dns.Tracker.Lookup` → `MatchDomain` (IP, чей домен уже известен), потом линейный проход по правилам, иначе `defaultAction`.
- `MatchDomain(domain)`: точное совпадение из `exactDomains`, затем суффиксное из `suffixDomains` (правило-суффикс задаётся ведущей точкой, `.example.com`), иначе `RouteNone`.
- `TunRouter.RoutePacket`: определяет версию IP, для IPv4-UDP:53 — перехват DNS (при `dnsOverride` сразу в туннель, иначе `MatchDomain` по qname), для остального — `parseDstIP`/`parseDstIP6` → `RuleSet.Route`; битые/чужие пакеты уходят `sendDirect`.
- `TunRouter` держит `RuleSet` в `atomic.Pointer` — `SetRuleSet` позволяет подменять правила на лету (refresh) без остановки пересылки; `SetTunnelSend` позволяет сессии переопределить отправку (UDP → secondary).
- `Resolver.Resolve()` копирует базовые списки и добавляет развёрнутые `include_sources`/`exclude_sources` (geoip/geosite/cidr/url), дедуплицируя результат; `Refresh()` сначала удаляет кэш-файлы, затем резолвит заново.
- Источники: `geoip.dat`/`geosite.dat` — protobuf v2fly, читаются `geoip.ReadGeoIP`/`ReadGeoSite`; `url` — `file://` или HTTP(S), формат «одна строка — CIDR (есть `/`) или домен», `#` — комментарий. Скачанные БД кэшируются в `<cacheDir>/{geoip,geosite}.dat` с TTL `source_ttl_hours` (по умолчанию 24ч).

## Key files

- `src/internal/routing/routing.go` — `RouteAction`, `Rule`, интерфейс `Matcher`.
- `src/internal/routing/rule_set.go` — `RuleSet`, порядок правил, `Route`/`MatchDomain`.
- `src/internal/routing/router.go` — `TunRouter`, разбор IPv4/IPv6-пакетов, DNS-перехват.
- `src/internal/routing/matcher.go` — CIDR- и exact-IP-матчеры.
- `src/internal/routing/resolver.go` — `Resolver`, `Resolve`/`Refresh`, скачивание и кэш источников.
- `src/internal/routing/domain_matcher.go` + `dns.go` — DNS-резолв доменов и парсер DNS-вопроса.
- `src/internal/routing/geoip/` — protobuf-типы (`*.pb.go`, не править руками) и парсеры `.dat`/URL-списков.

## Public contracts

- `RouteAction`: `RouteNone` (0), `RouteServer` (1), `RouteDirect` (2).
- `NewRuleSet(cfg, logger)`, `NewRuleSetWithResolver(cfg, dnsResolver, logger)`, `(*RuleSet).Route(netip.Addr)`, `(*RuleSet).MatchDomain(string)`, `(*RuleSet).SetTracker(*dns.Tracker)`.
- `NewTunRouter(rs, tunRead, tunWrite, tunnelSend, logger)`, `(*TunRouter).RoutePacket([]byte) error`, `SetRuleSet`, `SetTunnelSend`, `SetDNSOverride`.
- `NewResolver(cfg, cacheDir, logger)`, `Resolve()`, `Refresh()`, `ResolveSources([]config.SourceRule)` (для Web UI refresh).
- `NewCIDRMatcher`, `NewExactIPMatcher`, `NewDomainMatcher`, `ParseDNSQuestion(packet)`.

## Dependencies

- `src/internal/config` (типы `RoutingCfg`, `SourceRule`), `src/internal/dns` (`Resolver`, `Tracker`), `src/internal/routing/geoip`.
- Внешние: `go.uber.org/zap`, `google.golang.org/protobuf`.

## Consumed by

- `src/internal/client` — `RuleSet`/`TunRouter` в v2 TUN/proxy.
- `src/internal/webui` — `Resolver.Refresh` для `/api/config/refresh-sources`.
- `src/internal/webui/handler_config.go` — `Resolver.Refresh()` для `/api/config/refresh-sources`.

## Gotchas

- Порядок важен: exclude-правила всегда раньше include, поэтому «include внутри exclude-диапазона» не победит.
- `MatchDomain` обходит `map[RouteAction]` — при пересечении exact/suffix правил разных действий победитель недетерминирован; задавайте домены так, чтобы они не пересекались.
- Домены без ведущей точки — точное совпадение, с точкой — суффикс (`strings.HasSuffix`), родительский домен сам по себе не матчится.
- `DomainMatcher` (DNS-резолв доменов в IP) подключается только если передан не-nil `dns.Resolver`; `NewRuleSet` его не передаёт.
- `TunRouter` при ошибке разбора пакета или неизвестной версии IP отправляет пакет **напрямую**, а не в туннель.
- DNS-перехват работает только для IPv4/UDP/53; DNS-over-TCP и IPv6-UDP не разбираются.
- `ParseDNSQuestion` не поддерживает compression-указатели (`0xC0`) — имя обрывается на первом указателе.
- `Resolver` только разворачивает `*_sources` в списки; он не резолвит доменные источники в IP.
- Если geoip/geosite-БД недоступны, источники молча пропускаются (warn) — итоговый конфиг остаётся без них, ошибки нет.
- `geoip:private` — встроенные `10/8, 172.16/12, 192.168/16, 100.64/10, fc00::/7`; это не полный набор private-диапазонов.
- `SourceRule` валиден ровно с одним заполненным полем (`geoip|geosite|cidr|url`).
- `Refresh()` удаляет только `geoip.dat`/`geosite.dat`; URL-источники и `file://` перечитываются без кэша.
