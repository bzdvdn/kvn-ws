# fakeDNS domain-based routing for Android client

## Scope Snapshot

- In scope: Android VPN client получает возможность исключать и включать домены из VPN-туннеля через fakeDNS — по суффиксу (`.ru`, `.ozon.ru`, `.corp`) в дополнение к существующему CIDR/IP routing, с флагом `routingDomainsEnabled` для включения/отключения.
- Out of scope: Изменения серверной части, Go-клиента, Web UI, поддержка wildcard (`*.ozon.ru`) — только суффиксный match.

## Цель

Пользователь Android-клиента получает возможность указать список exclude-доменов (`.ozon.ru`, `.ru`) — трафик на эти домены идёт напрямую, минуя VPN-сервер — и include-доменов (`.corp`, `.internal`) — трафик гарантированно идёт через VPN. Это даёт гибкий split-tunnel на уровне доменов без необходимости указывать конкретные IP/CIDR. Успех измеряется тем, что запрос к `2ip.ru` возвращает реальный IP клиента (не сервера), а запрос к `internal.corp` идёт через VPN.

## Основной сценарий

0. Пользователь в конфигурации Android-клиента указывает `routingDomainsEnabled: true` (по умолчанию `false`) — включает fakeDNS.
1. Пользователь также указывает `routingExcludeDomains: [".ru", ".ozon.ru"]` и/или `routingIncludeDomains: [".corp"]`.
2. VPN-сервис стартует с TUN на `0.0.0.0/0` — ВЕСЬ трафик идёт через TUN.
3. Если `routingDomainsEnabled: false` — fakeDNS не запускается, DNS-пакеты не перехватываются, весь трафик обрабатывается по CIDR/IP routing как раньше.
4. Если `routingDomainsEnabled: true` — fakeDNS-перехватчик в `tunReader()` читает UDP-пакеты на порт 53, парсит DNS-запрос, извлекает домен.
5. Домен матчится по суффиксу против exclude/include списков. fakeDNS определяет, какой IP вернуть (реальный для exclude, fake для include/default):
   - **exclude** (`.ru`, `.ozon.ru`): DNS резолвится через DNS серверы из конфигурации (с `protect()`), получен реальный IP. Реальный IP сохраняется в reverse cache `excludedIp → domain` в runtime HashMap.
   - **include** (`.corp`): из пула `198.18.0.0/15` аллоцируется fake IP, сохраняется mapping fakeIP → домен.
   - **по умолчанию** (не exclude и не include): аллоцируется fake IP из пула.
6. fakeDNS конструирует DNS-ответ:
   - ID, Flags (QR+RA), вопросная секция копируются из исходного запроса
   - Source IP = IP DNS-сервера (dst оригинального запроса)
   - Dest IP = IP приложения (src оригинального запроса)
   - Answer section содержит A-запись с real/fake IP, TTL
   - Ответ записывается напрямую в TUN fd (даже не доходя до WebSocket tunnel)
   - Приложение получает ответ как от реального DNS-сервера и извлекает IP адрес
7. routing engine в `tunReader()` для каждого последующего IP-пакета:
   - dst IP в `excludedIp` set → deliver directly (через `protect()` + raw socket или пропуск мимо TUN)
   - dst IP в `fakeIp` pool → lookup реальный домен, rewrite dst IP, пересчёт checksum (IP + TCP/UDP pseudo-header), forward на сервер
   - иначе → forward на сервер как есть
8. При дисконнекте/стопе VPN все кеши и маппинги очищаются.

## MVP Slice

- exclude domains только (без include domains)
- fakeDNS перехватывает DNS, для exclude возвращает реальный IP
- fakeDNS конструирует DNS-ответ и пишет его в TUN fd
- routing engine в tunReader: проверка dst по excludedIp set → deliver directly или forward на сервер
- AC-001, AC-003, AC-004, AC-009

## First Deployable Outcome

- После первого implementation pass можно указать `.ru` в exclude, запустить VPN, открыть `2ip.ru` в браузере — покажет реальный IP, а не IP сервера.

## Scope

