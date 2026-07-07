# fakeDNS domain-based routing — Модель данных

## Scope

- Связанные `AC-*`: `AC-001`, `AC-002`, `AC-003`, `AC-005`, `AC-006`, `AC-008`, `AC-009`
- Связанные `DEC-*`: `DEC-001`, `DEC-003`, `DEC-004`
- Статус: `changed`

## Сущности

### DM-001 ConnectionConfig (поля)

- Назначение: конфигурация Android-клиента (QR/JSON)
- Источник истины: QR-код / ручной ввод
- Инварианты: `routingDomainsEnabled` boolean, default false; списки опциональны
- Связанные `AC-*`: `AC-001`, `AC-002`, `AC-003`, `AC-009`
- Связанные `DEC-*`: —
- Поля:
  - `routingDomainsEnabled` — Boolean, optional, default `false` — включает/отключает fakeDNS
  - `routingExcludeDomains` — List<String>, optional — суффиксы для exclude
  - `routingIncludeDomains` — List<String>, optional — суффиксы для include
- Жизненный цикл: read-only после старта VPN; при переподключении читается заново

### DM-002 FakeIpPool

- Назначение: аллокация fake IP из 198.18.0.0/15; mapping fake IP ↔ реальный домен
- Источник истины: runtime in-memory (не persisted)
- Инварианты: каждый fake IP уникален; одно направление fakeIP→domain; при дисконнекте очищается
- Связанные `AC-*`: `AC-002`, `AC-005`, `AC-006`, `AC-008`
- Связанные `DEC-*`: `DEC-004`
- Поля:
  - `bitmap: BitSet(32768)` — занятые адреса в 198.18.0.0/15
  - `forward: HashMap<fakeIp, domain>` — fake IP → домен
- Жизненный цикл:
  - создаётся при старте VPN (если `routingDomainsEnabled=true`)
  - аллокация при первом DNS-запросе на include/не-matched домен
  - освобождение при дисконнекте VPN (AC-005)
- Замечания по консистентности: при exhaustion (битмапа полна) возвращается ошибка, DNS-запрос forward без обработки

### DM-003 ExcludedIp Reverse Cache

- Назначение: реальный IP exclude домена → маркер для deliver directly
- Источник истины: runtime in-memory, LRU (max 1024 entries)
- Инварианты: каждый IP маппится в признак excluded; LRU eviction при переполнении
- Связанные `AC-*`: `AC-001`, `AC-003`, `AC-005`
- Связанные `DEC-*`: `DEC-003`
- Поля:
  - `cache: LinkedHashMap<ip, Unit>` (access-order, max 1024)
- Жизненный цикл:
  - populate: при резолве exclude домена (все резолвленные IP)
  - evict: LRU при превышении 1024
  - clean: при дисконнекте VPN (AC-005)

## Связи

- `DM-001 → DM-002`: `routingDomainsEnabled=true` — триггер создания FakeIpPool
- `DM-001 → DM-003`: поля `routingExcludeDomains` — источник для суффиксного matching

## Производные правила

- Suffix match: домен матчится, если `".$suffix"` является суффиксом QNAME. Пример: `.ru` → `2ip.ru` матч, `google.com` нет.
- Приоритет проверки routing engine: excluded CIDR/IP → excludedIp set → fakeIp pool → forward как есть

## Переходы состояний

- fakeDNS не активен (`routingDomainsEnabled=false`) → активен (`routingDomainsEnabled=true`) — при старте VPN
- VPN running → cleaning: все кеши очищаются (FakeIpPool, excludedIp reverse cache, DnsCache)

## Вне scope

- Persistence для fake IP mapping (не требуется — runtime only)
- IPv6 fake IP адреса
- Wildcard/glob matching — отдельная фича

Готово к: /speckeep.tasks android-fakedns-routing
