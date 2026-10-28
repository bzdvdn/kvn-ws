# Сервер выдаёт уникальный TUN address

## Scope Snapshot

- In scope: v2-сервер ведёт сохраняемый на диск пул TUN-адресов, выдаёт каждому клиенту уникальный `tun_address`, общий `tun_gateway` и lease с TTL; клиент применяет их без локального fallback и переживает рестарт сервера.
- Out of scope: Android v2 data plane (перенесён в `v2-standard` T4), IPv6, wire-format v2, ротация адресов внутри сессии и кластерный lease-store.

## Цель

Убрать ручной TUN-адрес из клиента и не допустить одинаковых адресов у разных клиентов. Сервер хранит диапазон, выдаёт уникальный IPv4 на клиента и возвращает его через авторизованный session endpoint. Клиент применяет адрес и завершает подключение с ошибкой, если сервер не смог выдать lease.

## Основной сценарий

1. Администратор задаёт `tun.subnet`, `range_start`, `range_end`, `gateway`, `lease_ttl` в конфиге v2-сервера.
2. Клиент при первом запуске получает/сохраняет `client_id` и запрашивает сессию с `X-Request-ID: <client_id>`.
3. Сервер выдаёт свободный IPv4 из диапазона, возвращает `tun_address`, `tun_gateway`, `lease_ttl_sec`; повторный запрос того же клиента продлевает тот же lease.
4. Клиент поднимает TUN с выданным адресом; при ошибке/пустом ответе/исчерпании пула TUN не поднимается и показывается ошибка.

## User Stories

- P1: как пользователь, я не ввожу TUN-адрес и не получаю конфликт с другим клиентом.
- P2: как администратор, я задаю диапазон и TTL в одном месте.

## MVP Slice

- Pool config + validation.
- In-memory lease allocator с TTL.
- Авторизованный session endpoint с `X-Request-ID`.
- Desktop client + web UI без локальных TUN-полей.
- Закрывает `AC-001`–`AC-006`, `AC-008`, `AC-009`.

## First Deployable Outcome

- Два клиента с разными `client_id` получают разные `tun_address`; один клиент повторно получает свой lease.
- При исчерпании диапазона новый клиент получает ошибку, TUN не поднимается.

## Scope

- v2 server config/pool/lease/session endpoint.
- v2 client `client_id`/session/apply/fail-closed.
- Web UI read-only assignment.
- Tests/docs/примеры.

## Контекст

- v2 терминирует TCP на сервере; клиентский TUN-адрес не используется для серверной маршрутизации, но должен быть уникальным на клиентской стороне.
- Ранее клиент имел локальные `tun_address`/`tun_gateway`; в 2.0 они удаляются.
- Server config содержит TUN-пул.
- Session endpoint должен оставаться browser-like: `GET /`, `Accept: application/json`, Bearer; без KVN path.
- Lease хранится в файле, чтобы рестарт сервера не сбрасывал адреса; активные потоки продлевают lease.

## Зависимости

- `v2-standard` (v2-only, stealth surface).
- HTTP/2 transport и существующий v2 client/TUN.

## Требования

- RQ-001 v2 server config ДОЛЖЕН поддерживать `tun.subnet`, `tun.range_start`, `tun.range_end`, `tun.gateway`, `tun.lease_ttl_sec`.
- RQ-002 Сервер ДОЛЖЕН валидировать IPv4 subnet, gateway внутри subnet, range внутри subnet и `range_start <= range_end`.
- RQ-003 Сервер ДОЛЖЕН выдавать уникальный адрес каждому активному `client_id`; повторный запрос продлевает существующий lease.
- RQ-004 При исчерпании диапазона сервер ДОЛЖЕН вернуть 503, а клиент — ошибку `tun assignment exhausted`, без fallback.
- RQ-005 Клиент ДОЛЖЕН автоматически генерировать `client_id`, если его нет в JSON/QR, сохранять его локально и передавать в `X-Request-ID`; QR/JSON не обязан содержать `client_id`.
- RQ-006 Клиент ДОЛЖЕН применять только серверный `tun_address`/`tun_gateway`; локальных TUN-полей и fallback нет.
- RQ-007 Web UI ДОЛЖЕН показывать TUN assignment как read-only.
- RQ-008 Lease ДОЛЖЕН истекать по TTL и освобождать адрес; повторный запрос и активные потоки продлевают TTL.
- RQ-011 Lease store ДОЛЖЕН сохраняться на диск и восстанавливаться после рестарта сервера.
- RQ-012 Клиент после рестарта сервера ДОЛЖЕН продолжить с тем же TUN-адресом за счёт стабильного `client_id` и persisted lease, без ручного переподключения.
- RQ-009 Неавторизованный/некорректный session-запрос ДОЛЖЕН получать fallback, без TUN JSON.

## Вне scope

- IPv6 TUN assignment.
- Persistent lease store между рестартами сервера.
- Ротация адреса внутри одной сессии.
- Локальный fallback/override TUN-адреса.
- Изменение v2 wire-format.

## Критерии приемки

### AC-001 Конфиг пула

