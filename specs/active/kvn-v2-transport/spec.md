# KVN v2: TCP-first stealth transport

## Scope Snapshot

- In scope: новый v2-протокол Go server/client, TCP split-connection через стандартный HTTP/2 streaming, stealth по умолчанию, упрощённый multi-server UI.
- Out of scope: совместимость с v1 wire-протоколом, Android, relay v2, HTTP/3/QUIC в MVP.

## Цель

Сделать VPN, который в TCP-only сети выглядит как обычный HTTPS/HTTP2-сервис, не страдает от TCP-in-TCP и требует минимум настроек. Пользователь добавляет несколько серверов, выбирает один и подключается; stealth/fallback включены по умолчанию, а не спрятаны в advanced.

## Основной сценарий

1. Пользователь добавляет сервер: имя, endpoint, токен.
2. Клиент открывает TLS 1.3 + HTTP/2 к домену, проходит Bearer-auth на стандартном пути.
3. TCP-потоки приложения завершаются локально и уходят отдельными HTTP/2-стримами; сервер открывает обычные TCP-сокеты.
4. UDP идёт best-effort стримом в MVP; при доступном UDP — H3 datagrams в следующей фазе.
5. Неавторизованный запрос получает реальный fallback-сайт, а не tunnel-ответ.
6. v1-серверы остаются для старых клиентов; v2-клиент их не трогает.

## User Stories

- P1: как пользователь в TCP-only сети, я хочу быстрый VPN без TCP-in-TCP.
- P2: как пользователь, я хочу добавить несколько серверов и переключаться между ними.
- P3: как пользователь, я не хочу настраивать padding/multiplex/secondary — stealth должен работать сам.
- P4: как оператор, я хочу, чтобы probe видел обычный сайт, а не VPN.
- P5: как владелец старых клиентов, я не хочу, чтобы v2 ломал v1-серверы.

## MVP Slice

- Go v2 server/client: TLS/HTTP2, Bearer-auth, TCP split-connection, per-flow streams.
- Stealth: fallback site/reverse-proxy, no legacy path, standard headers.
- Minimal multi-server UI.
- Закрывает AC-001, AC-002, AC-003, AC-005, AC-007.

## First Deployable Outcome

- v2-сервер и v2-клиент соединяются по HTTP/2; TCP-поток проходит без вложенного TCP; без токена отдаётся fallback-сайт; UI позволяет выбрать один из нескольких серверов.

## Scope

- Новый v2 wire-protocol и отдельные endpoints; v1 несовместим.
- HTTP/2 bidirectional streaming для TCP-потоков (gRPC/Connect — опционально).
- TCP termination на клиенте (userspace stack) и открытие target TCP на сервере.
- UDP MVP: best-effort stream; H3 datagrams — следующая фаза.
- Stealth by default: Bearer до upgrade/stream, fallback, стандартные заголовки, без кастомных subprotocol/маркеров.
- Упрощённый primary UI: multi-server, имя/endpoint/токен; Advanced доступен отдельными вкладками (Routing, DNS, TLS, Proxy).
- Benchmark и DPI-проверки.

## Контекст

- v1 страдает от TCP-in-TCP, одного primary/secondary WS, padding/no-pad, случайных uTLS-пресетов и сложного UI.
- v1-серверы остаются для старых клиентов; v2 может быть несовместим.
- Целевая сеть часто блокирует UDP, поэтому TCP-first обязателен.
- TLS 1.3 и HTTP/2 уже выглядят как обычный веб-трафик; HTTP/2 streaming — стандартный формат веб-трафика; gRPC/Connect можно добавить позже.
- Пользователи готовы тестировать на нескольких серверах.

## Зависимости

- `golang.org/x/net/http2`, gRPC/Connect, userspace TCP stack (gVisor/lwIP), существующие config/routing/DNS/TUN/NAT/webui.

## Требования

- RQ-001 v2 wire-protocol ДОЛЖЕН быть отдельным и несовместимым с v1; v1 endpoints не используются v2-клиентом.
- RQ-002 TCP ДОЛЖЕН терминироваться на клиенте и передаваться отдельными HTTP/2-стримами; сырые TCP-пакеты внутри туннеля запрещены.
- RQ-003 Каждый TCP-flow ДОЛЖЕН иметь независимый stream/flow control; медленный flow не блокирует остальные.
- RQ-004 UDP ДОЛЖЕН иметь явный best-effort путь в MVP и H3 datagrams при доступном UDP в следующей фазе.
- RQ-005 Stealth ДОЛЖЕН быть включён по умолчанию: стандартный TLS/HTTP2, Bearer до stream, fallback-сайт, без legacy path и кастомных маркеров.
- RQ-006 TLS-профиль ДОЛЖЕН быть консистентным между соединениями; случайная смена браузера на каждый connect запрещена.
- RQ-007 UI ДОЛЖЕН иметь минимальный primary flow (имя, endpoint, токен), поддерживать несколько серверов и сохранять доступ к Advanced: routing/DNS/TLS/proxy обязательны и не скрыты.
- RQ-008 v2 НЕ ДОЛЖЕН ухудшать throughput/p95 latency относительно v1 на сопоставимом канале.
- RQ-009 v1-серверы и v1-клиенты ДОЛЖНЫ продолжать работать независимо; миграция поэтапная.
- RQ-010 Диагностика ДОЛЖНА показывать flow/stream ошибки без токенов и без раскрытия v2-сигнатур.

