# DNS Response Tracker — Задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/dns/tracker.go` (новый) | T1.1, T3.1 |
| `src/internal/dns/tracker_test.go` (новый) | T1.1 |
| `src/internal/config/client.go` | T1.2 |
| `src/internal/routing/rule_set.go` | T2.1 |
| `src/internal/routing/rule_set_test.go` | T2.1 |
| `src/internal/dnsproxy/dnsproxy.go` | T2.2 |
| `src/internal/dnsproxy/dnsproxy_test.go` | T2.3 |
| `src/internal/routing/router.go` | T3.2 |
| `src/internal/bootstrap/client/tun.go` | T3.2 |
| `src/internal/bootstrap/client/proxy.go` | T3.3 |
| `src/internal/tun/tun.go` | T3.5 |
| `src/internal/tun/tun_common.go` | T3.5 |
| `src/internal/tun/tun_stub.go` | T3.5 |
| `src/internal/webui/frontend/src/App.tsx` | T3.4 |
| `src/internal/dns/dns_test.go` | T4.1 |

## Implementation Context

- Цель MVP: Tracker core (`IP → domain` cache) + RuleSet.Route(ip) lookup + DNS proxy tracking.
- Границы приемки: AC-001, AC-003, AC-005, AC-006, AC-007 (MVP); AC-002, AC-004 (итерация 2).
- Инварианты:
  - Tracker: in-memory `map[netip.Addr]trackedEntry` с lazy-delete по TTL.
  - `TrackResponse(qname, rawResponse)` парсит wire-формат DNS (A/AAAA записи).
  - TTL записи = min(TTL из DNS ответа, dns_cache.ttl из конфига).
  - thread-safe через `sync.RWMutex`.
- Контракты:
  - `RuleSet` получает `SetTracker(t *Tracker)` — lookup в `Route(ip)` перед `defaultAction`.
  - `dnsproxy.Server` получает `SetTracker(t *Tracker)` — вызов `TrackResponse` после `resolveDirect`.
- Proof signals:
  - `go test -race ./src/internal/dns/` проходит AC-001, AC-006, AC-007.
  - `go test -race ./src/internal/routing/ -run TestRuleSetRoutesWithTracker` проходит.
  - `go test -race ./src/internal/dnsproxy/ -run TestDNSProxyTracks` проходит (после T2.3).
  - `go build ./...` без ошибок.
- Вне scope: персистентность Tracker, CNAME-нормализация, изменения DomainMatcher.
- References: DEC-001, DEC-002, DEC-003, DM (no-change).

## Фаза 1: Основа

Цель: Tracker core + DNSCacheCfg конфиг.

- [x] T1.1 Создать `src/internal/dns/tracker.go` и `tracker_test.go`:
  - `type Tracker` с `mu sync.RWMutex`, `entries map[netip.Addr]trackedEntry`, `ttl time.Duration`.
  - `trackedEntry` c `domain string` и `deadline time.Time`.
  - `NewTracker(ttl time.Duration) *Tracker`.
  - `Track(domain string, ips []netip.Addr)` — сохранить маппинг IP→domain.
  - `TrackResponse(qname string, rawResponse []byte)` — распарсить DNS-ответ (A/AAAA), вызвать `Track`.
  - `Lookup(ip netip.Addr) (string, bool)` — lazy-delete просроченных записей.
  - Тесты: `TestTrackerTrackAndLookup` (AC-001), `TestTrackerTTL` (AC-006), `TestTrackerRace` (AC-007).
  - Touches: `src/internal/dns/tracker.go`, `src/internal/dns/tracker_test.go`

- [x] T1.2 Добавить `DNSCacheCfg` в `RoutingCfg`:
  - `type DNSCacheCfg struct { Enabled bool; TTL int }`.
  - Поле `DNSCache *DNSCacheCfg` в `RoutingCfg` с тегами `json`/`mapstructure`.
  - Default-ы в `NewFromConfig`: `{Enabled: false, TTL: 60}`.
  - Touches: `src/internal/config/client.go`

## Фаза 2: MVP Slice

Цель: Tracker интегрирован в RuleSet.Route(ip) и DNS proxy.

- [x] T2.1 Добавить `SetTracker(t *Tracker)` в `RuleSet`. В `Route(ip)` перед `defaultAction`: lookup IP в Tracker → `MatchDomain(domain)` → вернуть action.
  - Тест: `TestRuleSetRoutesWithTrackerLookup` (AC-003).
  - Touches: `src/internal/routing/rule_set.go`, `src/internal/routing/rule_set_test.go`

- [x] T2.2 Добавить `SetTracker(t *Tracker)` в `dnsproxy.Server`. В `resolveDirect` после успешного ответа: парсить ответ через `tracker.TrackResponse`.
  - Touches: `src/internal/dnsproxy/dnsproxy.go`

