# DNS Response Tracker — План реализации

## Цель

Добавить DNS Tracker (`IP → domain`) и DNS proxy в TUN-режим, чтобы доменные правила маршрутизации (exclude_domains/include_domains) применялись к data-пакетам по IP во всех режимах: TUN, proxy (SOCKS5/HTTP), transparent proxy.

Фича opt-in через `routing.dns_cache.enabled: true`. Default `false` — обратная совместимость.

## MVP Slice

Tracker core + RuleSet.Route(ip) lookup + DNS proxy tracking. AC-001, AC-003, AC-005, AC-006, AC-007.

AC-002 (полный TUN сценарий с DNS proxy) и AC-004 (proxy onConn) — следующий инкремент.

## First Validation Path

```bash
cd src/internal/dns && go test -run TestTracker -v   # AC-001, AC-006, AC-007
cd src/internal/routing && go test -run TestRuleSetRoutesWithTracker -v  # AC-003
cd src/internal/dnsproxy && go test -run TestDNSProxyTracks -v  # AC-005
```

## Scope

- `src/internal/dns/tracker.go` — новый модуль
- `src/internal/routing/rule_set.go` — Tracker lookup в `Route(ip)`
- `src/internal/dnsproxy/dnsproxy.go` — отслеживание DNS-ответов для excluded доменов
- `src/internal/routing/router.go` — Tracker в TunRouter (опционально, для фонового резолва)
- `src/internal/bootstrap/client/tun.go` — DNS proxy для TUN при suffix-доменах
- `src/internal/bootstrap/client/proxy.go` — Tracker lookup в `onConn`
- `src/internal/config/client.go` — `DNSCacheCfg` struct
- `src/internal/webui/frontend/src/App.tsx` — UI поля dns_cache

Не меняется: протокол "клиент-сервер", dns.Cache, DomainMatcher (кроме опционального SetTracker).

## Performance Budget

- `none`: Tracker — in-memory map с lazy-delete, типичное число записей < 1000.

## Implementation Surfaces

| Surface | Статус | Роль |
|---------|--------|------|
| `src/internal/dns/tracker.go` | новый | IP→domain кэш, парсинг DNS-ответов, thread-safe |
| `src/internal/routing/rule_set.go` | модификация | `Route(ip)`: lookup в Tracker после CIDR/IP, затем `MatchDomain` |
| `src/internal/dnsproxy/dnsproxy.go` | модификация | `resolveDirect`: парсинг ответа → `tracker.TrackResponse` |
| `src/internal/routing/router.go` | модификация | Передача Tracker, при RouteDirect — `tracker.Track(domain, ips)` |
| `src/internal/bootstrap/client/tun.go` | модификация | Создание Tracker, DNS proxy при suffix-доменах |
| `src/internal/bootstrap/client/proxy.go` | модификация | Tracker lookup в `onConn` для IP-адресов |
| `src/internal/config/client.go` | модификация | `DNSCacheCfg` struct, default-ы в `NewFromConfig` |
| `src/internal/webui/frontend/src/App.tsx` | модификация | UI: чекбокс + ttl input |

## Bootstrapping Surfaces

`src/internal/dns/tracker.go` — первая файл, который нужно создать. Всё остальное — модификация существующих файлов.

## Влияние на архитектуру

- Локальное: новый модуль в пакете `dns`, минимальные изменения в routing/dnsproxy/bootstrap.
- Интеграции: DNS proxy теперь может быть запущен не только в transparent proxy, но и в TUN-режиме.
- Compatibility: `dns_cache.enabled: false` (default) — поведение не меняется. Никаких миграций не требуется.

## Acceptance Approach

- AC-001: unit-тест `TestTrackerTrackAndLookup` в `src/internal/dns/tracker_test.go`
- AC-002: интеграционный тест: запуск TUN + DNS proxy, проверка логов `"matched rule" action=2`
- AC-003: unit-тест `TestRuleSetRoutesWithTrackerLookup` в `routing/rule_set_test.go`
- AC-004: unit-тест с mock Tracker в proxy-коллбеке
- AC-005: unit-тест `TestDNSProxyTracksExcludedDomains` в `dnsproxy/`
- AC-006: unit-тест с time.Sleep или clock mock
- AC-007: `go test -race ./src/internal/dns/`

