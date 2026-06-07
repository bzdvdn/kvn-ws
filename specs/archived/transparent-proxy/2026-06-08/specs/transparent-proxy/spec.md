# Прозрачный прокси (transparent proxy)

## Scope Snapshot

- In scope: система настраивает iptables/nftables (Linux) или pf (macOS) для перенаправления всего TCP-трафика через локальный KVN proxy без настройки приложений; работает в Docker контейнере (bridge и host mode).
- Out of scope: UDP transparent proxy (TPROXY), модификация существующего SOCKS5/HTTP CONNECT хендлера, GUI-переключатель.

## Цель

Пользователь включает «Transparent proxy» — и весь TCP-трафик системы автоматически уходит через KVN, без установки HTTP_PROXY env vars, без настройки браузера, без SwitchyOmega. Фича считается успешной, если после включения `curl -v http://example.com` и `curl -v https://example.com` проходят через прокси без каких-либо переменных окружения или конфигурации приложения, а лог показывает "transparent proxy active" и перехваченные соединения. Фича также должна работать внутри Docker контейнера (с `--cap-add=NET_ADMIN`).

## Основной сценарий

1. Пользователь запускает kvn-client в proxy mode с флагом `--transparent` или через конфиг `transparent: true`.
2. Клиент проверяет права (root/sudo/CAP_NET_ADMIN) и платформу, затем устанавливает правила фаервола: на Linux — iptables REDIRECT или nftables, на macOS — pf anchor.
3. Все TCP-соединения, не попадающие в исключения (локальные сети, excluded CIDR/domain), перенаправляются на локальный proxy listener.
4. Proxy listener обнаруживает, что соединение пришло через transparent redirect (по наличию исходного адреса назначения из SO_ORIGINAL_DST / pf rdr), и обрабатывает его как обычный proxy stream к оригинальному хосту:порту.
5. DNS-запросы обрабатываются внутри клиента: клиент поднимает встроенный DNS-прокси (на 127.0.0.54:53 или через перехват `/etc/resolv.conf`), который резолвит через KVN tunnel. Это позволяет обрабатывать DNS без TPROXY/UDP-перехвата.
6. При остановке клиента правила фаервола удаляются, DNS-прокси отключается, трафик возвращается к прямой маршрутизации.
7. Docker: контейнер запускается с `--cap-add=NET_ADMIN` (bridge или host mode). Клиент внутри контейнера поднимает iptables правила в своём network namespace; все исходящие TCP-пакеты из контейнера (включая проброшенные порты в bridge mode) идут через прокси.

## User Stories

- **P1 (MVP)**: Linux transparent proxy через iptables REDIRECT (TCP only) + встроенный DNS-proxy. Пользователь включает — весь HTTP/HTTPS трафик идёт через KVN, DNS резолвится через туннель.
- **P2**: macOS transparent proxy через pf anchor + DNS-proxy.
- **P3**: Docker-совместимость: контейнер с `--cap-add=NET_ADMIN` работает с transparent proxy (bridge + host mode) без дополнительных флагов.
- **P4**: TUN mode в Docker — если пользователь запускает kvn-client в TUN mode (а не proxy), трафик уже идёт через TUN-устройство; transparent proxy для TUN не требуется.

## MVP Slice

- P1 + P3 (Linux iptables REDIRECT + DNS-proxy + Docker bridge/host). Закрывает AC-001, AC-002, AC-003, AC-005, AC-006, AC-009.

## First Deployable Outcome

- После первого implementation pass можно запустить `kvn-client --mode proxy --transparent` на Linux (хосте или Docker), и `curl google.com` проходит через прокси без HTTP_PROXY. Лог показывает "transparent proxy: intercepted <ip>:<port>".

## Scope

1. Управление iptables/nftables правилами (установка, проверка, удаление) — пакет `internal/transparent/iptables`.
2. Управление pf anchor на macOS — пакет `internal/transparent/pf`.
3. Обнаружение transparent-соединений в proxy listener — извлечение `SO_ORIGINAL_DST` / pf rdr address.
4. Интеграция с `runProxySession`: при включённом transparent режиме proxy listener слушает на `0.0.0.0:2310` вместо `127.0.0.1:2310` и распознаёт transparent-соединения.
5. Exclude rules: локальные сети, CIDR, домены из конфига маршрутизации не перенаправляются (записываются в исключения iptables/pf).
6. Graceful cleanup: сигнал SIGTERM/SIGINT снимает правила.
7. Встроенный DNS-proxy: клиент поднимает локальный DNS-резольвер (на `127.0.0.54:53`), который резолвит запросы через KVN tunnel. DNS-proxy не требует UDP-перехвата — `/etc/resolv.conf` перенаправляется на локальный DNS. Для excluded доменов запрос резолвится локально через оригинальные nameserver-ы.
8. Docker-адаптация: обнаружение работы внутри контейнера (`/.dockerenv`), корректная работа с `iptables-legacy` vs `iptables-nft`, поддержка bridge mode (без `--network host`).

