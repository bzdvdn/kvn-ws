# Context — src/internal/dns

## Purpose

Базовый DNS-слой для маршрутизации: TTL-кэш домен → IP, резолвер с кэшем и `Tracker`, который запоминает соответствие IP → домен из DNS-ответов, чтобы `routing.RuleSet` мог принимать решения по домену для уже установленных соединений.

## Responsibilities

- Владеет: `Cache`, `Resolver`/`DefaultResolver`, `Tracker`, парсером ответов `ParseDNSResponse` (A/AAAA, compression pointers).
- Не владеет: сетевым DNS-сервером и маршрутизацией запросов (`dnsproxy`), правилами split-tunnel (`routing`), DNS-конфигом (`config`).

## How it works

- `NewCache()` → потокобезопасная `map[domain]entry` с `deadline`; `Get` при истечении удаляет запись (переход RLock→Lock), `Set(domain, ips, ttl)` ставит `now+ttl`.
- `NewDefaultResolver(cache)` / `NewDefaultResolverWithTTL` — `Resolver` поверх `net.DefaultResolver.LookupNetIP(ctx, "ip", domain)`; результат кэшируется только если `ttl > 0` (по умолчанию 60с).
- `NewTracker(ttl)` → `map[netip.Addr]trackedEntry{domain, expires}` под `RWMutex`.
- `Tracker.Track(domain, ips)` пишет deadline `now+ttl` для каждого IP (последняя запись для IP побеждает); `Lookup(ip)` возвращает домен, а истёкшую запись удаляет.
- `Tracker.TrackResponse(qname, raw)` парсит DNS-ответ и трекает только найденные A/AAAA-адреса.
- `ParseDNSResponse`: пропускает 12-байтовый заголовок и `QDCOUNT` вопросов, затем идёт по `ANCOUNT` ответам, поддерживает сжатые имена (`0xC0`), читает `TYPE/CLASS/TTL/RDLENGTH`, собирает A (`rtype=1,len=4`) и AAAA (`rtype=28,len=16`, `Unmap` для A).

## Key files

- `src/internal/dns/cache.go` — `Cache`.
- `src/internal/dns/resolver.go` — `Resolver`, `DefaultResolver`, `DefaultTTL`.
- `src/internal/dns/tracker.go` — `Tracker`, `ParseDNSResponse`, `skipDNSName`.

## Public contracts

- `Resolver interface { Lookup(ctx, domain) ([]netip.Addr, error) }`.
- `NewCache()`, `(*Cache).Get(domain) ([]netip.Addr, bool)`, `Set(domain, ips, ttl)`, `Clear()`.
- `NewDefaultResolver(cache)`, `NewDefaultResolverWithTTL(cache, ttl)`, `DefaultTTL = 60s`.
- `NewTracker(ttl)`, `(*Tracker).Track(domain, ips)`, `TrackResponse(qname, raw)`, `Lookup(ip) (string, bool)`.
- `ParseDNSResponse(raw []byte) []netip.Addr`.

## Dependencies

- Только стандартная библиотека (`net`, `net/netip`, `sync`, `time`, `encoding/binary`).

## Consumed by

- `src/internal/routing` — `RuleSet.SetTracker` и `DomainMatcher` (через `Resolver`).
- `src/internal/dnsproxy` — `SetTracker`/`TrackResponse` для domain-routing.
- `src/internal/bootstrap/client` — создаёт `Tracker` и передаёт в DNS-прокси и `RuleSet`.

## Gotchas

- `Cache` не ограничен по размеру и не имеет eviction — долгоживущий процесс с уникальными доменами растёт неограниченно.
- `Cache.Get` возвращает **внутренний срез** `ips`; вызывающий может его мутировать и испортить кэш.
- `Set(..., 0)` ставит `deadline = now`, то есть запись сразу просрочена — «без TTL» означает «не кэшировать».
- `Tracker` TTL отсчитывается от момента `Track`, а не от TTL записи DNS; при частых запросах deadline сдвигается вперёд.
- `Tracker` хранит только один домен на IP — CDN/общие IP перетирают предыдущее соответствие.
- `ParseDNSResponse` учитывает только A/AAAA: ответ с одним CNAME и без адресов не даст IP; `skipDNSName` не разворачивает цепочки указателей, а только считает их длину.
- `Lookup` удаляет просроченную запись лениво (на чтении), фоновой очистки нет.
- `DefaultResolver` при `cache == nil` работает без кэша; при ошибке резолва ничего не кэширует.
