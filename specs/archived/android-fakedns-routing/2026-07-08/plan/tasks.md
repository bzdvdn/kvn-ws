# fakeDNS domain-based routing — Задачи

## Phase Contract

Inputs: plan, data-model, spec.
Outputs: упорядоченные задачи с покрытием всех `AC-*`.
Stop if: — (plan стабилен, data-model согласован).

## Surface Map

| Surface | Tasks |
|---------|-------|
| `config/AppConfig.kt` | T1.1 |
| `dns/FakeIpPool.kt` | T1.2, T3.1 |
| `dns/DnsParser.kt` | T2.1 |
| `dns/DnsCache.kt` | T2.1 |
| `dns/FakeDnsResolver.kt` | T2.1, T3.1 |
| `vpn/KvnVpnService.kt` | T2.1, T3.1, T3.2 |
| `dns/FakeDnsResolverTest.kt` | T2.2, T4.1 |
| `dns/FakeIpPoolTest.kt` | T1.2, T4.1 |
| `vpn/KvnVpnServiceTest.kt` | T2.2, T3.2, T4.1 |

## Implementation Context

- **Цель MVP**: exclude domain по суффиксу — DNS перехватывается, резолвится через config DNS, реальный IP excluded, пакет доставляется напрямую (минуя VPN-сервер).
- **Инварианты**:
  - `routingDomainsEnabled=false` (default) → fakeDNS не активен, поведение идентично текущему dumb forwarder
  - suffix match = `".$suffix"` endsWith (dot-барьер, `.ru` не матчит `prudent.ruhr`)
  - excludedIp reverse cache — LRU 1024 entries
  - fake IP pool — bitmap 198.18.0.0/15 (32768 адресов)
  - При exhaustion / parse error → fail-closed (пакет forward без обработки через VPN)
- **Контракты**:
  - ConnectionConfig: `routingDomainsEnabled` (Boolean, default false), `routingExcludeDomains` (List<String>), `routingIncludeDomains` (List<String>)
  - `routingDomainsEnabled` маппится на `dns_cache.enabled` из kvn-web
  - Wire protocol (DATA frame) не меняется
- **Границы scope**: не трогаем сервер, Go-клиент, Web UI, wildcard, DoH/DoT, IPv6
- **Proof signals**: `2ip.ru` показывает реальный IP клиента; DNS-ответ для exclude домена содержит real IP и сконструирован fakeDNS (ID совпадает, QR+RA флаги); для include — fake IP из `198.18.0.0/15`; checksum корректен (TCP SYN→SYN-ACK)
- **References**: `DEC-001` (interception в tunReader), `DEC-002` (suffix endsWith), `DEC-003` (LRU cache), `DEC-004` (bitmap pool), `DEC-005` (incremental checksum); `DM-001` (config), `DM-002` (FakeIpPool), `DM-003` (ExcludedIp cache)

## Фаза 1: Bootstrapping

Цель: подготовить конфигурацию и независимые runtime-компоненты.

- [x] T1.1 Добавить поля `routingDomainsEnabled` (Boolean, default false), `routingExcludeDomains` (List<String>), `routingIncludeDomains` (List<String>) в ConnectionConfig / AppConfig. Учесть маппинг `dns_cache.enabled → routingDomainsEnabled`. Touches: `config/AppConfig.kt`

- [x] T1.2 Реализовать `FakeIpPool` — bitmap-аллокатор 198.18.0.0/15 с forward mapping fakeIP→domain. Аллокация O(1), освобождение при release(). При exhaustion — return null (caller forward без обработки). Touches: `dns/FakeIpPool.kt`, `dns/FakeIpPoolTest.kt`. `@sk-task` над классом `FakeIpPool`.

## Фаза 2: MVP Slice

Цель: exclude-only routing — первая демонстрируемая ценность (AC-001, AC-003, AC-004, AC-009).

