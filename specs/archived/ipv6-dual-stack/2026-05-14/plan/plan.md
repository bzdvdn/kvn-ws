# IPv6 & Dual-Stack — План

## Цель

Добавить поддержку IPv6 в TUN-туннель: отдельный IPv6-пул, расширение handshake, IPv6 NAT и раздельная маршрутизация. Существующая IPv4-логика остаётся без регрессии.

## MVP Slice

Dual-stack TUN + IPv6 IP pool + расширение handshake + IPv6 NAT. AC-001, AC-002, AC-003, AC-004.

## First Validation Path

Ручная проверка: сервер с `pool_ipv6: fd00::/112`, клиент с `ipv6: true` → `ip -6 addr show dev kvn` показывает адрес, `ping6 fd00::1` проходит, `nft list table ip6 kvn-nat` показывает masquerade.

## Scope

- IPv6 пул: отдельный `IPPool` для IPv6 с аллокацией из `/112` подсети
- Handshake: ServerHello с length-prefixed IP (семейство + длина + адрес)
- TUN: `SetIP6()` метод или расширение `SetIP()` для IPv6
- NAT: nftables table `ip6 kvn-nat` с postrouting masquerade
- Роутинг: `parseDstIP6()` + определение IP-версии пакета по первой ниббле
- DNS: dual-stack резолв (A + AAAA)
- Конфиг: `NetworkCfg.PoolIPv6`, клиентский `IPv6` флаг
- Kill-switch: поддержка IPv6 (nftables table `ip6`)

**Вне scope:** DNS64/NAT64, IPv6-only транспорт, ICMPv6 PMTUD.

## Implementation Surfaces

| Surface | Путь | Роль | Статус |
|---|---|---|---|
| TunDevice | `src/internal/tun/tun.go` | Настройка IPv6 адреса на TUN | сущ. |
| IPPool | `src/internal/session/session.go` | IPv6 пул аллокации / релиза | сущ., расширение |
| Handshake | `src/internal/protocol/handshake/handshake.go` | Передача IPv6 в ServerHello | сущ., расширение |
| NAT | `src/internal/nat/nftables.go` | nftables ip6 table | сущ., расширение |
| Router | `src/internal/routing/router.go` | Парсинг dst IPv6 | сущ., расширение |
| DNS | `src/internal/dns/resolver.go` | AAAA запросы | сущ., расширение |
| Server config | `src/internal/config/server.go` | PoolIPv6 | сущ., расширение |
| Client config | `src/internal/config/client.go` | IPv6 флаг (уже есть) | сущ., dead код |
| Server main | `src/cmd/server/main.go` | Инициализация двух пулов | сущ., расширение |
| Client main | `src/cmd/client/main.go` | TUN IPv6 + kill-switch ip6 | сущ., расширение |

## Bootstrapping Surfaces

`none` — все нужные файлы существуют.

## Влияние на архитектуру

- `SessionManager` получает второй `*IPPool` для IPv6. Локально — расширение конструктора.
- `Session` получает `AssignedIPv6 net.IP` — обратная совместимость через проверку на nil.
- `NFTManager` получает флаг `ipv6 bool` для Setup/Teardown.
- `ServerHello.Encode` меняет формат — требует обновления клиента (несовместимость протокола).

## Acceptance Approach

- **AC-001** (TUN IPv6): `TunDevice.SetIP()` с `*net.IPNet` для IPv6 → `ip -6 addr show dev kvn`
- **AC-002** (Пул): два клиента, два вызова `Pool.Allocate()` возвращают разные fd00:: адреса
- **AC-003** (NAT): `NFTManager.Setup()` с ipv6=true → `nft list table ip6 kvn-nat`
- **AC-004** (ping6): end-to-end: клиент → сервер → nft masquerade → icmp echo reply
- **AC-005** (Dual-stack routing): `parseDstIP()` определяет v4/v6 → `RuleSet.Route()` → correct action

Surfaces: AC-001→tun + pool; AC-002→pool; AC-003→nat; AC-004→tun+pool+handshake+nat; AC-005→router.

## Данные и контракты

См. `data-model.md`. Ключевые изменения:
- `ServerHello.Encode/Decode` — wire format меняется. Старые клиенты несовместимы.
- `BoltDB` — новый ключ `session:{id}:ipv6`, старые записи игнорируются.

## Стратегия реализации

### DEC-001: Раздельные IPv4/IPv6 пулы

