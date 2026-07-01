# DNS Response Tracker — маршрутизация данных по доменным правилам через отслеживание DNS-ответов

## Scope Snapshot

- In scope: механизм, который отслеживает DNS-ответы для excluded/included доменов и применяет доменные правила маршрутизации к последующим data-пакетам по IP-адресу во всех режимах (TUN, proxy, transparent proxy).
- Out of scope: рефакторинг существующего DNS-кэша, изменение протокола "клиент-сервер", переработка системы правил.

## Цель

Пользователь, настроивший `exclude_domains: [.ru]` (или `include_domains`), ожидает, что трафик на `ozon.ru` пойдёт напрямую (direct), а не через туннель (server). Сейчас правило срабатывает только на этапе DNS-запроса (по имени домена), но теряется при маршрутизации data-пакетов по IP. Фича добавляет DNS Response Tracker — кэш `IP → domain`, который наполняется при DNS-резолве excluded/included доменов и используется при маршрутизации всех последующих пакетов.

Признак успеха: для `exclude_domains: [.ru]` curl ozon.ru и curl mail.ru получают `RouteDirect` во всех режимах, включая TUN и transparent proxy.

## Основной сценарий

Сценарий разделяется на два трека, так как TUN и proxy режимы имеют разные механики DNS.

### Трек A: Proxy / Transparent Proxy

1. Клиент запущен с `exclude_domains: [.ru]`.
2. Приложение резолвит `ozon.ru` — в proxy режиме через SOCKS5/HTTP (шлёт домен), в transparent proxy через DNS proxy (iptables redirect на порт 53).
3. DNS-запрос/хост перехватывается: `MatchDomain("ozon.ru")` → `RouteDirect`.
4. DNS-ответ (локальный резолв) парсится: IP-адреса `ozon.ru` сохраняются в `Tracker` с маппингом `IP → "ozon.ru"` и TTL.
5. Приложение открывает TCP-соединение на полученный IP (`95.163.249.123:443`).
6. `onConn` получает `dst`:
   - **Proxy**: `"ozon.ru:443"` → `MatchDomain` срабатывает сразу → `RouteDirect`.
   - **Transparent proxy**: `"95.163.249.123:443"` → lookup IP в Tracker → найден домен → `MatchDomain("ozon.ru")` → `RouteDirect`.
7. Соединение идёт напрямую.

### Трек B: TUN

**Проблема**: DNS-запросы НЕ проходят через TUN (systemd-resolved шлёт их напрямую через физический интерфейс). `TunRouter.routeDNSQuery` никогда не видит DNS-трафика приложений. Суффиксные домены (`.ru`) невозможно зарезолвить через `DomainMatcher.Match(ip)`, т.к. `DomainMatcher` хранит суффиксы только для `MatchDomain(domainName)`, а имя домена в data-пакете отсутствует.

**Решение**: в TUN-режиме запускается локальный DNS proxy (как в transparent proxy), который перехватывает ВСЕ DNS-запросы через override `/etc/resolv.conf`:

1. Клиент запущен с `exclude_domains: [.ru]`, TUN-режим.
2. При старте запускается DNS proxy на `127.0.0.54:53`, `/etc/resolv.conf` заменяется на `nameserver 127.0.0.54`.
3. Приложение резолвит `ozon.ru` — DNS-запрос идёт на DNS proxy.
4. DNS proxy извлекает QNAME `"ozon.ru"`, вызывает `routeDirect("ozon.ru")` = `MatchDomain("ozon.ru")` → `RouteDirect`.
5. DNS proxy резолвит `ozon.ru` через оригинальные nameserver-ы (с фильтром loopback, с сохранением private для corporate DNS behind ppp0).
6. DNS-ответ парсится: IP `95.163.249.123` сохраняется в Tracker с маппингом `IP → "ozon.ru"` и TTL.
7. DNS proxy добавляет `/32` kernel exclude route для resolved IP (кроме private/loopback) — последующие пакеты идут напрямую через физический интерфейс, минуя TUN.
8. DNS-ответ возвращается приложению.
9. Приложение открывает TCP-соединение на `95.163.249.123:443` — пакет идёт через TUN (default route).
10. `routeByRule(95.163.249.123)` → `Route(ip)`:
    - Проверка CIDR/IP правил — нет совпадения.
    - Lookup IP в Tracker → найден домен `"ozon.ru"` → `MatchDomain("ozon.ru")` → `RouteDirect`.