## Данные и контракты

Data model меняется минимально:
- Добавляется `DNSCacheCfg` (Go struct) — не persisted, только runtime config.
- Tracker — in-memory runtime state, не сохраняется.
- См. `data-model.md`.

Контракты API (фронтенд ↔ бэкенд) не меняются — поле `dns_cache` передаётся транзитом через существующие JSON-структуры.

## Стратегия реализации

### DEC-001 Tracker как отдельный модуль, а не расширение dns.Cache

- Why: `dns.Cache` — прямой кэш `domain → IP` с TTL для resolver'а. Tracker — обратный кэш `IP → domain` с парсингом wire-формата DNS-ответов. Разная семантика, разные методы доступа. Смешивать их — усложнять существующий, стабильный код.
- Tradeoff: два кэша вместо одного. Дублирование TTL-логики (lazy-delete). Компенсация: Tracker ~80 строк, копирование тривиально.
- Affects: `src/internal/dns/tracker.go` (новый), `src/internal/dns/cache.go` (не меняется)
- Validation: AC-001, AC-007

### DEC-002 DNS proxy в TUN-режиме при suffix-доменах

- Why: DNS не проходит через TUN (systemd-resolved). `DomainMatcher.Match(ip)` не умеет резолвить суффиксы (`.ru`). Единственный способ перехватить DNS до systemd-resolved — запустить DNS proxy + override resolv.conf.
- Tradeoff: TUN-режим получает зависимость от DNS proxy. При остановке клиента надо восстанавливать resolv.conf (уже реализовано в transparent proxy). Дополнительная латентность ~1ms на DNS-запрос.
- Affects: `src/internal/bootstrap/client/tun.go`, `src/internal/dnsproxy/dnsproxy.go`
- Validation: AC-002

### DEC-003 Tracker lookup в RuleSet.Route(ip) как fallback после CIDR/IP

- Why: существующий `Route(ip)` проверяет CIDR → ExactIP → DomainMatcher. Tracker — ещё один источник маппинга, который должен срабатывать, если ни одно из существующих правил не совпало. Это минимизирует изменения в существующей логике.
- Tradeoff: Tracker не участвует в порядке правил (exclude → include). Если домен есть и в exclude, и в include — выигрывает exclude (благодаря порядку в `addRules`). Tracker как fallback не нарушает этого порядка.
- Affects: `src/internal/routing/rule_set.go`
- Validation: AC-003

### DEC-004 TUN DNS proxy: kernel exclude routes + cleanup + corporate DNS support

- Why: base TUN DNS proxy (DEC-002) не добавлял `/32` routes для resolved IP — пакеты шли через TUN, TunRouter → RouteDirect → sendDirect, что работало, но неоптимально. Также не чистил exclude routes при disconnect (stale routes ломали openfortivpn). `SetRouteFunc` был missing — все DNS шли TCP upstream. `resolveDirect` пробовал только первый resolver.
- Tradeoff: CleanupExcludeRoutes() требует хранения списка добавленных маршрутов в tunDevice. Добавляет ~100 строк. Компенсация: полная очистка при disconnect, поддержка corporate VPN (openfortivpn) через ppp0.
- Affects: `src/internal/tun/tun.go`, `src/internal/tun/tun_common.go`, `src/internal/tun/tun_stub.go`, `src/internal/dnsproxy/dnsproxy.go`, `src/internal/bootstrap/client/tun.go`
- Validation: AC-002, AC-007, ручное тестирование с openfortivpn

### DEC-005 resolveDirect multi-resolver fallback

- Why: resolveDirect пробовал только первый resolver из списка. Если он не отвечал или возвращал NXDOMAIN, fallback на второй не происходил. С корпоративным DNS (10.x.x.x) для .ru доменов это приводило к failure.
- Tradeoff: увеличение latency в худшем случае (таймаут на первом → попытка второго). Компенсация: более надёжное DNS-резолвление для mixed environments.
- Affects: `src/internal/dnsproxy/dnsproxy.go`
- Validation: ручное тестирование с openfortivpn + .ru + .we-on.com

