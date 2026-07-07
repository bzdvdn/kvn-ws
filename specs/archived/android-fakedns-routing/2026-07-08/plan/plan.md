# fakeDNS domain-based routing — План

## Phase Contract

Inputs: spec, inspect (pass), constitution summary, repo map.
Outputs: plan, data-model.
Stop if: — (spec стабильна, inspect pass).

## Цель

Добавить в Android-клиент перехват DNS (UDP/53) через fakeDNS для маршрутизации по доменам (exclude/include по суффиксу) с флагом `routingDomainsEnabled`. Флаг по умолчанию `false` — обратная совместимость с существующим CIDR/IP routing. Флаг маппится на `dns_cache.enabled` из kvn-web.

## MVP Slice

- exclude domains только (без include domains)
- fakeDNS перехватывает DNS, для exclude возвращает реальный IP через DNS серверы из конфигурации
- fakeDNS конструирует DNS-ответ и пишет его в TUN fd
- routing engine проверяет dst IP по excludedIp set → deliver directly или forward на сервер
- AC-001, AC-003, AC-004, AC-009

## First Validation Path

1. Собрать APK с конфигурацией `routingDomainsEnabled: true`, `routingExcludeDomains: [".ru"]`
2. Запустить VPN, открыть в браузере `2ip.ru`
3. Убедиться, что `2ip.ru` показывает реальный IP клиента (не сервера)
4. Открыть `google.com` — трафик идёт через VPN (показывает IP сервера)

## Scope

- `src/android/app/src/main/kotlin/com/kvn/client/dns/FakeDnsResolver.kt` — новый: перехват DNS, suffix matching, резолв через DNS серверы из конфигурации, конструирование DNS-ответа и запись в TUN fd
- `src/android/app/src/main/kotlin/com/kvn/client/dns/FakeIpPool.kt` — новый: аллокация fake IP из `198.18.0.0/15`, mapping fakeIP ↔ домен
- `src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt` — routing engine в tunReader: excludedIp check → deliver directly; fakeIp check → rewrite dst IP + checksum → forward
- `src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt` — поля `routingDomainsEnabled`, `routingIncludeDomains`, `routingExcludeDomains`
- `src/android/app/src/main/kotlin/com/kvn/client/dns/DnsParser.kt` — расширение: buildResponse (конструкция DNS-ответа из запроса + answer IP)
- `src/android/app/src/main/kotlin/com/kvn/client/dns/DnsCache.kt` — расширение (fakeDNS использует как TTL-кеш)

Вне scope: сервер, Go-клиент, Web UI, wildcard/glob matching, DoH/DoT, IPv6.

## Performance Budget

- exclude DNS resolution (fakeDNS → system DNS → response): <50ms p99 поверх времени системного DNS
- IP rewrite + checksum: <1ms p95 на пакет
- Memory: <1 MB peak overhead (fake IP pool bitmap ~4KB, reverse cache LRU до 1024 записей)

## Implementation Surfaces

| Surface | Роль | Статус |
|---|---|---|
| `dns/FakeDnsResolver.kt` | interception, matching, resolution | новый |
| `dns/FakeIpPool.kt` | fake IP allocation, mapping | новый |
| `dns/DnsParser.kt` | DNS wire parse + response build | расширение |
| `dns/DnsCache.kt` | TTL-кеш DNS ответов | расширение |
| `vpn/KvnVpnService.kt` | routing engine, packet rewrite | изменение |
| `config/AppConfig.kt` | новые поля конфигурации | изменение |
| `dns/FakeDnsResolverTest.kt` | unit-тесты | новый |
| `dns/FakeIpPoolTest.kt` | unit-тесты | новый |
| `vpn/KvnVpnServiceTest.kt` | routing engine тесты | расширение |

## Bootstrapping Surfaces

- `config/AppConfig.kt` — поля должны быть добавлены первыми (от них зависит поведение остальных модулей)
- `dns/FakeDnsResolver.kt`, `dns/FakeIpPool.kt` — могут создаваться параллельно

## Влияние на архитектуру

- В `KvnVpnService.tunReader()` появляется conditional branch: если `routingDomainsEnabled` — UDP/53 пакеты направляются в `FakeDnsResolver`, который конструирует DNS-ответ и пишет его обратно в TUN fd (приложение получает ответ как от реального DNS). Остальные пакеты проходят через routing engine с проверкой excludedIp / fakeIp.
- `FakeDnsResolver` использует DNS серверы из конфигурации (per-app DNS, `android-per-app-dns`) для резолва exclude доменов.
- Флаг `routingDomainsEnabled` по умолчанию `false` — поведение существующего dumb forwarder не меняется.

