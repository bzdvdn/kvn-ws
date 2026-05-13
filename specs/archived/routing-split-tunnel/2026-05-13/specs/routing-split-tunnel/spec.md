# Routing & Split Tunnel

## Scope Snapshot

- In scope: клиент KVN-WS маршрутизирует IP-пакеты: весь трафик через туннель (server), напрямую (direct), или выборочно (split tunnel) по CIDR, IP, доменам с ordered rules engine и DNS resolver.
- Out of scope: балансировка, многосерверная маршрутизация, межсетевые экраны на стороне сервера кроме MASQUERADE.

## Цель

Администратор получает возможность тонко управлять маршрутизацией трафика клиента: пускать корпоративные ресурсы через VPN-туннель, а остальной трафик (YouTube, соцсети) — напрямую, без потери производительности. Успех измеряется `dig` + `curl --resolve`, подтверждающими маршрутизацию YouTube напрямую, а корпоративных ресурсов — через туннель.

## Основной сценарий

1. Клиент запускается, читает конфиг с routing-секцией (default: server, и списки include/exclude).
2. При поступлении исходящего IP-пакета правила применяются по порядку: exclude → include, first match wins.
3. Для доменных правил клиент предварительно разрешает домен через встроенный DNS resolver (кеш, TTL), затем применяет IP-правило.
4. Пакеты, попавшие под `server`, шифруются и отправляются через WebSocket-туннель; сервер выполняет NAT (MASQUERADE) и выпускает трафик в интернет.
5. Пакеты, попавшие под `direct`, отправляются напрямую минуя туннель.
6. Если ни одно правило не сработало — применяется default_route.
7. В full-tunnel режиме (default_route: server, без exclude-правил) DNS-запросы клиента форвардятся через туннель (DNS override), чтобы избежать DNS-leak.

## User Stories

- P1 Администратор настраивает конфиг: default_route: direct + exclude_ranges: [10.0.0.0/8, 172.16.0.0/12] — и убеждается, что корпоративные IP идут через туннель.

- P2 Администратор добавляет include_domains: [internal.corp.ru] — клиент разрешает домен в IP и маршрутизирует через туннель, остальной трафик идёт напрямую.

## MVP Slice

default_route: server/direct + CIDR split tunnel + ordered rules engine. Закрывает AC-001, AC-002, AC-006, AC-009.

## First Deployable Outcome

Конфиг клиента с routing-секцией, при запуске пакеты уходят либо в tun, либо напрямую согласно правилам. На сервере работает MASQUERADE. Проверка `curl --resolve` на external IP.

## Scope

- Пакет `src/internal/routing/` — rules engine, matchers
- Пакет `src/internal/nat/` — MASQUERADE (nftables/iptables)
- Пакет `src/internal/config/` — routing-секция в ClientConfig
- Пакет `src/internal/dns/` — DNS resolver с кешем и TTL
- Конфигурация `configs/client.yaml` — routing-блок

## Контекст

- TUN-устройство уже читает/пишет пакеты через `src/internal/tun/`
- Пакеты — сырые IP-датаграммы (AF_INET), IPv6 пока вне scope
- Сервер работает на Linux с nftables
- Клиент может работать на Linux, macOS, Windows (для MASQUERADE — только Linux)
- BoltDB/SQLite для persistence не требуются для маршрутизации

## Требования

- RQ-001 Клиент ДОЛЖЕН поддерживать default_route: server | direct.
- RQ-002 Клиент ДОЛЖЕН применять exclude-правила перед include-правилами (exclude→include→default).
- RQ-003 Система ДОЛЖНА поддерживать списки CIDR: include_ranges, exclude_ranges.
- RQ-004 Система ДОЛЖНА поддерживать списки отдельных IP: include_ips, exclude_ips.
- RQ-005 Система ДОЛЖНА разрешать домены через встроенный DNS resolver с кешем и уважением TTL.
- RQ-006 Система ДОЛЖНА поддерживать списки доменов: include_domains, exclude_domains.
- RQ-007 Сервер ДОЛЖЕН выполнять MASQUERADE (nftables/iptables) для пакетов, пришедших через туннель.
- RQ-008 В full-tunnel режиме DNS-запросы клиента ДОЛЖНЫ форвардиться через туннель (DNS override).
- RQ-009 Система ДОЛЖНА логировать применённое правило для каждого пакета (debug-level).