- `src/android/app/src/main/kotlin/com/kvn/client/dns/DnsParser.kt` — расширение: buildResponse (конструкция DNS-ответа из запроса + answer IP)
- `src/android/app/src/main/kotlin/com/kvn/client/dns/DnsCache.kt` — расширение (fake dns resolver использует)
- `src/android/app/src/main/kotlin/com/kvn/client/dns/FakeDnsResolver.kt` — новый файл: перехват DNS, matching, резолв
- `src/android/app/src/main/kotlin/com/kvn/client/dns/FakeIpPool.kt` — новый файл: аллокация fake IP, mapping
- `src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt` — routing engine в tunReader, IP rewrite + checksum
- `src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt` — новые поля `routingDomainsEnabled`, `routingIncludeDomains`, `routingExcludeDomains`
- `src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnServiceTest.kt` — тесты routing engine
- `src/android/app/src/test/kotlin/com/kvn/client/dns/FakeDnsResolverTest.kt` — новый файл
- `src/android/app/src/test/kotlin/com/kvn/client/dns/FakeIpPoolTest.kt` — новый файл

## Контекст

- Android VpnService не позволяет «исключить IP из маршрутизации» динамически — можно только указать CIDR при establish(). TUN = `0.0.0.0/0` означает, что ВЕСЬ трафик идёт через TUN. Чтобы трафик шёл напрямую, нужно в `tunReader()` детектить такие пакеты и отправлять их через `protect()` + raw socket, минуя WebSocket tunnel.
- Существующий `tunReader()` (KvnVpnService.kt:694) — dumb forwarder: читает IP пакет → шлёт как DATA frame через WebSocket.
- fakeDNS перехватывает DNS-запрос (UDP/53) **до** того, как он уходит в WebSocket tunnel, и **сам конструирует DNS-ответ** (с корректным ID, source IP = DNS-сервер, dest IP = приложение), записывая его обратно в TUN fd. Приложение получает ответ как от реального DNS-сервера — реальный DNS-запрос никогда не доходит до сети.
- Существующий `DnsParser` умеет парсить DNS wire format (extractQName, parseResponse).
- Существующий `DnsCache` — TTL-based LRU кеш.
- fake IP range `198.18.0.0/15` выбран как зарезервированный (RFC 3330) — не конфликтует с реальными IP.
- Сервер не меняется: получает IP пакет с уже реврайтнутым реальным dst IP.
- Параметр `routingDomainsEnabled` маппится на `dns_cache.enabled` из kvn-web: при включении `dns_cache.enabled` в web-интерфейсе в Android-конфигурацию передаётся `routingDomainsEnabled: true`.

## Зависимости

- `android-dns-cache` (archived): DnsParser и DnsCache уже реализованы — эта фича их использует.
- `android-per-app-dns` (archived): per-app DNS servers — fakeDNS резолвит exclude домены через указанные в конфигурации DNS серверы.
- Нет внешних библиотек: IP checksum и TCP/UDP pseudo-header checksum — до 20 строк кода каждая.

## Требования

- RQ-001 Android-клиент ДОЛЖЕН поддерживать конфигурацию `routingIncludeDomains` и `routingExcludeDomains` через ConnectionConfig.
- RQ-002 fakeDNS ДОЛЖЕН перехватывать UDP/53 пакеты из TUN, парсить DNS-запрос и извлекать домен.
- RQ-003 exclude matching ДОЛЖЕН работать по суффиксу: `.ru` матчит `mail.ru`, `2ip.ru`, но НЕ `google.com` и не `ru`.
- RQ-004 Для exclude доменов fakeDNS ДОЛЖЕН резолвить реальный IP через DNS серверы из конфигурации (с `protect()`) и возвращать реальный IP в DNS-ответе.
- RQ-005 Для exclude доменов routing engine ДОЛЖЕН доставлять пакеты на реальный IP напрямую (минуя VPN-сервер).
- RQ-006 Для include доменов fakeDNS ДОЛЖЕН возвращать fake IP из пула `198.18.0.0/15` и сохранять mapping fakeIP → домен.
- RQ-007 routing engine ДОЛЖЕН реврайтить dst IP с fake на реальный в IP-пакетах перед отправкой на сервер, с пересчётом IP checksum и TCP/UDP pseudo-header checksum.
- RQ-008 При дисконнекте VPN все кеши (fake IP pool, reverse mapping exclude IP, DNS cache) ДОЛЖНЫ очищаться.
- RQ-009 exclude/include CIDR и IP (routingExcludeRanges, routingIncludeRanges, routingExcludeIps, routingIncludeIps) ДОЛЖНЫ работать параллельно с domain routing: проверка идёт сначала по CIDR/IP, потом по domain mapping.
- RQ-010 Android-клиент ДОЛЖЕН поддерживать флаг `routingDomainsEnabled` в ConnectionConfig. При `false` fakeDNS не запускается, DNS-пакеты не перехватываются, трафик обрабатывается только по CIDR/IP правилам. Значение по умолчанию — `false`.
- RQ-011 Параметр `routingDomainsEnabled` ДОЛЖЕН маппиться на `dns_cache.enabled` из kvn-web: `dns_cache.enabled=true` → `routingDomainsEnabled=true`, `dns_cache.enabled=false` → `routingDomainsEnabled=false`.
- RQ-012 fakeDNS ДОЛЖЕН конструировать валидный DNS-ответ с корректным ID, QR/RA флагами и A-записью, и записывать его напрямую в TUN fd (source IP = DNS-сервер, dest IP = приложение).