## Acceptance Approach

| AC | Подход | Surfaces | Наблюдение |
|---|---|---|---|
| AC-001 | fakeDNS перехватывает DNS → exclude suffix match → резолв через config DNS → buildResponse → запись в TUN fd | FakeDnsResolver, DnsParser.buildResponse, DnsCache | Лог: resolve через config DNS, excludedIp set содержит IP; DNS-ответ сконструирован и записан в TUN |
| AC-003 | routing engine проверяет excludedIp set до domain matching | KvnVpnService.tunReader | Пакет на CIDR/IP из exclude доставлен напрямую |
| AC-004 | suffix match: `.ru` матчит `mail.ru`, не матчит `google.com` | FakeDnsResolver.matchDomain() | Unit test с разными входными |
| AC-009 | `routingDomainsEnabled=false` → tunReader не запускает fakeDNS, DNS-ответ не конструируется | KvnVpnService.tunReader | UDP/53 пакет идёт как обычный DATA frame |
| AC-002 | include suffix → fake IP → mapping → buildResponse (fake IP в ответе) → rewrite dst IP → forward | FakeIpPool, FakeDnsResolver, DnsParser.buildResponse, KvnVpnService | DNS-ответ с fake IP из 198.18.0.0/15 сконструирован и записан в TUN |
| AC-005 | stop VPN → очистка FakeIpPool, excludedIp reverse cache, DnsCache | KvnVpnService.onRevoke | После реконнекта mapping не восстанавливается |
| AC-006 | TCP fake dst IP → rewrite + checksum → SYN-ACK от сервера | KvnVpnService.rewritePacket | Integration test с реальным TCP-соединением |
| AC-007 | прямой IP без DNS → CIDR/IP exclude → deliver directly | KvnVpnService.tunReader | SYN на exclude IP не уходит в WS |
| AC-008 | не-matched домен → fake IP → buildResponse → rewrite → forward | FakeDnsResolver, FakeIpPool, DnsParser.buildResponse | DNS-ответ с fake IP из 198.18.0.0/15 сконструирован, пакет реврайтнут |

## Данные и контракты

- ConnectionConfig (QR): добавляются поля `routingDomainsEnabled` (boolean, default false), `routingExcludeDomains` (list of string), `routingIncludeDomains` (list of string)
- Нет изменений в wire protocol (сервер не меняет формат DATA frame)
- Нет изменений в persisted state (BoltDB/SQLite не затрагиваются)
- `data-model.md` — описание runtime сущностей

## Стратегия реализации

- DEC-001 FakeDNS перехват в tunReader до DATA frame + DNS response injection в TUN fd
  Why: единственная точка, где виден весь IP-трафик; не требует отдельного потока/прокси. Существующий tunReader уже читает IP-пакеты — добавление conditional branch (UDP/53 → fakeDNS) минимально инвазивно. fakeDNS конструирует ответ и пишет обратно в TUN fd — приложение видит ответ как от реального DNS.
  Tradeoff: увеличивает latency tunReader для DNS-пакетов; требует синхронизации доступа к excludedIp set / fakeIp pool; должен уметь конструировать валидные DNS-ответы.
  Affects: KvnVpnService.tunReader(), FakeDnsResolver, DnsParser

- DEC-002 Suffix matching через endsWith с dot-барьером
  Why: `.ru` не должен матчить `prudent.ruhr` — конкатенация `.` + suffix + endsWith даёт корректное поведение без regex. O(n) по числу суффиксов, n — единицы/десятки.
  Tradeoff: не поддерживает wildcard/glob; не матчит голый `ru` (без точки) — осознанное упрощение.
  Affects: FakeDnsResolver.matchDomain()
  Validation: AC-004

- DEC-003 excludedIp reverse cache — LRU с max 1024 entries
  Why: exclude домен может резолвиться в несколько IP; без reverse cache routing engine не поймёт, что пакет на этот IP нужно deliver directly. LRU предотвращает переполнение при большом числе уникальных exclude доменов.
  Tradeoff: при cache miss пакет пойдёт через VPN (fail-closed — безопасно). При 1024 entry + 1 резолв ~4 IP — покрывает ~256 уникальных exclude доменов.
  Affects: FakeDnsResolver (populate), KvnVpnService.tunReader (lookup)
  Validation: AC-001, AC-003