## Контекст

- Требуются root-привилегии (или `sudo`, или `CAP_NET_ADMIN` в Docker). Клиент ДОЛЖЕН явно проверять и логировать ошибку, если прав недостаточно.
- REDIRECT не поддерживает UDP — это осознанное ограничение. DNS решается не через UDP-перехват, а через встроенный DNS-proxy (сервер на локальном порту 53, клиент перенаправляет `/etc/resolv.conf` на него).
- Для получения оригинального адреса назначения используется `getsockopt(IP_ORIGINAL_DST)` на Linux (для REDIRECT) или pf `rdr` на macOS.
- Существующий proxy listener (`internal/proxy/listener.go`) принимает TCP-соединения и определяет тип (SOCKS5 / HTTP CONNECT). Transparent-соединения будут третьим типом, определяемым по неудаче распознавания SOCKS/HTTP — тогда читается оригинальный dst из сокета.
- НЕ ДОЛЖЕН нарушаться существующий SOCKS5/HTTP CONNECT приём при включённом transparent mode.
- Docker bridge mode: iptables внутри контейнера работают, `--network host` не требуется. REDIRECT в bridge mode перенаправляет трафик на localhost внутри контейнера.
- TUN mode — это альтернатива transparent proxy. В TUN mode клиент уже перехватывает весь IP-трафик через TUN-устройство; transparent proxy относится только к proxy mode.

## Требования

- RQ-001 Система ДОЛЖНА настраивать iptables REDIRECT для TCP портов 80 и 443 (и опционально всех остальных) на локальный proxy порт при включении transparent proxy на Linux.
- RQ-002 Система ДОЛЖНА извлекать оригинальный адрес назначения из transparent-соединения через `SO_ORIGINAL_DST` (Linux) или pf metadata (macOS).
- RQ-003 Система ДОЛЖНА удалять все установленные правила фаервола при штатном завершении (SIGTERM/SIGINT).
- RQ-004 Система ДОЛЖНА проверять права root/CAP_NET_ADMIN перед установкой правил и логировать понятную ошибку при их отсутствии.
- RQ-005 Система ДОЛЖНА поддерживать exclude-правила: не перенаправлять трафик в локальные сети (RFC1918) и CIDR/домены из конфига маршрутизации.
- RQ-006 Система ДОЛЖНА корректно работать внутри Docker контейнера с `--cap-add=NET_ADMIN` (bridge и host mode).
- RQ-007 Система ДОЛЖНА на macOS настраивать pf anchor для перенаправления TCP трафика на локальный proxy порт.
- RQ-008 Прокси listener ДОЛЖЕН определять, пришло ли соединение через transparent redirect, и в этом случае использовать оригинальный dst вместо ожидания SOCKS5/HTTP CONNECT.
- RQ-009 Система ДОЛЖНА запускать встроенный DNS-proxy на `127.0.0.54:53` (по умолчанию, конфигурируется через `dns_proxy.listen`) при активации transparent proxy и перенаправлять DNS-запросы через KVN tunnel.
- RQ-010 Система ДОЛЖНА при активации transparent proxy перезаписывать `/etc/resolv.conf` на `nameserver <dns_proxy.listen>` (с сохранением оригинального для восстановления).
- RQ-011 Система ДОЛЖНА корректно обрабатывать `getsockopt(SO_ORIGINAL_DST)` — `syscall.Errno(0)` не должен интерпретироваться как ошибка.
- RQ-012 Система ДОЛЖНА для excluded доменов резолвить DNS локально (через оригинальные nameserver-ы), а не через KVN tunnel.

## Вне scope

- UDP transparent proxy (TPROXY) — отложено на будущие версии. DNS решается через встроенный DNS-proxy, а не через UDP-перехват.
- GUI-переключатель transparent proxy в webui — только CLI/конфиг.
- HA/кластеризация правил фаервола.
- Поддержка IPv6 transparent proxy в MVP (только IPv4).
- Поддержка Android/iOS (только Linux Desktop/Server, Docker, macOS).
- Transparent proxy для TUN mode — TUN уже перехватывает весь трафик на уровне ядра.

## Критерии приемки

### AC-001 Linux transparent proxy активируется и деактивируется

