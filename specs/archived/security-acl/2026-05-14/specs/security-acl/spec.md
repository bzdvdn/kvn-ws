# Security & ACL — защита сервера и управление доступом

## Scope Snapshot

- In scope: CIDR ACL, per-token bandwidth/session limits, Origin/Referer validation, Admin API (CLI over HTTP), mTLS (optional)
- Out of scope: rate limiting (глобальное, уже реализовано), токенов на основе JWT/OAuth2, RBAC/роли, Web Application Firewall, DDoS-защита выше уровня сессии

## Цель

Оператор VPN-сервера получает инструменты для контроля доступа: фильтрация по подсетям источника, ограничение bandwidth и сессий на токен, защита от CSWSH-атак через Origin/Referer, административный API для мониторинга и управления активными сессиями, а также опциональная mTLS-аутентификация клиентов. Успех определяется прохождением security audit checklist (OWASP top-10 для WebSocket).

## Основной сценарий

1. Сервер запускается с конфигурацией, содержащей списки allow/deny CIDR, per-token лимиты, max_sessions, Origin/Referer whitelist и опционально CA для mTLS.
2. Входящее TCP-соединение проходит CIDR-фильтрацию по RemoteAddr; соединения из deny-подсетей отклоняются до TLS-handshake.
3. HTTP-запрос на /tunnel проверяется на Origin/Referer; невалидные запросы получают 403.
4. После WebSocket-апгрейда и аутентификации по токену сервер проверяет max_sessions для этого токена и применяет bandwidth quota.
5. Оператор использует HTTP Admin API (отдельный порт/Unix-socket) для просмотра активных сессий и принудительного завершения.
6. Опционально: клиент предъявляет TLS-сертификат при handshake; сервер валидирует его против CA.

## User Stories

- P1 (Admin): "Я могу запретить доступ с определённых подсетей и ограничить скорость для каждого токена, чтобы защитить сервер от злоупотреблений."
- P2 (Admin): "Я могу через HTTP API посмотреть, кто подключён, и принудительно отключить подозрительную сессию без SSH на сервер."
- P3 (Security): "Я включаю Origin-проверку и mTLS, чтобы исключить подключения из непроверенных источников."

## MVP Slice

CIDR ACL (4.1) + Origin/Referer validation (4.4) — минимальный защитный слой, закрывающий OWASP-векторы (CSWSH, доступ из недоверенных сетей).

## First Deployable Outcome

После первого implementation pass оператор может:
- задать список разрешённых/запрещённых CIDR в конфиге и убедиться, что пакеты из deny-сетей не проходят
- настроить Origin whitelist и убедиться, что запросы с невалидным Origin получают 403
- проверить через логи, что фильтрация работает

## Scope

- CIDR ACL на уровне TCP-Listener (до TLS): проверка RemoteAddr, allow/deny lists в конфиге
- Per-token bandwidth quota: tokenizer config с полем `bandwidth_bps`, `rate.Limiter` на токен
- Session limits per token: `max_sessions` в конфиге токена, проверка при `SessionManager.Create()`
- Origin/Referer validation: configurable whitelist, проверка перед WebSocket upgrade
- Admin API: отдельный HTTP JSON API сервер (localhost:port, chi router), endpoints: GET /admin/sessions, DELETE /admin/sessions/{id}
- mTLS (опционально): `client_ca_file` в TLS-конфиге, `ClientAuth` = `RequireAndVerifyClientCert`
- Admin API аутентификация: static token в заголовке `X-Admin-Token`

## Контекст

- Зависимости по спринту: 4.1 → завершённые 1.7 (TUN+транспорт), 4.2/4.3 → 1.6 (аутентификация), 4.4 → 1.2 (WebSocket сервер), 4.5 → 1.5 (управление сессиями), 4.6 → 1.3 (TLS)
- Сервер работает на TCP 443, TLS 1.3, gorilla/websocket
- Конфигурация через YAML + viper
- Текущий `CheckOrigin` всегда true — spec меняет это на проверяемый
- Admin API НЕ должен быть доступен снаружи (localhost-only или Unix-socket по умолчанию)
- mTLS опционален — включается опцией в конфиге (`tls.client_ca_file`); сервер стартует и без неё