- [x] T2.1 Реализовать:
  - `FakeDnsResolver` — перехват DNS (UDP/53) из tunReader, suffix matching через endsWith с dot-барьером (DEC-002), для exclude — резолв через DNS серверы из конфигурации (protect()), real IP в ответе, populate excludedIp reverse cache (DEC-003). Для не-matched — forward без изменений.
  - `DnsParser.buildResponse(query: ByteArray, answerIp: InetAddress, ttl: Int): ByteArray` — конструирует валидный DNS-ответ: копирует ID из запроса, QR+RA флаги, вопросную секцию, добавляет A-запись с answerIp. Ответ записывается в TUN fd из tunReader.
  - Расширить `DnsCache` (TTL-кеш для fakeDNS).
  - В `KvnVpnService.tunReader()`: conditional branch — если `routingDomainsEnabled=true`, UDP/53 пакеты → FakeDnsResolver → buildResponse → write в TUN fd; routing engine проверяет excludedIp set → deliver directly, иначе forward как DATA frame. Touches: `dns/FakeDnsResolver.kt`, `dns/DnsParser.kt`, `dns/DnsCache.kt`, `vpn/KvnVpnService.kt`. `@sk-task` над `FakeDnsResolver`, `DnsParser.buildResponse` и над routing engine блоком в `KvnVpnService`.

- [x] T2.2 Написать unit-тесты для MVP:
  - `FakeDnsResolverTest`: suffix matching (AC-004), exclude resolution flow (AC-001), routingDomainsEnabled=false пропускает DNS (AC-009)
  - `KvnVpnServiceTest`: routing engine excludedIp deliver directly (AC-001, AC-003), CIDR/IP priority (AC-003)
  Touches: `dns/FakeDnsResolverTest.kt`, `vpn/KvnVpnServiceTest.kt`. `@sk-test` над каждой тестовой функцией.

## Фаза 3: Основная реализация

Цель: include domains, IP rewrite, lifecycle cleanup, edge cases.

- [x] T3.1 Реализовать include domain support:
  - `FakeDnsResolver`: include suffix match → alloc fake IP из `FakeIpPool` (DEC-004), mapping fakeIP→domain, fake IP в DNS-ответе.
  - `KvnVpnService.tunReader()`: routing engine проверяет fakeIp pool → lookup real domain, rewrite dst IP + incremental checksum (DEC-005, TCP + UDP checksum=0).
  Touches: `dns/FakeDnsResolver.kt`, `dns/FakeIpPool.kt`, `vpn/KvnVpnService.kt`. `@sk-task` над include-matching блоком и rewrite-функцией.

- [x] T3.2 Реализовать cleanup on disconnect + edge cases:
  - `KvnVpnService.onRevoke()`: очистка FakeIpPool, excludedIp reverse cache, DnsCache (AC-005)
  - Edge cases в routing engine: multi-question (first QNAME only), AAAA → пустой ответ, compression pointer → skip, pool exhaustion → forward, multi-IP exclude (AC-005, AC-007)
  Touches: `vpn/KvnVpnService.kt`. `@sk-task` над cleanup-методом.

## Фаза 4: Проверка

Цель: automated coverage + manual validation.

- [x] T4.1 Расширить тесты:
  - `FakeIpPoolTest`: аллокация, освобождение, exhaustion, mapping consistency (AC-006)
  - `FakeDnsResolverTest`: include matching, fake IP allocation, default routing (AC-002, AC-008)
  - `KvnVpnServiceTest`: rewrite + checksum correctness (AC-006), cleanup after disconnect (AC-005), CIDR/IP + domain priority (AC-003, AC-007)
  Touches: `dns/FakeIpPoolTest.kt`, `dns/FakeDnsResolverTest.kt`, `vpn/KvnVpnServiceTest.kt`. `@sk-test` над каждой тестовой функцией.