- Почему это важно: пользователь включает фичу, трафик идёт через прокси; выключает — всё возвращается.
- **Given** kvn-client запущен в proxy mode с `transparent: true` на Linux с root-правами
- **When** клиент завершил инициализацию
- **Then** `iptables -t nat -L PREROUTING` показывает REDIRECT правило на порт клиента; после остановки клиента правило отсутствует
- Evidence: вывод `iptables -t nat -L` до/после

### AC-002 HTTP-запрос проходит через transparent proxy

- Почему это важно: базовый сценарий работы — HTTP без env vars.
- **Given** transparent proxy активен на Linux
- **When** выполняется `curl -v http://httpbin.org/get`
- **Then** запрос успешно выполняется, в логе клиента присутствует "transparent proxy: intercepted"
- Evidence: успешный HTTP ответ + запись в логе

### AC-003 HTTPS-запрос проходит через transparent proxy

- Почему это важно: HTTPS — основной протокол современного веба.
- **Given** transparent proxy активен на Linux
- **When** выполняется `curl -v https://example.com`
- **Then** запрос успешно выполняется (SSL handshake через proxy), в логе "transparent proxy: intercepted"
- Evidence: успешный HTTPS ответ + запись в логе

### AC-004 Исключения (exclude CIDR/домены) не перенаправляются

- Почему это важно: локальный трафик и доверенные адреса не должны идти через прокси.
- **Given** transparent proxy активен, в exclude_rules указан `10.0.0.0/8`
- **When** выполняется `curl -v http://10.0.0.1`
- **Then** соединение НЕ попадает в прокси (в логе нет "transparent proxy: intercepted"), curl получает ответ напрямую (или timeout — в зависимости от существования хоста)
- Evidence: отсутствие записи в логе прокси

### AC-005 Правила снимаются при SIGTERM

- Почему это важно: защита от «забытых» правил после падения/остановки клиента.
- **Given** transparent proxy активен
- **When** клиент получает SIGTERM
- **Then** после завершения клиента `iptables -t nat -L PREROUTING` не показывает KVN-правил
- Evidence: вывод iptables после остановки

### AC-006 Transparent proxy работает в Docker контейнере (bridge mode)

- Почему это важно: поставка через Docker multi-stage — основной способ, bridge mode — режим по умолчанию.
- **Given** Docker контейнер запущен с `--cap-add=NET_ADMIN` (без `--network host`)
- **When** внутри контейнера выполняется curl к внешнему хосту
- **Then** запрос проходит через прокси, правила iptables установлены внутри контейнера
- Evidence: лог "transparent proxy active" + успешный ответ curl

### AC-009 DNS-запросы резолвятся через KVN tunnel при transparent proxy

- Почему это важно: DNS должен работать через прокси, чтобы не было DNS-утечек.
- **Given** transparent proxy активен, DNS-proxy включён
- **When** выполняется `dig google.com` или `getent hosts google.com`
- **Then** DNS-запрос резолвится через KVN tunnel, в логе клиента присутствует запись о DNS-запросе
- Evidence: успешный ответ dig + запись в логе

### AC-007 macOS pf anchor активируется и деактивируется

- Почему это важно: поддержка macOS для пользователей не-Linux.
- **Given** kvn-client запущен на macOS с `transparent: true` с root-правами
- **When** клиент завершил инициализацию
- **Then** `pfctl -s ancor kvn` показывает anchor правила; после остановки правила отсутствуют
- Evidence: вывод `pfctl -s anchor` до/после

### AC-008 Без root-прав клиент логирует ошибку и не включает transparent proxy

- Почему это важно: понятная обратная связь вместо таинственного неработающего прокси.
- **Given** kvn-client запущен без root/CAP_NET_ADMIN с `transparent: true`
- **When** клиент пытается активировать transparent proxy
- **Then** клиент логирует "transparent proxy requires root privileges, skipping" и продолжает работу в обычном proxy mode
- Evidence: строка в логе с warning

### AC-010 SO_ORIGINAL_DST корректно обрабатывает errno 0

- Почему это важно: без этого transparent mode полностью не работает (все соединения падают с RST).
- **Given** kvn-client запущен с `transparent: true`
- **When** transparent-соединение принимается listener-ом
- **Then** `getOriginalDst` возвращает корректный оригинальный адрес, а не ошибку `errno 0`
- Evidence: лог "transparent dst=ip:port" вместо "getOriginalDst failed"

### AC-011 Exclude_domains резолвятся локально в transparent mode