## Вне scope

- Балансировка между несколькими серверами
- IPv6-маршрутизация
- Policy-based routing (по процессам/пользователям)
- Firewall-правила на клиенте (например, блокировка портов)
- Proxy auto-config (PAC)
- Поддержка Windows NAT (WinNAT)

## Критерии приемки

### AC-001 Default route mode

- Почему это важно: базовый выбор — весь трафик через VPN или напрямую.
- **Given** запущенный клиент с конфигом `routing.default_route: server`
- **When** клиент отправляет пакет на 8.8.8.8
- **Then** пакет идёт через WebSocket-туннель
- Evidence: на сервере `tcpdump -i tun0` видит пакет от IP клиента; `dig +short myip.opendns.com @resolver1.opendns.com` возвращает IP сервера.

### AC-002 Split tunnel по CIDR

- Почему это важно: администратор выбирает диапазоны для туннеля.
- **Given** конфиг: `default_route: direct`, `exclude_ranges: []`, `include_ranges: [10.0.0.0/8]`
- **When** клиент шлёт пакет на 10.10.10.10
- **Then** пакет идёт через туннель
- **When** клиент шлёт пакет на 8.8.8.8
- **Then** пакет идёт напрямую (default_route: direct)
- Evidence: `curl --resolve example.com:80:10.10.10.10 http://example.com` проходит; `curl --resolve example.com:80:8.8.8.8 http://example.com` идёт мимо туннеля.

### AC-003 Routing по отдельным IP

- Почему это важно: исключить/включить конкретный хост без CIDR.
- **Given** `include_ips: [192.168.1.100]`, `default_route: direct`
- **When** пакет на 192.168.1.100
- **Then** пакет через туннель
- **When** пакет на 192.168.1.101
- **Then** пакет напрямую
- Evidence: лог маршрутизации показывает `route=server` для 192.168.1.100.

### AC-004 DNS resolver с кешем и TTL

- Почему это важно: доменные правила требуют разрешения в IP; без кеша каждый пакет вызывает DNS.
- **Given** домен internal.corp.ru с TTL=300
- **When** клиент разрешает internal.corp.ru
- **Then** результат кешируется на 300с
- **When** повторный запрос в течение 300с
- **Then** возвращается кешированный IP
- **When** после 300с
- **Then** DNS запрашивается заново
- Evidence: тест с fake DNS-сервером проверяет количество запросов.

### AC-005 Routing по доменам

- Почему это важно: администратор указывает домены вместо IP.
- **Given** `include_domains: [internal.corp.ru]`, `default_route: direct`
- **When** клиент разрешает internal.corp.ru → 10.10.10.10 и шлёт пакет
- **Then** пакет на 10.10.10.10 через туннель
- **When** пакет на 8.8.8.8
- **Then** пакет напрямую
- Evidence: `curl --resolve internal.corp.ru:80:10.10.10.10 http://internal.corp.ru` идёт через туннель.

### AC-006 Ordered rules engine

- Почему это важно: exclude должен выигрывать у include; порядок предсказуем.
- **Given** `exclude_ips: [10.10.10.10]`, `include_ranges: [10.0.0.0/8]`, `default_route: direct`
- **When** пакет на 10.10.10.10
- **Then** exclude срабатывает первым → пакет напрямую (direct)
- **When** пакет на 10.10.10.11
- **Then** exclude не сработал, include сработал → через туннель
- Evidence: лог показывает matched rule exclude_ip(10.10.10.10) = direct, include_range(10.0.0.0/8) = server.

### AC-007 Server-side NAT (MASQUERADE)