## Вне scope

- wildcard/glob matching (`*.ozon.ru`, `ozon.*`) — только суффиксный match
- DoH/DoT interception — только UDP/53
- IPv6 fakeDNS — только IPv4
- Поддержка exclude/include доменов на Go-сервере (уже есть)
- Web UI для редактирования exclude/include domains (отдельная фича)
- Fake DNS для включения IPv6 адресов (AAAA записи возвращают пустой ответ или NXDOMAIN)

## Критерии приемки

### AC-001 exclude domain по суффиксу идёт напрямую

- Почему это важно: пользователь указывает `.ru` и ожидает, что `2ip.ru`, `mail.ru` пойдут напрямую.
- **Given** VPN запущен с `routingExcludeDomains: [".ru"]`
- **When** приложение делает DNS-запрос для `2ip.ru`
- **Then** fakeDNS возвращает реальный IP `2ip.ru`
- Evidence: DNS-запрос `2ip.ru` зарезолвлен через DNS серверы из конфигурации; fakeDNS сконструировал DNS-ответ и записал в TUN fd; приложение получило ответ с реальным IP; excludedIp set содержит этот IP; пакет на этот IP доставлен напрямую (не через WebSocket tunnel).

### AC-002 include domain по суффиксу идёт через VPN

- Почему это важно: пользователь указывает `.corp` и ожидает, что `internal.corp` пойдёт через VPN.
- **Given** VPN запущен с `routingIncludeDomains: [".corp"]` и `routingExcludeDomains: []`
- **When** приложение делает DNS-запрос для `internal.corp`
- **Then** fakeDNS возвращает fake IP из пула `198.18.0.0/15`
- Evidence: DNS-ответ с fake IP из пула `198.18.0.0/15` сконструирован fakeDNS и записан в TUN fd; пакет на этот fake IP реврайтнут в реальный IP `internal.corp` перед отправкой на сервер.

### AC-003 exclude CIDR/IP работают параллельно с domain routing

- Почему это важно: существующие CIDR/IP правила не должны ломаться.
- **Given** VPN запущен с `routingExcludeRanges: ["10.0.0.0/8"]`, `routingExcludeDomains: [".ru"]`
- **When** приложение шлёт пакет на `10.1.2.3:443`
- **Then** пакет доставляется напрямую, без проверки domain matching
- Evidence: routing engine сначала проверяет excludedIp set (включая CIDR/IP), доставка напрямую.

### AC-004 domain matching только по суффиксу (не по подстроке)

- Почему это важно: `.ru` не должен матчить `google.com` или `prudent.ruhr`.
- **Given** `routingExcludeDomains: [".ru"]`
- **When** приложение делает DNS-запрос для `google.com` и `prudent.ruhr`
- **Then** `google.com` НЕ матчится; `prudent.ruhr` матчится
- Evidence: `google.com` получает fake IP, `prudent.ruhr` получает реальный IP.

### AC-005 очистка кешей при дисконнекте

- Почему это важно: при переподключении старые mapping не должны влиять.
- **Given** VPN был запущен, fakeDNS закешировал маппинги
- **When** VPN дисконнектится
- **Then** DnsCache, FakeIpPool, excludedIp reverse cache очищаются
- Evidence: после реконнекта новый DNS-запрос для того же домена проходит полный цикл (не берётся из старого кеша).

### AC-006 правильный пересчёт checksum при rewrite dst IP