- Почему это важно: корпоративные домены должны резолвиться через локальный DNS (напр. openfortivpn), а не через туннель.
- **Given** transparent proxy активен, в `exclude_domains` указан `.corp.ru`
- **When** выполняется `nslookup server.corp.ru 127.0.0.54`
- **Then** DNS-запрос резолвится локально (через оригинальные nameserver-ы из /etc/resolv.conf), а не через туннель
- Evidence: ответ содержит IP из корпоративной сети, лог DNS proxy показывает локальный резолв

## Допущения

- На Linux используется `iptables-legacy` или `iptables-nft` — клиент проверяет `iptables --version` и выбирает подходящий.
- Docker bridge mode: iptables внутри контейнера могут REDIRECT на localhost. Правила применяются внутри network namespace контейнера, `--network host` не требуется.
- На macOS `pfctl` доступен и пользователь имеет права sudo.
- Proxy listener стартует на `0.0.0.0:2310` при transparent mode (не только на loopback).
- Исходный код пакета `internal/proxy/listener.go` расширяется без рефакторинга существующей логики SOCKS5/HTTP CONNECT.
- DNS-proxy слушает на `127.0.0.54:53` (по умолчанию, конфигурируется через `dns_proxy.listen`), клиент перезаписывает `/etc/resolv.conf` при старте и восстанавливает при остановке. В Docker контейнере DNS-proxy настраивается аналогично.
- TUN mode не использует transparent proxy — весь трафик уже идёт через TUN-устройство на уровне ядра; фича относится только к proxy mode.

## Критерии успеха

- SC-001 Время установки/снятия правил iptables <100ms.
- SC-002 Overhead transparent proxy против обычного SOCKS5: <5% по latency.
- SC-003 Весь трафик (HTTP + HTTPS) основных use-кейсов проходит без ошибок: `curl`, `wget`, `apt`, `git clone`.

## Краевые случаи

- Клиент запущен без root → transparent proxy логирует ошибку и не включается, proxy mode продолжает работу.
- Docker контейнер без `--cap-add=NET_ADMIN` → то же поведение.
- iptables отсутствует в системе → клиент логирует ошибку.
- Двойной запуск клиента с transparent → второй экземпляр проверяет наличие правил и перезаписывает.
- Сетевой интерфейс поднимается/опускается после установки правил → iptables правила сохраняются (они не привязаны к интерфейсу).
- pf anchor не удалился при краше клиента → CLI флаг `--transparent-cleanup` для ручного снятия.

## Domain-based DNS routing (AC-011)

- **Проблема**: exclude_domains из RoutingCfg не работали в transparent mode, потому что destination приходит как IP от SO_ORIGINAL_DST, а MatchDomain() ожидает доменное имя.
- **Решение**: DNS proxy теперь проверяет домен из DNS-запроса через RouteSet.MatchDomain(). Если домен совпадает с exclude → запрос резолвится ЛОКАЛЬНО через оригинальные nameserver-ы (сохранённые из `/etc/resolv.conf` до оверрайда). Если не exclude → шлётся через туннель как раньше.
- **Ограничение**: для TCP-трафика доменный exclude всё ещё требует CIDR в `exclude_ranges`, т.к. iptables REDIRECT работает на уровне IP, а не доменов. DNS-proxy возвращает реальный IP, и если CIDR корпоративной сети добавлен в exclude_ranges, TCP-соединение не редиректится.

## SO_ORIGINAL_DST errno fix

- **Проблема**: `getOriginalDst` всегда падала с `errno 0` — классическая Go ошибка: `syscall.Errno(0)` при присваивании в `error`-интерфейс становится non-nil, поэтому `if opErr != nil` срабатывал даже при успешном вызове `getsockopt`.
- **Решение**: замена `var opErr error` → `var errno syscall.Errno` и проверка `errno != 0` по значению, а не по nil-ности интерфейса.

## Открытые вопросы

- Должен ли transparent proxy перенаправлять трафик на `0.0.0.0:2310` или отдельный порт? — На текущий момент `0.0.0.0:2310`, тот же порт, что и обычный proxy, listener отличает transparent от обычных по наличию `SO_ORIGINAL_DST`.
- Нужна ли опция `--transparent-port` для отдельного порта? — Пока нет, listener различает сам.
- DNS-proxy: реализовывать как отдельный простой UDP DNS-сервер внутри клиента или использовать стороннюю библиотеку? — На MVP простой forwarder (получил запрос → отправил через KVN tunnel → вернул ответ).
- Как быть с TUN mode в Docker? — TUN mode уже маршрутизирует трафик через TUN-устройство. Transparent proxy — альтернатива для proxy mode. Документировать различие.