- Почему это важно: пакеты клиента должны выходить в интернет с IP сервера.
- **Given** сервер с nftables, клиент подключён через туннель
- **When** клиент шлёт пакет на внешний IP через туннель
- **Then** сервер выполняет MASQUERADE, пакет выходит с IP сервера
- **When** ответ возвращается
- **Then** сервер де-MASQUERADE'ит и шлёт обратно в туннель
- Evidence: на сервере `nft list ruleset` показывает postrouting masquerade правило.

### AC-008 DNS override для full-tunnel

- Почему это важно: в full-tunnel режиме DNS не должен утекать на сторону.
- **Given** `default_route: server`, без exclude-правил
- **When** клиент делает DNS-запрос на system resolver
- **Then** DNS-запрос форвардится через туннель на DNS-сервер (например, 1.1.1.1), настроенный на сервере
- Evidence: `tcpdump` на клиенте не показывает DNS-пакеты вне туннеля: все UDP 53 идут через tun0.

### AC-009 Конфигурация в client.yaml

- Почему это важно: админ описывает маршрутизацию декларативно.
- **Given** файл client.yaml с routing-секцией
- **When** клиент загружает конфиг
- **Then** routing-секция парсится в ClientConfig без ошибок
- **When** routing-секция отсутствует
- **Then** default_route: server по умолчанию
- Evidence: `LoadClientConfig("configs/client.yaml")` возвращает корректно заполненный ClientConfig.

### AC-010 Gate: YouTube напрямую, корп. ресурсы через туннель

- Почему это важно: интеграционный тест всего split tunnel.
- **Given** конфиг: `default_route: direct`, `include_domains: [corp.example.com]`, `include_ranges: [10.0.0.0/8]`
- **When** `dig +short youtube.com` и `curl --resolve youtube.com:80:$(dig +short youtube.com | head -1) http://youtube.com`
- **Then** пакеты идут напрямую (default_route: direct)
- **When** `dig +short corp.example.com` и `curl --resolve corp.example.com:80:$(dig +short corp.example.com) http://corp.example.com`
- **Then** пакеты идут через туннель
- Evidence: логи клиента показывают `matched_rule=default(direct)` для youtube, `matched_rule=include_domain(corp.example.com→server)` для corp.

## Допущения

- TUN-устройство уже реализовано и читает/пишет IP-пакеты (задача 1.x).
- Сервер работает на Linux с nftables (iptables-legacy не требуется).
- DNS resolver клиента использует стандартный системный резолвер (/etc/resolv.conf) или явно указанный в конфиге.
- Для MASQUERADE на сервере требуются root-привилегии.
- Все exclude-правила проверяются раньше include-правил (exclude → include → default).
- Default route `direct` означает полный bypass туннеля — пакет идёт через native сетевой стек.
- Кеш DNS — in-memory, без персистентности между перезапусками.

## Критерии успеха

- AC-001–AC-010 проходят в CI.
- Split tunnel gate (AC-010) проверяется вручную в demo-окружении.
- Задержка на принятие routing-решения < 1µs на пакет (benchmark).

## Краевые случаи

- Пустой список include_ranges + exclude_ranges: используется только default_route.
- Домен разрешается в несколько IP: все IP домена получают одинаковое правило.
- TTL=0: DNS не кешируется, каждый раз запрос.
- CIDR с префиксом /0: эквивалентно default_route server/direct соответственно.
- Одновременное совпадение exclude и include: exclude выигрывает.
- DNS resolve timeout: пакет направляется по default_route, ошибка логируется.
- Сервер без nftables: MASQUERADE не применяется, ошибка при старте.

## Открытые вопросы

- Нужна ли поддержка iptables-legacy на сервере?
  - Пока только nftables; если понадобится legacy — расширяем в отдельной задаче.
- Должен ли клиент поддерживать DNS-over-HTTPS/TLS для резолвинга?
  - Нет, использует стандартный системный резолвер.
- Нужен ли health-check для DNS resolver (падение upstream)?
  - Нет на данном этапе; timeout обрабатывается как fallback на default_route.
