# Routing & Split Tunnel — Модель данных

## Scope

- Связанные `AC-*`: AC-001–AC-010
- Связанные `DEC-*`: DEC-001–DEC-005
- Статус: `changed`
- Модель расширяется: новый конфиг-блок RoutingCfg, in-memory RuleSet, DNSCache. Персистентность не добавляется.

## Сущности

### DM-001: RoutingCfg (конфигурация)

- Назначение: декларативное описание правил маршрутизации в YAML.
- Источник истины: `configs/client.yaml`.
- Инварианты:
  - `DefaultRoute` — одно из `server` | `direct`.
  - CIDR-строки валидируются при загрузке (`netip.ParsePrefix`).
  - IP-строки валидируются при загрузке (`netip.ParseAddr`).
- Связанные `AC-*`: AC-009.
- Связанные `DEC-*`: DEC-001.
- Поля:
  - `default_route` — `string`, required, default `"server"`.
  - `include_ranges` — `[]string`, optional, CIDR-списки для туннеля.
  - `exclude_ranges` — `[]string`, optional, CIDR-списки для bypass.
  - `include_ips` — `[]string`, optional, отдельные IP для туннеля.
  - `exclude_ips` — `[]string`, optional, отдельные IP для bypass.
  - `include_domains` — `[]string`, optional, домены для туннеля.
  - `exclude_domains` — `[]string`, optional, домены для bypass.
- Жизненный цикл:
  - создаётся: `LoadClientConfig()` парсит YAML.
  - обновляется: только перезапуск клиента (hot-reload вне scope).
- Замечания по консистентности: невалидные CIDR/IP — ошибка загрузки, клиент не стартует.

### DM-002: RuleSet (in-memory engine)

- Назначение: внутреннее представление правил для быстрого матчинга пакетов.
- Источник истины: создаётся из `RoutingCfg` при старте клиента.
- Инварианты:
  - Правила упорядочены: exclude → include → default.
  - default_rule всегда последний.
- Связанные `AC-*`: AC-001–AC-006, AC-010.
- Связанные `DEC-*`: DEC-001, DEC-002.
- Поля:
  - `rules` — `[]Rule`, ordered list.
  - `defaultAction` — `RouteAction`.
- Жизненный цикл:
  - создаётся: `NewRuleSet(cfg)` при старте.
  - обновляется: только перезапуск (runtime ROUTE_UPDATE — вне scope).
- Замечания по консистентности: immutable после создания.

### DM-003: Matcher (interface)

- Назначение: стратегия проверки IP-адреса пакета на соответствие правилу.
- Варианты: `CIDRMatcher`, `ExactIPMatcher`, `DomainMatcher` (resolve → IP).
- Связанные `AC-*`: AC-002, AC-003, AC-005.
- Связанные `DEC-*`: DEC-002.
- Поля:
  - `Match(ip netip.Addr) bool`.
- Жизненный цикл: создаётся из `RoutingCfg` полей, живёт пока жив RuleSet.

### DM-004: DNSCache (in-memory cache)

- Назначение: кеш resolved IP → TTL для доменных правил.
- Источник истины: upstream DNS-сервер.
- Инварианты:
  - entry с истёкшим TTL не возвращается.
  - TTL=0 — не кешируется.
- Связанные `AC-*`: AC-004, AC-005.
- Связанные `DEC-*`: DEC-003.
- Поля:
  - entries — `map[string]cacheEntry{ips []netip.Addr, deadline time.Time}`.
- Жизненный цикл:
  - создаётся: при старте DNS resolver.
  - запись: после успешного resolve.
  - инвалидация: по TTL.
  - read: при `Lookup()`.
- Замечания по консистентности: не thread-safe без `sync.RWMutex`.

## Связи

- `DM-001 RoutingCfg → DM-002 RuleSet`: один к одному, RuleSet строится из RoutingCfg.
- `DM-002 RuleSet → DM-003 Matcher`: RuleSet содержит список Matcher-ов (каждый Rule содержит Matcher + RouteAction).
- `DM-004 DNSCache → DM-003 DomainMatcher`: DomainMatcher использует DNSCache для resolve → IP.

## Производные правила

- `RouteAction` — enum: `server` | `direct`. Определяет, куда направить пакет.
- Default `default_route` = `server` если секция routing отсутствует (AC-009).

## Переходы состояний

- `client.yaml (raw YAML) → LoadClientConfig → RoutingCfg (parsed) → NewRuleSet → RuleSet (runtime)`
- Каждый шаг — валидация; ошибка → клиент не стартует.
- In-memory только; persistence не требуется.

## Вне scope

- Persistence для правил (BoltDB/SQLite)
- Hot-reload / ROUTE_UPDATE dynamic rules
- Priority/weight per rule
- Rule versioning или audit log