### DEC-006 Private/loopback IP skip в kernel exclude routes

- Why: `/32` exclude route via phy для corporate IP (10.x.x.x) behind ppp0 перебивал маршрут openfortivpn. Private и loopback IP не должны получать kernel route через физический интерфейс.
- Tradeoff: private IPs не получают exclude route — пакеты к corporate IP идут через ppp0 (если есть маршрут). Если ppp0 нет — через TUN.
- Affects: `src/internal/bootstrap/client/tun.go`
- Validation: ручное тестирование с openfortivpn

## Incremental Delivery

### MVP (AC-001, AC-003, AC-005, AC-006, AC-007)

- Tracker core (`tracker.go`): TrackResponse, Lookup, Track (domain+ips напрямую), lazy-delete
- RuleSet.Route(ip): lookup Tracker → MatchDomain
- DNS proxy: при `routeDirect` — парсинг ответа, вызов Tracker
- config: DNSCacheCfg struct

### Итерация 2 (AC-002, AC-004)

- TUN: запуск DNS proxy при suffix-доменах + dns_cache.enabled
- Proxy onConn: Tracker lookup для IP-адресов
- UI: поля dns_cache в форме Routing

## Порядок реализации

1. `tracker.go` + тесты — база, от которой всё зависит
2. `rule_set.go` — Tracker lookup в Route(ip)
3. `dnsproxy/dnsproxy.go` — отслеживание DNS-ответов
4. `config/client.go` — DNSCacheCfg struct, default-ы
5. `tun.go` — DNS proxy при suffix-доменах
6. `proxy.go` — Tracker lookup в onConn
7. `App.tsx` — UI

Параллельно: 2+3 (независимы после tracker.go)

## Риски

- Риск: resolv.conf override не восстанавливается при краше. Mitigation: reuse существующий механизм `dnsproxy.BackupResolvConf` + `Restore` из transparent proxy.
- Риск: DNS proxy добавляет latency. Mitigation: Tracker кэширует резолв, повторные запросы не ходят в upstream.
- Риск: Tracker не успевает заполниться до первого data-пакета (race DNS vs TCP). Mitigation: TCP SYN обычно приходит через >1ms после DNS-ответа; парсинг DNS-ответа ~0.01ms. Запас достаточен.
- Риск: Stale kernel exclude routes после disconnect ломают openfortivpn. Mitigation: `CleanupExcludeRoutes()` в defer `runSession`. 

## Rollout и compatibility

Специальных rollout-действий не требуется. Фича под флагом `dns_cache.enabled: false` (default). Включение — осознанное действие пользователя.

## Проверка

| Шаг | Команда | AC/DEC |
|-----|---------|--------|
| Tracker unit | `go test -race ./src/internal/dns/ -run TestTracker` | AC-001, AC-006, AC-007 |
| RuleSet unit | `go test -race ./src/internal/routing/ -run TestRuleSetRoutesWithTracker` | AC-003 |
| DNS proxy unit | `go test -race ./src/internal/dnsproxy/ -run TestDNSProxyTracks` | AC-005 |
| Integration | `go test -race ./src/internal/bootstrap/client/ -run TestTunDNSProxy` | AC-002 |
| Proxy integration | `go test -race ./src/internal/proxy/ -run TestProxyOnConnTracker` | AC-004 |
| Race all | `go test -race ./...` | AC-007 |
| Manual | `curl ozon.ru` с `dns_cache.enabled: true` + логи | SC-002 |
| Manual (corporate VPN) | openfortivpn + KVN, `ping -c 3 <corp-host>` через ppp0 | DEC-004, DEC-005, DEC-006 |
| Disconnect cleanup | `systemctl stop kvn` → `ip route show` без stale exclude routes | DEC-004 |

## Соответствие конституции

Нет конфликтов. Trace-маркеры `@sk-task` на новых и изменённых объявлениях обязательны.