## Требования

- RQ-001 Сервер ДОЛЖЕН проверять RemoteAddr входящего соединения против списка allow/deny CIDR до TLS-handshake
- RQ-002 Сервер ДОЛЖЕН применять bandwidth quota на уровне токена (token → rate.Limiter)
- RQ-003 Сервер ДОЛЖЕН отклонять новые сессии для токена при превышении max_sessions
- RQ-004 Сервер ДОЛЖЕН проверять HTTP Origin (и опционально Referer) перед WebSocket upgrade
- RQ-005 Admin API ДОЛЖЕН предоставлять GET /admin/sessions (список активных сессий)
- RQ-006 Admin API ДОЛЖЕН предоставлять DELETE /admin/sessions/{id} (принудительный disconnect)
- RQ-007 Admin API ДОЛЖЕН требовать аутентификацию через X-Admin-Token
- RQ-008 Опционально: сервер ДОЛЖЕН проверять клиентский TLS-сертификат при mTLS

## Вне scope

- Ротирование/управление токенами через API (только статический конфиг)
- История сессий (log только active)
- Per-IP rate limiting (уже реализовано)
- Web Application Firewall / DPI
- OAuth2 / JWT / LDAP интеграция

## Критерии приемки

### AC-001 CIDR-фильтрация — deny

- Почему это важно: защита от трафика из недоверенных сетей на транспортном уровне
- **Given** конфигурация сервера содержит `acl.deny_cidrs: ["10.0.0.0/8"]`
- **When** клиент с IP 10.0.0.1 пытается установить TCP-соединение
- **Then** соединение отклоняется до TLS-handshake, сервер логирует `"connection denied by CIDR ACL"`
- Evidence: net.Dial из 10.0.0.1 получает reset/refused; в логе сервера запись о блокировке

### AC-002 CIDR-фильтрация — allow

- Почему это важно: гарантия, что разрешённые подсети работают
- **Given** конфигурация сервера содержит `acl.allow_cidrs: ["192.168.0.0/16"]` (и нет deny_cidrs, перекрывающего)
- **When** клиент с IP 192.168.1.100 пытается установить TCP-соединение
- **Then** соединение принимается и проходит TLS-handshake
- Evidence: WebSocket upgrade успешен (наблюдается через тест)

### AC-003 Per-token bandwidth quota

- Почему это важно: изоляция клиентов по пропускной способности
- **Given** конфиг токена `bandwidth_bps: 102400` (100 Kbps)
- **When** аутентифицированный клиент передаёт данные через туннель
- **Then** скорость передачи данных клиента не превышает 102400 bps (измеряется пачками по 1s)
- Evidence: тест отправляет 1 MB, замеряет время — throughput ≤ 100 Kbps

### AC-004 Session limit per token

- Почему это важно: предотвращение мультиплексирования одного токена на много сессий
- **Given** конфиг токена `max_sessions: 2` и уже 2 активные сессии с этим токеном
- **When** третий клиент пытается аутентифицироваться с тем же токеном
- **Then** сервер отклоняет третью сессию с ошибкой `"max sessions exceeded"`
- Evidence: третье соединение получает ошибку; в логе сервера запись о превышении лимита

### AC-005 Origin validation — валидный Origin

- Почему это важно: разрешить доступ доверенным источникам
- **Given** конфиг `origin.whitelist: ["https://example.com"]`
- **When** клиент отправляет HTTP-запрос на /tunnel с Origin: https://example.com
- **Then** запрос проходит проверку и WebSocket upgrade выполняется
- Evidence: успешное соединение (наблюдаемое через тест)

### AC-006 Origin validation — невалидный Origin