11. Пакет получает `RouteDirect`, отправляется напрямую (kernel `/32` route уже есть).
12. По истечении TTL запись в Tracker удаляется; при следующем DNS-запросе маппинг обновляется.

**TUN DNS proxy enhancements** (T3.5): `CleanupExcludeRoutes()` удаляет все kernel route при disconnect; `SetRouteFunc` добавлен в TUN mode (был missing); `SetDirectRouteFunc` добавляет `/32` routes для resolved IP; loopback resolver filter (systemd-resolved) + upstream fallback; private IP resolver filter не добавляет exclude routes через phy (corporate DNS behind ppp0); `resolveDirect` пробует все резолверы последовательно; `directRouteFn` пропускает private/loopback IP.

## User Stories

- P1: Пользователь TUN-режима: exclude_domains работают для data-трафика (не только DNS).
- P1: Пользователь transparent proxy: exclude_domains работают для data-трафика.
- P2: Пользователь SOCKS5/HTTP proxy: exclude_domains работают даже если клиент сам резолвит DNS и шлёт IP.

## MVP Slice

Tracker + интеграция в TUN router + интеграция в proxy mode onConn. AC-001, AC-002, AC-003.

## First Deployable Outcome

После implementation pass: бинарник cliente собирается, тест `curl ozon.ru` в TUN- или proxy-режиме с `exclude_domains: [.ru]` показывает `RouteDirect` для data-пакетов (проверка по логам `"domain matched"`/`"matched rule"` с `action=2`).

## Scope

- Модуль `src/internal/dns/tracker.go` — IP→domain кэш с TTL, парсинг DNS-ответов (A/AAAA).
- Модификация `src/internal/routing/rule_set.go` — интеграция Tracker в `Route(ip)`: при отсутствии совпадения по CIDR/IP — lookup IP в Tracker, затем `MatchDomain`.
- Модификация `src/internal/routing/router.go` — передача Tracker в TunRouter, при `routeDNSQuery` с `RouteDirect` — фоновый резолв домена.
- Модификация `src/internal/bootstrap/client/tun.go` — запуск DNS proxy при наличии suffix-доменов (exclude_domains/include_domains с `.`), создание Tracker, передача в TunRouter, override resolv.conf.
- Модификация `src/internal/bootstrap/client/proxy.go` — использование Tracker в `onConn` для IP→domain lookup.
- Модификация `src/internal/dnsproxy/dnsproxy.go` — при локальном резолве excluded домена — сохранение маппинга в Tracker.
- Модификация `src/internal/routing/domain_matcher.go` — опционально: `SetTracker` для дополнения resolved-кэша.

## Контекст

- Существующий `dns.Cache` — кэш `domain → IP`, используется `DefaultResolver` и `DomainMatcher`.
- `DomainMatcher` создаётся только при наличии `dns.Resolver` в `NewRuleSetWithResolver`. В TUN и proxy mode `NewRuleSet` вызывается без resolver'а (значит, `DomainMatcher` не создаётся).
- Существующий `dns.Cache` не поддерживает reverse lookup (IP → domain).
- В transparent proxy `getOriginalDst` возвращает IP, а не домен.
- DNS TTL в ответах может быть коротким (30-300с), Tracker должен уважать TTL из ответа.
- **ВАЖНО**: В TUN-режиме DNS-запросы НЕ проходят через TUN-интерфейс. systemd-resolved (или другой локальный резолвер) шлёт запросы напрямую через физический интерфейс. `TunRouter.routeDNSQuery` никогда не вызывается для DNS-трафика приложений.
- `DomainMatcher.Match(ip)` работает ТОЛЬКО для точных доменов (без префикса `.`). Суффиксные домены (`.ru`) хранятся в `suffixes` и используются только в `MatchDomain(domainName)`, который требует имя домена. Для TUN-режима с суффиксами единственный способ — перехватить DNS до systemd-resolved через локальный DNS proxy.