- Почему важно: администратор задаёт диапазон и TTL один раз.
- **Given** `tun.subnet: 10.66.0.0/24`, `range_start: 10.66.0.2`, `range_end: 10.66.0.10`, `gateway: 10.66.0.1`, `lease_ttl_sec: 300`
- **When** сервер загружает конфиг
- **Then** значения валидны; gateway/range внутри subnet проверены
- Evidence: unit-тест valid/invalid config.

### AC-002 Уникальные адреса

- Почему важно: два клиента не должны получить один IPv4.
- **Given** пул с двумя свободными адресами
- **When** два клиента с разными `client_id` запрашивают сессию
- **Then** они получают разные `tun_address`
- Evidence: allocator/session integration test.

### AC-003 Повторный lease

- Почему важно: reconnect не должен менять адрес без причины.
- **Given** клиент уже получил lease
- **When** он повторно запрашивает сессию до истечения TTL
- **Then** сервер возвращает тот же адрес и продлевает lease
- Evidence: allocator test с TTL.

### AC-004 Исчерпание пула

- Почему важно: нельзя молча выдать занятый/неверный адрес.
- **Given** все адреса пула заняты
- **When** новый `client_id` запрашивает сессию
- **Then** сервер возвращает 503, клиент не поднимает TUN и показывает `tun assignment exhausted`
- Evidence: integration test + client fail-closed test.

### AC-005 Применение на клиенте

- Почему важно: серверный адрес должен попасть в TUN.
- **Given** валидный session response
- **When** клиент запускает TUN
- **Then** TUN использует только серверные address/gateway; `client_id` сохраняется
- Evidence: client test с fake session и TUN config capture.

### AC-006 UI

- Почему важно: пользователь не вводит адрес вручную.
- **Given** v2-only UI
- **When** пользователь открывает настройки
- **Then** TUN assignment показан read-only, локальных полей address/gateway нет
- Evidence: frontend build/API test.

### AC-008 TTL и освобождение

- Почему важно: пул не должен утекать после отключений.
- **Given** lease с истёкшим TTL
- **When** новый клиент запрашивает адрес
- **Then** освободившийся адрес может быть выдан
- Evidence: allocator test с управляемыми часами.

### AC-011 Рестарт сервера

- Почему важно: рестарт сервера не должен менять TUN-адрес активного клиента.
- **Given** клиент получил lease и есть файл lease store
- **When** сервер перезапускается и клиент переподключает HTTP/2
- **Then** тот же `client_id` получает тот же `tun_address`, а новый клиент — другой
- Evidence: pool restart test + client reconnect smoke.

### AC-012 Продление lease

- Почему важно: TTL не должен освобождать адрес активного клиента.
- **Given** активный TCP-поток с `X-Request-ID`
- **When** поток идёт дольше TTL
- **Then** lease продлевается, адрес не выдаётся другому клиенту
- Evidence: pool `Touch` test + transport stream header test.

### AC-009 Unauthorized/fallback

- Почему важно: endpoint не выдаёт TUN JSON без токена.
- **Given** `GET /` с `Accept: application/json` без валидного Bearer
- **When** сервер обрабатывает запрос
- **Then** возвращается обычный fallback без TUN JSON и KVN-заголовков
- Evidence: integration test + active probe.

### AC-010 Автогенерация client_id

- Почему важно: импорт QR/JSON не должен требовать ручного `client_id`.
- **Given** конфиг/QR без `client_id`
- **When** клиент впервые запускается
- **Then** он генерирует случайный ID, сохраняет локально и использует его при повторных запусках
- Evidence: unit-тест config store и клиентский smoke без `client_id`.

## Допущения

- Lease store — JSON-файл с атомарной заменой; рестарт сервера восстанавливает leases.
- `client_id` — случайный UUID, генерируется клиентом и хранится в едином `~/.config/kvn/config.yaml`.
- Gateway один для пула.
- `X-Request-ID` используется как ключ lease; это обычный HTTP-заголовок.
- Клиент без валидного assignment не поднимает TUN.
- Один клиент = один активный lease; lease продлевается session-запросом и активными потоками.
- Android использует этот же contract позже, в `v2-standard` T4.

## Критерии успеха

- SC-001 100 клиентов получают 100 разных адресов в пуле ≥100.
- SC-002 Session p95 < 300 ms в lab; lease lookup O(1).
- SC-003 После истечения TTL адрес переиспользуется.
- SC-004 После рестарта сервера активный клиент сохраняет адрес.

## Краевые случаи

- Пул пуст/все заняты → 503 + client error.
- `range_start/range_end` вне subnet → server startup error.
- Gateway вне subnet → server startup error.
- Клиент без `client_id` → 400/ошибка, без выдачи адреса.
- Просроченный lease → адрес освобождается и переиспользуется.
- Неверный/просроченный токен → fallback, не 500.
- `tun` отсутствует → server startup error.
- Lease-файл отсутствует/повреждён → сервер стартует с пустым пулом (без падения).
- Рестарт сервера → leases восстанавливаются из файла.

## Открытые вопросы

- none