**Why:** IPv6 /64 слишком большой для линейного сканирования. Раздельные пулы позволяют разным стратегиям аллокации (IPv4 — range scan, IPv6 — `/112` shard с random offset или counter).
**Tradeoff:** Два пула = два mutex'а, больше кода в SessionManager. Но логика проще, чем generic pool.
**Affects:** `session/session.go`, `cmd/server/main.go`
**Validation:** `Allocate()` возвращает IP из fd00::/112.

### DEC-002: Length-prefixed IP в ServerHello

**Why:** IPv6 — 16 байт, IPv4 — 4 байта. Единый формат с family + length универсален и расширяем (IPoIB и т.д.).
**Tradeoff:** Ломает wire-совместимость со старыми клиентами (все клиенты должны обновиться).
**Affects:** `protocol/handshake/handshake.go`
**Validation:** Encode → Decode возвращает те же IP.

### DEC-003: IPv6 NAT через отдельную nftables table

**Why:** nftables `ip` family не обрабатывает IPv6. `ip6` — чистое разделение, нет риска сломать IPv4 NAT.
**Tradeoff:** Две таблицы = два nft вызова при setup. Минимально.
**Affects:** `nat/nftables.go`
**Validation:** `nft list table ip6 kvn-nat` показывает masquerade.

### DEC-004: Определение семейства пакета по версии IP

**Why:** Первый ниббл IPv4 = 4, IPv6 = 6. Парсинг без доп. контекста.
**Tradeoff:** Требует две копии `parseDstIP()` — незначительный дубляж.
**Affects:** `routing/router.go`
**Validation:** Пакет IPv4 → parseDstIP ; пакет IPv6 → parseDstIP6.

### DEC-005: DNS dual-stack

**Why:** `net.Resolver.LookupNetIP(ctx, "ip", domain)` возвращает и A, и AAAA записи.
**Tradeoff:** Двойные DNS-запросы. Минимизировано TTL-кэшем.
**Affects:** `dns/resolver.go`
**Validation:** Домен с AAAA записью резолвится в IPv6 адрес.

## Incremental Delivery

### MVP (Первая ценность)

Задачи: IPv6 пул → handshake → TUN IPv6 → IPv6 NAT. AC-001, AC-002, AC-003, AC-004.
Проверка: `ping6 fd00::1` с клиента.

### Итеративное расширение

1. Dual-stack routing (AC-005): парсинг IPv6 пакетов + маршрутизация по правилам.
2. DNS AAAA: резолвер с `"ip"` netwok.
3. Kill-switch IPv6: блокировка IPv6 трафика вне туннеля.

## Порядок реализации

1. **Config**: `PoolIPv6` в `NetworkCfg` — без этого ничего не сконфигурировать.
2. **Pool**: `IPPool` с IPv6 стратегией — без пула нет адресов.
3. **Handshake**: length-prefixed ServerHello — клиент и сервер должны договориться.
4. **TUN**: настройка IPv6 на TUN интерфейсе — следующий шаг после получения адреса.
5. **NAT**: nftables ip6 — сервер должен уметь NATить IPv6.
6. **SessionManager**: второй пул + `AssignedIPv6` — интеграция всех компонентов.

Пункты 7-8 (Routing, DNS, Kill-switch) — после MVP, параллельно.

## Риски

- **Wire breakage**: старый клиент не поймёт новый ServerHello. **Mitigation:** bump proto version, старый клиент получит AuthError с пояснением.
- **IPv6 /112 размер**: 65534 адреса, аллокация без сканирования. **Mitigation:** random offset + bitmap allocation.
- **nftables ip6 отсутствует**: ядро без `CONFIG_NF_TABLES_IPV6`. **Mitigation:** graceful degradation — IPv6 NAT disabled, warning в лог.

## Rollout и compatibility

- Изменение wire format — **major version bump** протокола. Все клиенты должны обновиться.
- Старый сервер с новым клиентом: клиент шлёт ClientHello с флагом IPv6, сервер не понимает — игнорирует, назначает только IPv4.
- Новый сервер со старым клиентом: сервер шлёт ServerHello с length-prefixed IP, старый клиент decode падает → ошибка.
- Feature flag: `server.pool_ipv6` опционально, без него — только IPv4.

## Проверка

- **Unit тесты:** IPPool IPv6 аллокация, handshake encode/decode, parseDstIP6.
- **Integration:** NFTManager Setup/Teardown ip6, TunDevice SetIP IPv6.
- **Gate test:** `docker-compose.test.yml` с dual-stack конфигом, `ping6` и `nft` assertions.
- **AC coverage:** каждый AC имеет минимум один automated test.

## Соответствие конституции

- Нет конфликтов. Go-код, DDD (доменный пакет session/pool, infrastructure nat/dns), traceability (`@sk-task` / `@sk-test`) — все требования соблюдены.