## Зависимости

- Стабильность: `dnsproxy`, `routing/router.go`, `routing/rule_set.go`, `proxy/listener.go` — интерфейсы не меняются, только расширяются.
- Внешние: стандартная библиотека `net`, `net/netip`.

## Требования

- **RQ-001** Система ДОЛЖНА извлекать IP-адреса (A и AAAA записи) из DNS-ответов для excluded/included доменов.
- **RQ-002** Система ДОЛЖНА хранить маппинг `IP → domain` с TTL, равным минимальному TTL среди записей в DNS-ответе.
- **RQ-003** Система ДОЛЖНА при маршрутизации data-пакета по IP выполнять reverse lookup в Tracker и, при нахождении домена, применять к нему `MatchDomain`.
- **RQ-004** В TUN-режиме при наличии суффиксных доменов (с префиксом `.` в `include_domains`/`exclude_domains`) система ДОЛЖНА запускать локальный DNS proxy с override `/etc/resolv.conf`, перехватывать DNS-запросы и сохранять маппинг `IP → domain` в Tracker.
- **RQ-005** В transparent proxy режиме при локальном резолве DNS (в `dnsproxy`) система ДОЛЖНА парсить ответ и сохранять маппинг в Tracker.
- **RQ-006** В proxy режиме при получении `onConn` с IP-адресом система ДОЛЖНА выполнять lookup IP в Tracker, и при нахождении домена — применять `MatchDomain`.
- **RQ-007** Tracker ДОЛЖЕН быть thread-safe (sync.RWMutex).
- **RQ-008** При штатной остановке клиента Tracker НЕ требует явного shutdown (память освобождается GC).

## Вне scope

- Изменение протокола "клиент-сервер" (frame types).
- Рефакторинг `dns.Cache` или замена на Tracker.
- Поддерак CNAME/MX/NS записей (только A и AAAA).
- Персистентность маппингов (только in-memory).
- Автоматическое удаление просроченных записей через горутину (только lazy-delete на Get).

## Критерии приемки

### AC-001 Tracker сохраняет IP→domain из DNS-ответа

- Почему это важно: без парсинга DNS-ответа система не получит актуальные IP-адреса домена.
- **Given** DNS-ответ для `ozon.ru` с A-записью `95.163.249.123` и TTL=120
- **When** `tracker.TrackResponse("ozon.ru", response)` вызван
- **Then** `tracker.Lookup(netip.MustParseAddr("95.163.249.123"))` возвращает `"ozon.ru", true`
- **And** `tracker.Lookup(netip.MustParseAddr("1.1.1.1"))` возвращает `"", false`
- Evidence: unit-тест `TestTrackerTrackAndLookup`

### AC-002 TUN-режим: DNS proxy + Tracker для маршрутизации data-пакетов с суффиксными доменами

- Почему это важно: DNS идёт через systemd-resolved, не через TUN. Суффиксы (`.ru`) не резолвятся через `DomainMatcher.Match(ip)`. Нужен DNS proxy для перехвата DNS и Tracker для IP→domain lookup.
- **Given** TUN-режим с `exclude_domains: [.ru]` и запущенным DNS proxy
- **And** DNS proxy перехватил запрос `ozon.ru`, зарезолвил локально, сохранил IP `95.163.249.123 → "ozon.ru"` в Tracker
- **When** `routeByRule(95.163.249.123, packet)` вызван
- **Then** `Route(ip)` делает lookup в Tracker → находит домен → `MatchDomain("ozon.ru")` → возвращает `RouteDirect`
- Evidence: юнит-тест `TestTunRouterRoutesWithTracker` проверяет, что data-пакет на IP excluded домена получает `RouteDirect` через Tracker