- Почему это важно: блокировка CSWSH-атак
- **Given** конфиг `origin.whitelist: ["https://example.com"]`
- **When** клиент отправляет HTTP-запрос на /tunnel с Origin: https://evil.com
- **Then** сервер возвращает HTTP 403, WebSocket upgrade не выполняется
- Evidence: HTTP-ответ 403; в логе запись `"origin not allowed"`

### AC-007 Admin API — список сессий

- Почему это важно: оперативный мониторинг активных подключений
- **Given** Admin API включён, административный токен сконфигурирован, одна активная сессия с ID `sess-abc-123`
- **When** запрос GET /admin/sessions с заголовком X-Admin-Token: <admin_token>
- **Then** ответ JSON: `{"sessions": [{"id": "sess-abc-123", "token_name": "user1", "remote_addr": "1.2.3.4", "connected_at": "..."}]}`
- Evidence: curl запрос возвращает корректный JSON со списком сессий

### AC-008 Admin API — принудительный disconnect

- Почему это важно: реакция на инциденты (отключение подозрительной сессии)
- **Given** активная сессия `sess-abc-123`
- **When** запрос DELETE /admin/sessions/sess-abc-123 с X-Admin-Token: <admin_token>
- **Then** сессия завершается, ответ HTTP 200, сессия исчезает из списка активных
- Evidence: после DELETE запрос GET /admin/sessions не показывает sess-abc-123; клиент получает ошибку/разрыв соединения

### AC-009 mTLS — успешная аутентификация (опционально)

- Почему это важно: дополнительный фактор аутентификации клиента
- **Given** конфиг `tls.client_ca_file: /etc/certs/ca.pem` и `tls.client_auth: require`
- **When** клиент подключается с валидным сертификатом, подписанным CA
- **Then** TLS-handshake успешен, Subject CN из сертификата доступен для логирования
- Evidence: сервер принимает соединение и логирует `"mTLS client CN: <cn>"`

### AC-010 mTLS — отклонение без сертификата (опционально)

- Почему это важно: гарантия, что RequireAndVerify работает
- **Given** та же конфигурация mTLS
- **When** клиент подключается без клиентского сертификата
- **Then** TLS-handshake завершается ошибкой
- Evidence: tls.Dial возвращает ошибку; сервер логирует предупреждение

### AC-011 Admin API — аутентификация отсутствует

- Почему это важно: защита Admin API от неавторизованного доступа
- **Given** Admin API включён
- **When** запрос GET /admin/sessions без заголовка X-Admin-Token
- **Then** сервер возвращает HTTP 401 Unauthorized
- Evidence: curl без токена получает 401

## Допущения

- CIDR-списки статичны, загружаются при старте (reload через SIGHUP опционален)
- Per-token bandwidth quota — это простой rate.Limiter из golang.org/x/time/rate (как существующий sessionPacketLimiter, но на токен)
- max_sessions проверяется в SessionManager.Create() до выделения IP
- Admin API слушает на localhost:port (по умолчанию localhost:8443), недоступен снаружи; используется chi router с JSON ответами
- Origin whitelist поддерживает glob-паттерны (`https://*.example.com`), а не только точное совпадение

## Критерии успеха

- SC-001 Проверка CIDR не добавляет >1ms к времени установки соединения (бенчмарк на 10k коннектов)
- SC-002 Admin API отвечает за <100ms при 1000 активных сессиях

## Краевые случаи

- CIDR без allow_cidrs (deny-only): все подсети кроме запрещённых разрешены
- CIDR без deny_cidrs (allow-only): только указанные подсети разрешены
- Пустой Origin/Referer: настраиваемое поведение (allow/deny)
- max_sessions = 0: unlimited (отключение проверки)
- bandwidth_bps = 0: unlimited (отключение ограничения)
- Административный токен в конфиге отсутствует: Admin API недоступен
- Одновременное удаление сессии через Admin API и естественное завершение: graceful handling (idempotent delete)

## Открытые вопросы

none — все вопросы уточнены.