- [x] T2.3 Написать unit-тест `TestDNSProxyTracksExcludedDomains` для AC-005:
  - Создать `dnsproxy.Server` с mock `routeDirect`, `tracker` и upstream.
  - Подать DNS-запрос для excluded домена.
  - Проверить, что `tracker.Lookup` возвращает домен после `resolveDirect`.
  - Проверить, что для non-excluded домена трекер не заполняется (опционально).
  - Тег: `@sk-test dns-response-tracker#T2.3 (AC-005) TestDNSProxyTracksExcludedDomains`.
  - Touches: `src/internal/dnsproxy/dnsproxy_test.go`

## Фаза 3: Основная реализация

Цель: TUN DNS proxy + proxy onConn + UI.

- [x] T3.1 Добавить `Track(domain string, ips []netip.Addr)` shortcut в `Tracker` (уже в T1.1, проверить готовность).
  - Touches: `src/internal/dns/tracker.go`

- [x] T3.2 В `tun.go` (`runSession`): при `cfg.Routing.DNSCache.Enabled && hasSuffixDomains`:
  - Создать `Tracker`.
  - Запустить `dnsproxy.Server` (как в `proxy.go` transparent), передать Tracker.
  - Override resolv.conf через `dnsproxy.BackupResolvConf` / `OverrideResolvConf`.
  - Передать Tracker в `TunRouter` через `SetTracker`.
  - В `TunRouter.routeByRule`: Tracker lookup для IP (уже в `Route` через `RuleSet.Route`).
  - Восстановление resolv.conf при остановке.
  - Touches: `src/internal/bootstrap/client/tun.go`, `src/internal/routing/router.go`

- [x] T3.3 В `proxy.go` (`onConn`): Tracker init + hostname tracking при `dns_cache.enabled`:
  - Создать `dnsTracker`, set на `routeSet`.
  - После `LookupHost` — `dnsTracker.Track(host, ips)`.
  - Touches: `src/internal/bootstrap/client/proxy.go`

- [x] T3.4 В `App.tsx`:
  - TypeScript: добавить `dns_cache?: { enabled?: boolean; ttl?: number }` в `routing` интерфейс.
  - JSX: `Checkbox` для `dns_cache.enabled` + number input для `dns_cache.ttl` в секции Routing.
  - Touches: `src/internal/webui/frontend/src/App.tsx`

- [x] T3.5 TUN DNS proxy production hardening:
  - `CleanupExcludeRoutes()` в `tunDevice` — удаление kernel exclude routes при disconnect (AC-007).
  - `SetRouteFunc` в TUN mode — DNS proxy вызывал `routeDirect` для excluded доменов (было missing — все DNS шли TCP upstream).
  - `SetDirectRouteFunc` hook — add exclude `/32` route для resolved IP excluded домена (TUN bypass).
  - Loopback resolver filter — `127.0.0.53` (systemd-resolved) отфильтрован, fallback на upstream DNS.
  - Private IP resolver filter — corporate DNS (10.x.x.x) behind ppp0 не получает exclude route, пакеты идут через ppp0.
  - `resolveDirect` multi-resolver — теперь пробует ВСЕ резолверы, не только первый.
  - `directRouteFn` private skip — `/32` exclude route не добавляется для private/loopback IP.
  - Touches: `src/internal/dnsproxy/dnsproxy.go`, `src/internal/bootstrap/client/tun.go`,
    `src/internal/tun/tun.go`, `src/internal/tun/tun_common.go`, `src/internal/tun/tun_stub.go`

## Фаза 4: Проверка

Цель: полное тестовое покрытие + финальная проверка.

- [x] T4.1 Добавить оставшиеся тесты:
  - `TestTunRouterRoutesWithTracker` для AC-002 (mock DNS proxy + Tracker).
  - `TestProxyOnConnTracker` для AC-004 (mock Tracker в proxy коллбеке).
  - `go test -race ./...` — нет data race.
  - Touches: `src/internal/dns/dns_test.go`, `src/internal/routing/router_test.go`, `src/internal/proxy/listener_test.go`

## Покрытие критериев приемки

- AC-001 -> T1.1
- AC-002 -> T3.2, T4.1
- AC-003 -> T2.1
- AC-004 -> T3.3, T4.1
- AC-005 -> T2.2, T2.3
- AC-006 -> T1.1
- AC-007 -> T1.1

## Заметки

- T1.1 и T1.2 независимы — можно параллелить.
- T2.1 и T2.2 независимы после T1.1 — можно параллелить.
- T3.2, T3.3, T3.4 независимы после T2.1/T2.2 — можно параллелить.