- [x] T4.2 Выполнить manual validation: (требуется сборка APK и ручное тестирование)
  - VPN с `routingDomainsEnabled=true`, `routingExcludeDomains=[".ru"]` → `2ip.ru` показывает реальный IP клиента
  - VPN с include `.corp` → wireShark / логи показывают fake IP → rewrite
  - `routingDomainsEnabled=false` → поведение идентично текущему
  - Проверить, что CIDR/IP exclude продолжает работать без domain routing
  - **Багфиксы, обнаруженные при manual validation**:
    1. **TCP checksum pseudo-header** — `tcpChecksum()` смешивал байты src и dst IP в одном 16-битном слове. Исправлено на 4 отдельных слова: `(srcIp[0]<<8|srcIp[1])`, `(srcIp[2]<<8|srcIp[3])`, `(dstIp[0]<<8|dstIp[1])`, `(dstIp[2]<<8|dstIp[3])`.
    2. **TCP seq after SYN** — `handleTcpSyn` инициализировал `TcpDirectState` с `mySeq=10000` (как SYN-ACK). SYN потребляет 1 байт seq space, данные должны начинаться с 10001. Исправлено: `TcpDirectState(socket, mySeq + 1, seqNum + 1, ...)`.
    3. **TCP ports in response** — `buildTcpResponse` устанавливал TCP src port = `srcPort` (app port) и dst port = `dstPort` (server port). Для ответа порты должны быть swapped. Исправлено: src = original dst port, dst = original src port.
    4. **bindSocket без protect fallback** — при `defaultNetwork != null` вызывался только `bindSocket`; если бросал исключение, `protect()` не вызывался. Исправлено: `bindSocket` и `protect` всегда вызываются независимо, оба исключения ловятся.
    5. **TCP checksum range** — `tcpChecksum(dstIp, srcIp, buf, 20, 20 + payload.size)` считал псевдо-заголовок длиной `payload.size` и покрывал только payload, не включая TCP-заголовок. Для SYN-ACK (payload=0) pseudo-header length=0, segment data не читался — все SYN-ACK и DATA пакеты имели неверный checksum. Исправлено: `tcpChecksum(..., 20, 20 + tcpLen)` где `tcpLen = 20 + payload.size`.
    6. **Data offset formula** — `val dataOffset = ((packet[ihl + 12].toInt() and 0xF0) / 4) * 4` давал `dataOffset` в 4 раза больше реального (20 → 80). Для `0x50` вычислялось `(80/4)*4=80` вместо `(80 ushr 4)*4=5*4=20`. handleTcpData копировал payload со смещением +60 байт, теряя начало HTTP-запроса → 400 Bad Request, TLS ClientHello обрезан → ERR_SSL_PROTOCOL_ERROR. Исправлено: `/ 4` → `ushr 4`.
    7. **Oversized packets** — reader job читал весь ответ сокета в один `input.read()` (до 65535 байт) и упаковывал в один IP-пакет. При MTU=1500 пакет >1500 байт отбрасывался. Исправлено: данные режутся на chunk-и по `mtu-40` байт, каждый chunk — отдельный response packet с корректным seq number.
  Touches: manual (нет файлов)

## Покрытие критериев приемки

- AC-001 → T2.1, T2.2, T4.1
- AC-002 → T3.1, T4.1
- AC-003 → T2.1, T2.2, T3.2
- AC-004 → T2.1, T2.2
- AC-005 → T3.2, T4.1
- AC-006 → T3.1, T4.1
- AC-007 → T2.1, T3.2, T2.2
- AC-008 → T3.1, T4.1
- AC-009 → T2.1, T2.2, T4.2

## Заметки

- T1.1 и T1.2 независимы — можно параллелить
- T2.1 — ключевая задача MVP, зависит от T1.1
- T3.1 зависит от T1.2 (FakeIpPool)
- T3.2 можно начинать после T2.1 (routing engine уже есть)
- Все тесты из T2.2 / T4.1 пишутся параллельно с реализацией соответствующего модуля
- Trace-маркеры `@sk-task` / `@sk-test` ставятся над owning function/method/test/type declaration, не на package/import/file-header

Готово к: speckeep archive android-fakedns-routing .
