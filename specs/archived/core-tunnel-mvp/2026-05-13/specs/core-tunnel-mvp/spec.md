# Core Tunnel MVP — работающий туннель

## Scope Snapshot

- **In scope:** самый короткий путь к работающему end-to-end туннелю: TUN, WebSocket, TLS, фрейминг, handshake, auth, packet forwarding, IP pool, graceful shutdown.
- **Out of scope:** split-tunnel, NAT, DNS resolver, keepalive, reconnect, метрики, persistence, IPv6, routing rules.

## Цель

Разработчик получает работающий VPN-туннель: клиент подключается к серверу через WSS, проходит аутентификацию, получает IP из пула, и весь IP-трафик клиента туннелируется через сервер. Успех измеряется командой `ping <server_assigned_ip>`.

## Основной сценарий

1. Сервер стартует с TLS-сертификатом, слушает `:443`, готов к WebSocket Upgrade.
2. Клиент стартует, открывает TUN-устройство, устанавливает WSS-соединение с сервером.
3. Клиент отправляет Client Hello (bearer-token) → Сервер валидирует токен, отвечает Server Hello (session_id, assigned IP).
4. Клиент настраивает TUN с assigned IP, запускает forwarding: TUN → WS → сервер → TUN → NAT/Internet.
5. Ответный трафик идёт: Internet → TUN сервера → WS → клиент → TUN клиента.
6. При SIGTERM обе стороны корректно закрывают TUN и WS-соединение.

## User Stories

- P1: Пользователь запускает клиент и сервер, пакеты идут через туннель.
- P2: Невалидный токен отклоняется до назначения IP.

## MVP Slice

Полный Sprint 1: все 10 задач обязательны, без них gate не проходится. Все AC-* должны быть закрыты.

## First Deployable Outcome

Два процесса (client + server), соединённых WSS/TLS. `ping <assigned_ip>` проходит от клиента до сервера. IP-пул работает в памяти.

## Scope

1. TUN-абстракция (Linux, `wireguard/tun`): открытие, чтение/запись IP-пакетов, установка IP/link MTU.
2. WebSocket transport: client dial, server accept через HTTP Upgrade.
3. TLS 1.3: серверный TLS listener (самоподписанный сертификат), TLS dial клиента с настраиваемым `InsecureSkipVerify`.
4. Бинарные фреймы: Type(1B)/Flags(1B)/Length(2B)/Payload(N) — encode/decode.
5. Client-Server handshake: Client Hello → Server Hello, session_id, assigned IP.
6. Bearer-token auth: проверка на сервере, отказ при невалидном.
7. Packet forwarding: client→server→TUN и server→client→TUN.
8. IP Pool Manager: in-memory allocate/release/resolve session→IP.
9. Graceful shutdown: SIGTERM контексты, закрытие TUN, WS, слушателя.
10. Конфигурация: расширение существующих client.yaml / server.yaml новыми полями (tun, ws, tls, auth).

## Контекст

- Только Linux (TUN). Другие ОС — вне scope.
- Самоподписанные TLS-сертификаты для MVP; production-сертификаты — позже.
- Только IPv4; IPv6 — следующий этап.
- IP пул in-memory (sync.Map или простой map+mutex); BoltDB — в Sprint 3.
- Один TUN-интерфейс на процесс; без multi-client в этом sprint (но архитектура должна позволять).
- Поведение сохранено: foundation (конфиги, логгер, graceful shutdown).
- `gorilla/websocket` — библиотека WS.

## Требования

### RQ-001 TUN-абстракция

Система ДОЛЖНА реализовать интерфейс `TunDevice` с методами `Open()`, `Close()`, `Read([]byte) (int, error)`, `Write([]byte) (int, error)`, `SetIP(net.IP, *net.IPNet)`, `SetMTU(int)`. Реализация через `golang.zx2c4.com/wireguard/tun`.

### RQ-002 WebSocket транспорт