## Вне scope

- Wire-совместимость с v1.
- Android v2 (отдельная фаза).
- Relay v2 (отдельная фаза).
- HTTP/3/QUIC в MVP (следующая фаза для UDP/latency).
- ECH/domain fronting/CDN orchestration (инфраструктурная фаза).
- Cover traffic и тяжёлый shaping.

## Критерии приемки

### AC-001 TCP без TCP-in-TCP

- Почему важно: это корень лагов в v1.
- **Given** TCP-поток приложения через v2
- **When** трафик идёт по HTTP/2 stream
- **Then** на wire нет вложенных TCP-сегментов; клиент завершает TCP локально, сервер открывает target TCP
- Evidence: unit/integration тест + pcap/структурный тест, latency/retransmit метрики.

### AC-002 Изоляция потоков

- Почему важно: один медленный flow не должен вешать остальные.
- **Given** два TCP-flow, один искусственно замедлен
- **When** второй передаёт данные
- **Then** второй не блокируется первым; HTTP/2 flow control работает per-stream
- Evidence: integration-тест с slow target.

### AC-003 Stealth по умолчанию

- Почему важно: пользователь не должен включать stealth вручную.
- **Given** v2-сервер с fallback-сайтом
- **When** приходит запрос без валидного Bearer
- **Then** отдаётся сайт/правдоподобный ответ, без tunnel-ошибок, legacy path и кастомных маркеров
- Evidence: httptest + проверка заголовков/путей.

### AC-004 Консистентный TLS-профиль

- Почему важно: случайная смена fingerprint — аномалия.
- **Given** серия v2-подключений
- **When** анализируется ClientHello/JA3/JA4
- **Then** профиль одинаковый (один браузерный/HTTP2-профиль), ALPN консистентен
- Evidence: тест серии ClientHello + DPI-лаборатория.

### AC-005 Упрощённый multi-server UI

- Почему важно: текущий UI перегружен.
- **Given** пользователь открывает v2 UI
- **When** добавляет сервер
- **Then** primary flow требует только имя, endpoint и токен; серверы перечислены и выбираются
- **When** открывает Advanced
- **Then** доступны Routing (include/exclude CIDR/IP/domain), DNS, TLS и proxy-настройки; stealth остаётся всегда on и не требует ручного включения
- Evidence: frontend build + UI-тест/скриншот + config roundtrip.

### AC-006 Производительность не хуже v1

- Почему важно: v2 должен давать измеримый выигрыш по latency/throughput, а не только новый вид.
- **Given** одинаковый канал и профиль TCP/UDP
- **When** сравниваются v1 и v2
- **Then** throughput не ниже, p95 latency не выше; allocs не растут
- Evidence: benchmark/soak-отчёт.

### AC-007 Миграция без поломки v1

- Почему важно: старые клиенты/серверы остаются.
- **Given** v1-клиент и v1-сервер
- **When** разворачивается v2 рядом
- **Then** v1 продолжает работать; v2-клиент не подключается к v1 endpoint
- Evidence: mixed-deployment integration.

### AC-008 UDP best-effort и H3-путь

- Почему важно: TCP-only сеть не должна ломать UDP-приложения.
- **Given** UDP-трафик в v2 MVP без H3
- **When** пакеты идут через best-effort stream
- **Then** они доставляются или дропаются по явной политике, без фейковой надёжности
- **Given** доступный UDP/H3
- **Then** используется datagram-путь
- Evidence: integration-тест + конфиг/лог выбора пути.

### AC-009 Диагностика без утечек

- Почему важно: оператор должен видеть flow/stream проблемы.
- **Given** ошибка stream/flow
- **When** пишется лог/метрика
- **Then** нет токена/SNI/полного endpoint; есть stream/flow id и причина
- Evidence: лог-тест.

## Допущения

- v2 может быть несовместим с v1; старые серверы остаются отдельно.
- Пользователь готов тестировать на нескольких серверах и мигрировать постепенно.
- Userspace TCP stack (gVisor/lwIP) приемлем по CPU/RAM; иначе fallback на OS-level transparent proxy.
- TLS-сертификат/домен валидны в production; fallback-сайт реально существует.
- UDP может быть недоступен; TCP-first MVP обязателен.

## Критерии успеха

- SC-001 v2 TCP p95 latency ≤ v1 на том же канале.
- SC-002 Throughput ≥ v1.
- SC-003 Один DPI-профиль на все соединения.
- SC-004 v1 и v2 работают параллельно без взаимного влияния.

## Краевые случаи

- UDP заблокирован полностью.
- TLS/HTTP2 middlebox ломает ALPN.
- Fallback upstream недоступен.
- Медленный/зависший target TCP.
- Много одновременных flow (лимит streams).
- Смена сервера во время звонка.
- v2-клиент по ошибке направлен на v1 endpoint.

## Открытые вопросы

- none