- DEC-004 Fake IP аллокация битмапой 198.18.0.0/15 (32768 адресов)
  Why: bitmap (4KB) — детерминированная O(1) аллокация/освобождение. 32768 адресов достаточно для realistic use (Android-клиент одновременно резолвит единицы доменов).
  Tradeoff: при exhaustion возвращается ошибка, DNS-запрос пропускается без обработки (fail-closed — трафик идёт через VPN).
  Affects: FakeIpPool
  Validation: AC-002, AC-008

- DEC-005 IP checksum rewrite — вычисление incremental checksum (RFC 1624)
  Why: полный пересчёт для каждого TCP/UDP пакета дорог; incremental checksum (delta = old_ip ⊕ new_ip) быстрее и проще.
  Tradeoff: UDP-пакеты с включённым checksum требуют отдельной обработки (UDP length dependency) — см. Open Question #2 в spec. Для MVP обрабатываем только TCP + UDP c checksum=0.
  Affects: KvnVpnService.rewritePacket()
  Validation: AC-006

## Incremental Delivery

### MVP (Первая ценность)

- AppConfig: `routingDomainsEnabled`, `routingExcludeDomains`
- FakeDnsResolver: только exclude matching, резолв через config DNS, buildResponse + запись в TUN fd
- KvnVpnService: conditional fakeDNS interception, excludedIp deliver directly
- FakeIpPool: заглушка (не нужна для exclude-only)
- AC-001, AC-003, AC-004, AC-009

### Итеративное расширение 1 — include domains

- FakeIpPool: полная реализация
- FakeDnsResolver: include matching, fake IP allocation
- KvnVpnService: rewrite dst IP + checksum
- AC-002, AC-006, AC-008

### Итеративное расширение 2 — lifecycle и edge cases

- Cleanup on disconnect (AC-005)
- CIDR/IP exclude с доменами (AC-003 уже частично)
- Edge cases: multi-question, AAAA, compression pointer, pool exhaustion
- AC-005, AC-007

## Порядок реализации

1. `AppConfig.kt` — поля конфигурации (нужны сразу)
2. `FakeIpPool.kt` — независим, можно параллельно с AppConfig
3. `FakeDnsResolver.kt` — зависит от AppConfig (для DNS servers), DnsParser, DnsCache
4. `KvnVpnService.kt` — routing engine изменения, зависит от FakeDnsResolver и FakeIpPool
5. Тесты — параллельно с каждым модулем
6. Edge cases и cleanup — после базового сценария

## Риски

- **Риск: некорректный checksum для UDP** — Open Question #2, UDP pseudo-header checksum зависит от UDP length. Mitigation: в MVP обрабатываем только TCP; UDP с ненулевым checksum forward как есть (fail-closed — через VPN).
- **Риск: race condition в tunReader** — несколько пакетов могут одновременно читаться. Mitigation: excludedIp set / fakeIp pool — synchronized collection или lock-free структура.
- **Риск: DNS compression pointer в QNAME** — DnsParser.extractQName может вернуть неполное имя. Mitigation: если extractQName не смог распарсить — пакет forward без обработки (fail-closed).
- **Риск: regression для обычного трафика** — routing engine добавляет проверку для каждого пакета. Mitigation: `routingDomainsEnabled=false` (default) — проверки нет, поведение идентично текущему.

## Rollout и compatibility

- `routingDomainsEnabled` default `false` — полная обратная совместимость
- Сервер не требует обновления
- Старые QR-конфигурации без новых полей работают как раньше (поля опциональны, default)
- Специальных rollout-действий не требуется

## Проверка

- `FakeDnsResolverTest.kt` — unit: suffix matching, DNS parse/response build, exclude vs include vs default (AC-001, AC-002, AC-004, AC-008)
- `FakeIpPoolTest.kt` — unit: аллокация, освобождение, exhaustion, mapping consistency (AC-006)
- `KvnVpnServiceTest.kt` — integration/mock: routing engine decisions, excludedIp direct delivery, rewrite + checksum (AC-001, AC-003, AC-005, AC-006, AC-007, AC-009)
- Manual: `2ip.ru` test (AC-001), `example.com` через VPN (AC-008), реконнект (AC-005)

## Соответствие конституции

- нет конфликтов
- Go 1.22+ / Kotlin 1.9+ — не затрагивается
- DDD + Clean Architecture — `dns/` и `vpn/` соответствуют текущей структуре
- Traceability: `@sk-task` над owning function/method/test declaration

Готово к: /speckeep.tasks android-fakedns-routing
