# DNS-зависимая маршрутизация (wildcard exclude/include доменов)

## Scope Snapshot

- In scope: suffix/TLD matching (.ru, .ozon.ru) для exclude/include доменов в routing; DNS-интерсептор для TUN mode; proxy mode support.
- Out of scope: HTTP CONNECT host header parsing (кроме socks5), DoH/DoT interception, full DNAT.

## Цель

Пользователь может исключить из туннеля или направить в туннель все домены `.ru`, `.ozon.ru` и любые wildcard-суффиксы, и это работает и в TUN mode, и в proxy mode, без resolve миллионов доменов в IP.

## Основной сценарий

1. Пользователь указывает в `client.yaml`:
   ```yaml
   routing:
     exclude_domains:
       - .ru
       - .ozon.ru
   ```
2. Запрос к `hh.ru` (TUN) — DNS-запрос перехватывается, домен совпадает с `.ru` → DNS отвечается напрямую (non-tunnel), пакеты уходят напрямую
3. Запрос к `api.ozon.ru` (proxy) — socks5 стрим с `dst = api.ozon.ru:443` → suffix match `.ozon.ru` → direct
4. Запрос к `google.com` — не совпадает → идёт через туннель

## User Stories

- P1: Пользователь добавляет `.ru` в exclude_domains и все запросы к российским сайтам идут напрямую, остальные через VPN
- P2: Работает и в TUN mode (через перехват DNS на TUN) и в proxy mode (через проверку домена из стрима)
- P3: Обратная совместимость — точные домены (`example.com`) продолжают работать как раньше (через DNS resolve + IP match)

## MVP Slice

1. Suffix/TLD matching в `DomainMatcher` — `.ru`, `.ozon.ru` матчатся без DNS lookup
2. DNS-интерсептор на TUN для TUN mode
3. Прямая проверка домена из `dst` в proxy mode

## First Deployable Outcome

`exclude_domains: [.ru, .ozon.ru]` → `curl hh.ru` идёт напрямую, `curl google.com` идёт через туннель.

## Scope

- `src/internal/routing/domain_matcher.go` — suffix matching без DNS lookup
- `src/internal/routing/rule_set.go` — proxy mode domain check
- `src/internal/proxy/stream.go` — передача домена в RuleSet
- `src/internal/tun/` — DNS-интерсептор (UDP 53)
- `src/internal/bootstrap/client/tun.go` — интеграция DNS-интерсептора
- `src/internal/bootstrap/client/proxy.go` — интеграция доменной проверки
- `src/internal/config/client.go` — домены с точкой-префиксом как suffix

## Выбор реализации

### DNS-интерсептор для TUN mode

TUN mode не видит домены — только IP-пакеты. DNS-запрос (UDP 53, A/AAAA) идёт через TUN-интерфейс обычным IP-пакетом.

**Алгоритм:**
1. TUN read loop проверяет: UDP, dst port 53 или src port 53
2. Если DNS query (dst:53) — парсим question section, извлекаем QNAME (домен)
3. Проверяем QNAME по `exclude_domains` / `include_domains` с suffix matching
4. Если exclude — DNS не идёт через туннель (resolv напрямую или synthetic NXDOMAIN для include)
5. Если include / no match — пакет идёт через туннель как обычно

**Важно:** DNS-ответ тоже может быть перехвачен, если exclude — мы не хотим, чтобы сервер DNS отвечал через туннель с внутренним IP.

**Упрощение для MVP:** DNS-пакет для exclude domain просто НЕ отправляется в туннель — он отправляется напрямую системному DNS-резолверу. Клиентский DNS (системный) получит ответ напрямую, а приложение — реальный IP.

### Proxy mode

`proxy.Stream.Dst` уже содержит домен (`hh.ru:443`). В `proxy.go` при проверке `routeSet.Route(nip)` — если nip невалидный или домен не разрешён, проверяем `routeSet.MatchDomain(host)`.

## Требования

- RQ-001 Система ДОЛЖНА поддерживать suffix-домены с префиксом `.` (`.ru`, `.ozon.ru`) для exclude/include
- RQ-002 Точные домены без точки (`example.com`) ДОЛЖНЫ работать по старому (DNS resolve + IP match)
- RQ-003 Домены с точкой-префиксом НЕ ДОЛЖНЫ резолвиться через DNS
- RQ-004 TUN mode ДОЛЖЕН перехватывать DNS (UDP 53) на входе и проверять домен
- RQ-005 Proxy mode ДОЛЖЕН проверять домен из `dst` стрима без DNS
- RQ-006 Обратная совместимость: конфиги без suffix-доменов работают как раньше

## Критерии приемки

### AC-001 Suffix matching .ru

- **Given** `exclude_domains: [.ru]`
- **When** запрос к `hh.ru` (TUN или proxy)
- **Then** трафик идёт напрямую
- Evidence: `curl hh.ru` → tcpdump не показывает туннельный трафик к этому IP

### AC-002 Suffix matching .ozon.ru

- **Given** `exclude_domains: [.ozon.ru]`
- **When** запрос к `api.ozon.ru`
- **Then** трафик идёт напрямую
- Evidence: `curl api.ozon.ru` → direct

### AC-003 Точный домен без точки совместим

- **Given** `exclude_domains: [example.com]`
- **When** запрос к `example.com`
- **Then** старое поведение (resolve + IP match)
- Evidence: работает как раньше

### AC-060 Proxy mode

- **Given** `exclude_domains: [.ru]` в proxy mode
- **When** `curl -x socks5h://... hh.ru`
- **Then** трафик идёт напрямую
- Evidence: `curl -x socks5h://... hh.ru` → direct

### AC-005 No match = tunnel

- **Given** `exclude_domains: [.ru]`
- **When** запрос к `google.com`
- **Then** трафик идёт через туннель
- Evidence: `curl google.com` → tunnel

## Допущения

- DNS-запросы только UDP/53 (не TCP, не DoH/DoT)
- Only A/AAAA question types
- Только первый question в DNS-запросе проверяется
- Suffix matching: `.ru` матчит `hh.ru`, `mail.ru`, `sub.hh.ru` — любой домен, оканчивающийся на `.ru`

## Критерии успеха

- SC-001 exclude_domains с TLD (.ru, .com) работают без DNS lookup на клиенте

## Краевые случаи

- `exclude_domains: [.ru]` + точное совпадение `ru` — `.ru` суффикс не должен матчить голое `ru` (без точки)
- `exclude_domains: [ru]` без точки — старый режим (resolve + IP)
- DNS-запрос для .onion (нерезолвимый) — корректно не создаёт ошибку, просто не матчится

## Открытые вопросы

- Нужен ли `dns_intercept: true/false` в конфиге, или включать автоматически при наличии suffix-доменов?
- Как обрабатывать CNAME-цепочки? DNS-ответ может содержать CNAME на другой домен — надо ли рекурсивно проверять?
- EDNS0/ECS — влияет на DNS-ответ; при прямом resolve IP может быть ближайший к серверу, а не к клиенту
- IPv6 DNS (AAAA) — тоже перехватывать?