### AC-003 RuleSet.Route(ip) использует Tracker как источник IP→domain маппинга

- Почему это важно: без Tracker в `Route(ip)` data-пакеты, идущие по IP, никогда не узнают исходный домен.
- **Given** Tracker содержит запись `95.163.249.123 → "ozon.ru"`, и `RuleSet` имеет `exclude_domains: [.ru]`
- **When** `RuleSet.Route(95.163.249.123)` вызван
- **And** ни одно CIDR/IP правило не совпало
- **Then** `Route(ip)` делает lookup IP в Tracker, находит домен `"ozon.ru"`, вызывает `MatchDomain("ozon.ru")` → `RouteDirect`
- Evidence: юнит-тест `TestRuleSetRoutesWithTrackerLookup`

### AC-004 Proxy onConn использует Tracker для IP→domain lookup

- Почему это важно: transparent proxy получает IP, а не домен; без tracker'а exclude не срабатывает.
- **Given** Tracker содержит запись `95.163.249.123 → "ozon.ru"`, и `routeSet` имеет `exclude_domains: [.ru]`
- **When** `onConn(client, "95.163.249.123:443")` вызван
- **Then** `MatchDomain("ozon.ru")` возвращает `RouteDirect`
- Evidence: юнит-тест с mock Tracker и проверкой действия в proxy-коллбеке

### AC-005 DNS proxy сохраняет маппинг при локальном резолве

- Почему это важно: transparent proxy использует DNS proxy для резолва excluded доменов; маппинг должен сохраняться.
- **Given** DNS proxy с `routeDirect` для `.ru`, и Tracker
- **When** DNS-запрос для `ozon.ru` приходит в DNS proxy и resolved locally
- **Then** Tracker содержит запись `IP(ozon.ru) → "ozon.ru"` после ответа
- Evidence: unit-тест `TestDNSProxyTracksExcludedDomains`

### AC-006 TTL из DNS-ответа соблюдается

- Почему это важно: использовать TTL из ответа важно для корректного обновления IP при изменении DNS.
- **Given** DNS-ответ с A-записью `95.163.249.123` и TTL=60
- **When** `tracker.TrackResponse("ozon.ru", response)` вызван
- **And** через 61 секунду вызван `tracker.Lookup(95.163.249.123)`
- **Then** результат — `"", false` (запись просрочена)
- Evidence: unit-тест с mock-таймером или time.Sleep (в горутине)

### AC-007 Thread-safety Tracker

- Почему это важно: DNS proxy и маршрутизатор работают из разных горутин.
- **Given** Tracker без записей
- **When** 10 горутин одновременно вызывают `Track` и `Lookup`
- **Then** нет data race (проверка `go test -race`)
- Evidence: `go test -race ./src/internal/dns/` проходит

## Допущения

- DNS-ответы приходят в стандартном wire-формате (RFC 1035).
- В TUN-режиме с суффиксными доменами (`.ru`) DNS-запросы перехватываются через локальный DNS proxy + override resolv.conf.
- DNS proxy запускается на `127.0.0.54:53` (по умолчанию), upstream — `1.1.1.1:53`.
- Tracker наполняется как из DNS proxy (TUN), так и из `resolveDirect` (transparent proxy).
- При расхождении IP (CDN, anycast, GEO-балансировка) допускаем, что пакет может временно уйти в туннель; следующий DNS-запрос скорректирует маппинг.
- Tracker не требует персистентности — после переподключения DNS-запросы будут сделаны заново.

## Критерии успеха

- SC-001: Ни один существующий тест не падает (go test -race ./...).
- SC-002: Для TUN mode с `exclude_domains: [.ru]` traceroute/ping до `ozon.ru` показывает прямой путь (не через туннель).