Система ДОЛЖНА реализовать клиентский dial (wss://) и серверный accept (HTTP Upgrade) через `gorilla/websocket`. Интерфейс `WSConn` с `ReadMessage()`, `WriteMessage()`, `Close()`.

### RQ-003 TLS 1.3

Сервер ДОЛЖЕН запускать TLS 1.3 listener. Клиент ДОЛЖЕН подключаться через TLS 1.3 dial с опцией `InsecureSkipVerify` (configurable). Путь к сертификату и ключу — поля в server.yaml.

### RQ-004 Фрейминг

Все данные поверх WS ДОЛЖНЫ передаваться в бинарных фреймах: Type (1B), Flags (1B), Length (2B big-endian), Payload (N bytes). Структура `Frame` с методами `Encode() ([]byte, error)` и `Decode([]byte) error`.

### RQ-005 Handshake

После WS-upgrade клиент ДОЛЖЕН отправить `ClientHello` (proto_version, token). Сервер ДОЛЖЕН ответить `ServerHello` (session_id, assigned_ip) или `AuthError`.

### RQ-006 Аутентификация

Сервер ДОЛЖЕН проверять bearer-token из ClientHello. При невалидном токене — `AuthError` и закрытие WS. Токен — статическая строка в server.yaml.

### RQ-007 Forwarding client→server

Клиент ДОЛЖЕН читать IP-пакеты из TUN, фреймировать их и отправлять через WS на сервер. Сервер ДОЛЖЕН дефреймировать, извлекать IP-пакет и писать в TUN-устройство сервера.

### RQ-008 Forwarding server→client

Сервер ДОЛЖЕН читать IP-пакеты из TUN, фреймировать и отправлять клиенту. Клиент ДОЛЖЕН дефреймировать и писать в TUN клиента.

### RQ-009 IP Pool

Система ДОЛЖНА предоставлять in-memory IP Pool с методами `Allocate(sessionID) (net.IP, error)`, `Release(sessionID)`, `Resolve(sessionID) (net.IP, bool)`. Подсеть настраивается в server.yaml.

### RQ-010 Graceful shutdown

Оба процесса ДОЛЖНЫ обрабатывать SIGTERM: отменять контекст, дожидаться завершения фрейм-читалок/писалок, закрывать TUN, WS, TLS listener.

## Вне scope

- Split-tunnel, routing rules, CIDR/domain policies (Sprint 2).
- NAT (iptables/nftables MASQUERADE) — предполагается hand-configured.
- DNS resolver, DNS override.
- Keepalive, PING/PONG, reconnect.
- Prometheus метрики, health endpoint.
- Admin API, CLI-управление сессиями.
- IP pool persistence (BoltDB).
- Multi-client поддержка (но архитектура должна допускать).
- IPv6.
- QUIC/HTTP3.
- Mobile-клиенты.
- GUI.

## Критерии приемки

### AC-001 TUN device читает и пишет IP-пакеты

- Почему это важно: без TUN нет туннеля, это основа.
- **Given** Linux-система с разрешённым TUN (/dev/net/tun)
- **When** TunDevice открыт (Open), ему назначен IP и MTU, и в него записан IP-пакет
- **Then** тот же пакет читается из TUN, и наоборот: запись → чтение
- Evidence: unit-тест с виртуальным TUN или `ping` через TUN-интерфейс

### AC-002 WebSocket-соединение клиент-сервер

- Почему это важно: транспортный канал туннеля.
- **Given** сервер слушает WSS
- **When** клиент выполняет dial на `wss://server:443/tunnel`
- **Then** соединение установлено, сообщения проходят в обе стороны
- Evidence: echo-тест (клиент шлёт сообщение, сервер отвечает)

### AC-003 TLS 1.3 listener и dial

- Почему это важно: шифрование трафика mandatory.
- **Given** сервер с TLS-сертификатом и TLS 1.3 конфигом
- **When** клиент подключается через `tls.Dial` с `tls.VersionTLS13`
- **Then** handshake успешен, соединение шифрованное
- Evidence: тест проверяет `tls.ConnectionState().Version == tls.VersionTLS13`

### AC-004 Бинарные фреймы кодируются/декодируются

- Почему это важно: единый протокол поверх WS.
- **Given** структура Frame с Type, Flags, Length, Payload
- **When** Frame.Encode() вызван, затем Decode() на полученных байтах
- **Then** поля совпадают (round-trip)
- Evidence: unit-тест с различными Type/Flags/Payload

### AC-005 Handshake с назначением session_id и IP

- Почему это важно: клиент должен получить идентификатор и IP.
- **Given** WS-соединение установлено, валидный bearer-token в конфиге клиента
- **When** клиент отправляет ClientHello
- **Then** сервер отвечает ServerHello с session_id и assigned_ip; у клиента появляется рабочий IP
- Evidence: лог клиента содержит "handshake complete: session=<id>, ip=<assigned_ip>"

### AC-006 Bearer-token auth отклоняет невалидный токен

- Почему это важно: безопасность.
- **Given** WS-соединение установлено
- **When** клиент отправляет ClientHello с неверным токеном
- **Then** сервер отвечает AuthError и закрывает WS
- Evidence: лог сервера содержит "auth failed", клиент получает AuthError

### AC-007 IP-пакет клиента доходит до TUN сервера

- Почему это важно: основной forwarding path.
- **Given** handshake выполнен, TUN открыт с обеих сторон
- **When** клиент пишет IP-пакет в свой TUN
- **Then** пакет появляется в TUN сервера (читается из TunDevice сервера)
- Evidence: интеграционный тест с echo-пакетом: client TUN → server TUN

### AC-008 Ответный IP-пакет возвращается клиенту

- Почему это важно: двусторонняя связь.
- **Given** handshake выполнен, forwarding client→server работает
- **When** сервер пишет IP-пакет в свой TUN (ответ)
- **Then** пакет появляется в TUN клиента
- Evidence: `ping <assigned_ip>` с клиента получает reply

### AC-009 IP пул выделяет и освобождает адреса

- Почему это важно: без пула клиенты не получат уникальные IP.
- **Given** IP Pool с настроенной подсетью (например 10.0.0.0/24)
- **When** Allocate вызывается для session_id
- **Then** возвращается уникальный IP из подсети; повторный Allocate того же session_id возвращает тот же IP
- **When** Release вызывается
- **Then** IP возвращается в пул
- Evidence: unit-тест: allocate 254 адреса, release, allocate снова

### AC-010 Graceful shutdown без ошибок

- Почему это важно: ресурсы не должны течь.
- **Given** клиент и сервер соединены, идёт трафик
- **When** SIGTERM отправлен серверу
- **Then** сервер закрывает TLS listener, WS-соединения, TUN — без ошибок (лог "shutdown complete")
- **When** SIGTERM отправлен клиенту
- **Then** клиент закрывает WS, TUN — без ошибок
- Evidence: лог обоих процессов содержит "shutting down" и завершается exit 0

## Допущения

- Linux (Ubuntu 22.04+), ядро с поддержкой TUN.
- Самоподписанные сертификаты для разработки; путь к cert/key — в server.yaml.
- Серверный TUN используется в режиме layer-3 (ip, не tap).
- Один клиент на сервер в этом sprint.
- Токен статический, задаётся в server.yaml (для Sprint 1).
- IP-пул использует простую map+mutex, без persistence.
- NAT на сервере настраивается вручную (iptables) или не требуется для локальных тестов.
- Порты: WS :443 (server), TUN — стандартное устройство /dev/net/tun.

## Критерии успеха

- SC-001: Handshake выполняется за <1s (TCP + TLS + WS + Hello).
- SC-002: Ping latency через туннель ≤ 2x от прямой ping latency (localhost).
- SC-003: Пропускная способность ≥ 50% от raw TCP через localhost.

## Краевые случаи

- TUN-устройство недоступно (нет /dev/net/tun, нет прав) — fatal-лог на старте.
- Неверный TLS-сертификат (путь не найден, истёк) — fatal на старте сервера.
- WS-соединение разорвано во время forwarding — клиент логирует ошибку, завершается.
- IP-пул исчерпан (все адреса заняты) — сервер отвечает `PoolExhausted`, отклоняет handshake.
- Повторный handshake с тем же session_id — сервер возвращает существующий IP.
- Пустой/нулевой токен — сервер отвечает AuthError.
- Двойной SIGTERM — второй игнорируется, контекст уже отменён.

## Открытые вопросы

- `none` — все решения приняты roadmap и конституцией.