- Почему это важно: некорректный checksum приводит к сбросу TCP-соединений.
- **Given** VPN запущен, fakeDNS вернул fake IP для `google.com`
- **When** TCP-пакет с fake dst IP проходит через routing engine
- **Then** dst IP реврайтнут, IP header checksum и TCP pseudo-header checksum пересчитаны корректно
- Evidence: TCP-соединение успешно устанавливается (SYN → SYN-ACK), сервер получает пакет с правильным checksum.

### AC-007 прямой запрос на IP (без DNS) обрабатывается CIDR/IP правилами

- Почему это важно: `curl 1.2.3.4:443` не должен триггерить fakeDNS.
- **Given** VPN запущен с `routingExcludeIps: ["1.2.3.4"]`
- **When** приложение шлёт TCP SYN на `1.2.3.4:443`
- **Then** пакет доставляется напрямую
- Evidence: routing engine матчит `1.2.3.4` по exclude IP list, пакет не идёт через tunnel.

### AC-008 DNS запросы для не-matched доменов получают fake IP

- Почему это важно: весь трафик по умолчанию идёт через VPN (split-tunnel с исключениями).
- **Given** VPN запущен с `routingExcludeDomains: [".ru"]`
- **When** приложение делает DNS-запрос для `example.com`
- **Then** fakeDNS возвращает fake IP из пула `198.18.0.0/15`
- Evidence: DNS-ответ содержит fake IP из `198.18.0.0/15` (сконструирован fake DNS), пакет реврайтнут и отправлен на сервер.

### AC-009 `routingDomainsEnabled=false` отключает fakeDNS

- Почему это важно: пользователь должен иметь возможность отключить доменную маршрутизацию, оставив CIDR/IP routing.
- **Given** VPN запущен с `routingDomainsEnabled: false` и `routingExcludeDomains: [".ru"]`
- **When** приложение делает DNS-запрос для `2ip.ru`
- **Then** fakeDNS не перехватывает запрос, DNS-ответ не модифицируется, пакет на реальный IP `2ip.ru` идёт через VPN-туннель (как обычно)
- Evidence: DNS-пакет на UDP/53 не перехвачен fakeDNS (response не конструируется); запрос доходит до реального DNS-сервера через VPN; `2ip.ru` возвращает IP сервера VPN (не реальный IP клиента).

## Допущения

- Все DNS-запросы от приложений идут через TUN (UDP/53). Android VpnService с `addDnsServer()` гарантирует это.
- Приложения не используют DoH/DoT для обхода DNS.
- fake IP range `198.18.0.0/15` не пересекается с реальными адресами назначения.
- Сервер корректно обрабатывает IP-пакеты с любым dst IP после rewrite (сервер проксирует трафик, ему не важно, какой IP).

## Критерии успеха

- SC-001 exclude домен резолвится и пакет доставляется напрямую за <50ms дополнительной задержки (поверх DNS).
- SC-002 include домен с fake IP → rewrite → forward занимает <1ms на пакет (накладные расходы на lookup + rewrite + checksum).
- SC-003 Нет регрессии в throughput для трафика, не проходящего через fakeDNS (CIDR/IP routed).

## Краевые случаи

- DNS-запрос с несколькими вопросами (multi-question) — обрабатывается только первый QNAME.
- DNS-запрос с compression pointer в QNAME — DnsParser.extractQName останавливается на compression pointer, возвращает частичное имя. Если не удалось извлечь — пакет пропускается без обработки.
- DNS-запрос на AAAA запись — возвращается пустой ответ (fakeDNS не поддерживает IPv6).
- fake IP пул исчерпан (более 32768 одновременных доменов) — аллокатор возвращает ошибку, DNS-запрос пропускается без обработки (идёт через tunnel как есть).
- exclude домен резолвится в несколько IP — все IP добавляются в reverse cache.
- Переполнение reverse cache excludedIp — LRU eviction (по аналогии с DnsCache).
- TCP/UDP пакет без payload (pure ACK) — rewrite dst IP без пересчёта payload checksum (только IP checksum).
- VPN переподключение с новым списком exclude/include доменов — старые кеши очищены (AC-005), новые применяются.

## Открытые вопросы

1. Нужна ли поддержка wildcard (`*.ozon.ru`) в этом spec, или вынести в отдельную фичу?
2. Как быть с UDP-трафиком на fake IP (например, QUIC на порт 443)? TCP rewrite покрывает pseudo-header checksum, UDP — отдельный случай с UDP length dependency.
3. Нужен ли rate limiter на fakeDNS (чтобы не резолвить один и тот же домен 1000 раз в секунду)?