## Краевые случаи

- Домен резолвится в несколько IP (A + AAAA) — все должны быть сохранены.
- DNS-ответ не содержит A/AAAA записей (CNAME, NXDOMAIN) — Tracker не сохраняет запись.
- Один IP принадлежит нескольким доменам (shared hosting) — Tracker хранит только последний домен (last-write-wins).
- Tracker пуст при старте — lookup возвращает false, маршрутизация по умолчанию.

## Конфигурация

Добавляется секция `dns_cache` в `RoutingCfg`:

```yaml
routing:
  default_route: server
  exclude_domains:
    - .ru
  dns_cache:
    enabled: true   # master switch, default false
    ttl: 60         # TTL для закэшированных IP→domain записей, default 60
```

Поля:
- `dns_cache.enabled` (bool, default `false`) — включает DNS Tracker:
  - **TUN mode**: запускает DNS proxy, override `/etc/resolv.conf`, Tracker привязывается к TunRouter
  - **Proxy/transparent**: Tracker привязывается к DNS proxy и `onConn`
- `dns_cache.ttl` (int, seconds, default `60`) — время жизни записи в Tracker. Если в DNS-ответе TTL меньше — используется TTL из ответа.

`false` по умолчанию — обратная совместимость, поведение не меняется.

Default-ы устанавливаются в `NewFromConfig`:
```go
if cfg.Routing != nil && cfg.Routing.DNSCache == nil {
    cfg.Routing.DNSCache = &DNSCacheCfg{Enabled: false, TTL: 60}
}
```

kvn-web: отдельной обработки не требуется. `WebUIConfig` встраивает `ClientConfig`, YAML marshal/unmarshal сериализует поле автоматически. `mergeConfig` заменяет `Routing` целиком при не-nil серверном конфиге (строка 165-167) — DNSCache пробрасывается транзитом.

**UI фронтенд** (`App.tsx`): добавить в форму Routing:

1. TypeScript-тип `routing` (строки 35-50): добавить `dns_cache?: { enabled?: boolean; ttl?: number }`
2. JSX в секции Routing (после `exclude_domains`):
```tsx
<Checkbox checked={serverConfig.routing?.dns_cache?.enabled ?? false}
  onChange={(v) => nestServer2("routing", "dns_cache", "enabled", v)}
  label="DNS Cache (track IP→domain for routed domains)" />
<label style={lbl}>DNS Cache TTL (s)
  <input type="number" style={inp} value={serverConfig.routing?.dns_cache?.ttl ?? 60}
    onChange={(e) => nestServer2("routing", "dns_cache", "ttl", parseInt(e.target.value) || 60)} />
</label>
```

JSON-сериализация (`saveAll`) подхватит поле автоматически через `JSON.stringify`. Go-бэкенд десериализует в `DNSCacheCfg`. Никаких изменений в API-обработчиках не нужно.

```go
// RoutingCfg update
type RoutingCfg struct {
    // ... существующие поля ...
    DNSCache *DNSCacheCfg `json:"dns_cache,omitempty" mapstructure:"dns_cache,omitempty"`
}

type DNSCacheCfg struct {
    Enabled bool `json:"enabled" mapstructure:"enabled"`
    TTL     int  `json:"ttl,omitempty" mapstructure:"ttl,omitempty"`
}
```

kvn-web: `RoutingCfg` уже embedded в `ServerEntry.ClientConfig`, изменения в JSON-схеме API не требуются — новое поле будет передаваться транзитом.

## Открытые вопросы

- Нужна ли очистка просроченных записей по горутине (background janitor) или достаточно lazy-delete на Lookup? — Решено: достаточно lazy-delete на Lookup для MVP. Janitor добавлять не требуется, т.к. записей ожидается не более нескольких тысяч.
- Как поступать с CNAME-нормализацией? — Решено: TrackResponse принимает `qname` как есть (CNAME не разрешаем), MatchDomain сработает по суффиксу.
