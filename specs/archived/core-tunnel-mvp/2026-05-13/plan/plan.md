# Core Tunnel MVP — План

## Phase Contract

Inputs: spec, inspect (pass), minimal repo-контекст.
Outputs: plan, data model.
Stop if: spec расплывчата для безопасного планирования.

## Цель

Реализовать end-to-end туннель: TUN → фрейминг → WS/TLS → handshake + auth → forwarding → IP pool. Все 10 задач Sprint 1 из roadmap. Foundation-скелет (пакеты, конфиги, main.go) уже существует.

## MVP Slice

Полный Sprint 1 — все AC-001–AC-010 обязательны для gate (`ping <assigned_ip>`).

## First Validation Path

```
# terminal 1 — server
go run ./src/cmd/server --config configs/server.yaml

# terminal 2 — client
go run ./src/cmd/client --config configs/client.yaml

# terminal 3 — ping через туннель
ping <IP_назначенный_клиенту>
```

## Scope

1. `src/internal/tun/` — TunDevice interface + wireguard/tun реализация
2. `src/internal/transport/websocket/` — WS dial/accept обёртка
3. `src/internal/transport/tls/` — TLS конфиг для listener и dial
4. `src/internal/transport/framing/` — Frame.Encode/Decode
5. `src/internal/protocol/handshake/` — ClientHello/ServerHello
6. `src/internal/protocol/auth/` — Bearer-token проверка
7. `src/internal/session/` — Session store + IP pool
8. `src/cmd/client/main.go` — инициализация и forwarding loop
9. `src/cmd/server/main.go` — listener, accept, forwarding loop

Границы не scope: routing/rules, NAT, keepalive, metrics, persistence.

## Implementation Surfaces

| Surface | Файлы | Статус |
|---------|-------|--------|
| tun-interface | `src/internal/tun/tun.go` | stub → impl |
| ws-transport | `src/internal/transport/websocket/websocket.go` | stub → impl |
| tls-config | `src/internal/transport/tls/tls.go` | stub → impl |
| framing-protocol | `src/internal/transport/framing/framing.go` | stub → impl |
| handshake-proto | `src/internal/protocol/handshake/handshake.go` | stub → impl |
| auth-logic | `src/internal/protocol/auth/auth.go` | stub → impl |
| session-mgr | `src/internal/session/session.go` | stub → impl |
| client-main | `src/cmd/client/main.go` | foundation → extend |
| server-main | `src/cmd/server/main.go` | foundation → extend |

## Bootstrapping Surfaces

`none` — все директории существуют как foundation stubs. Новых пакетов не требуется.

## Влияние на архитектуру

- Локальное: каждый stub-пакет получает реализацию. Никаких cross-cutting изменений.
- Packet forwarding loop добавляет две горутины на direction (client: tun→ws, ws→tun; server: ws→tun, tun→ws).
- Конфиги уже содержат нужные поля (ClientConfig.Server, .Auth.Token; ServerConfig.TLS, .Network.Pool, .Auth.Tokens) — foundation спроектирован с запасом.
- Никаких миграций или compatibility-последствий.

## Acceptance Approach

- AC-001 (TUN read/write) → tun-interface. Unit-тест с виртуальным TUN.
- AC-002 (WS connect) → ws-transport. Интеграционный тест dial+accept+echo.
- AC-003 (TLS 1.3) → tls-config. Тест с самоподписанным сертификатом, проверка Version == tls.VersionTLS13.
- AC-004 (Frame round-trip) → framing-protocol. Unit-тест Encode/Decode с разными payload.
- AC-005 (Handshake) → handshake-proto + session-mgr. Тест: ClientHello → ServerHello c session_id + IP.
- AC-006 (Auth reject) → auth-logic. Тест: невалидный токен → AuthError → WS closed.
- AC-007 (Forward client→server) → интеграция всех поверхностей. IP-пакет из TUN клиента → TUN сервера.
- AC-008 (Forward server→client) → интеграция. Ответный пакет из TUN сервера → TUN клиента.
- AC-009 (IP pool) → session-mgr. Unit-тест: allocate/release/resolve, exhaustion.
- AC-010 (Graceful shutdown) → client-main + server-main. SIGTERM → лог shutdown → exit 0.

## Данные и контракты

См. `data-model.md`. Изменений persisted model нет — всё in-memory.

## Стратегия реализации

### DEC-001 TUN через wireguard/tun

Why: единственная Cgo-зависимость, разрешённая конституцией, стабильная, production-tested.
Tradeoff: только Linux; не-zero deps (Cgo).
Affects: tun-interface.
Validation: AC-001.

### DEC-002 WS через gorilla/websocket

Why: roadmap decision, стабильнее альтернатив для серверного WS.
Tradeoff: gorilla больше не развивается активно, но для MVP стабильности достаточно.
Affects: ws-transport.
Validation: AC-002.

### DEC-003 Бинарный фрейм Type/Flags/Length/Payload

Why: минимальный оверхед, простой парсинг, big-endian для network byte order.
Tradeoff: Length 2 байта → max payload 65535 (достаточно для MTU≤1500).
Affects: framing-protocol.
Validation: AC-004.

### DEC-004 Handshake в один round-trip

Why: ClientHello (proto_version + token) → ServerHello (session_id + assigned_ip). Минимум latency до первого пакета.
Tradeoff: нет negotiation флагов (будут в будущих версиях через расширение фреймов).
Affects: handshake-proto, auth-logic, session-mgr.
Validation: AC-005, AC-006.

### DEC-005 Статический bearer-token

Why: самый простой путь для MVP, без JWT-библиотек.
Tradeoff: нет expiry, нет per-user granularity. Ротация токена — рестарт сервера.
Affects: auth-logic.
Validation: AC-006.

### DEC-006 IP pool in-memory (map+sync.Mutex)

Why: достаточно для Sprint 1, zero external deps.
Tradeoff: нет persistence (потеря при рестарте), нет race-free concurrent access без Mutex.
Affects: session-mgr.
Validation: AC-009.

### DEC-007 Forwarding через две горутины на направление

Why: каждая direction (TUN→WS и WS→TUN) — независимый цикл чтения-записи. Одна горутина читает из источника и пишет в приёмник.
Tradeoff: без backpressure — если одна сторона медленнее, буферы растут. Для MVP допустимо.
Affects: client-main, server-main, ws-transport, tun-interface, framing-protocol.
Validation: AC-007, AC-008.

### DEC-008 errgroup для lifecycle

Why: стандартная идиома Go для управления группой горутин. Отмена контекста завершает все горутины.
Tradeoff: errgroup.FirstError — первый не-nil error завершает группу. Нам подходит.
Affects: client-main, server-main.
Validation: AC-010.

## Incremental Delivery

### MVP (Первая ценность)

1. framing + tun interface + session/IP pool (core types)
2. WS transport + TLS config (каналы)
3. Handshake + auth (протокол)
4. Forwarding loops + интеграция в main.go
5. Graceful shutdown

### Итеративное расширение

- Sprint 2: routing, NAT, DNS resolver
- Sprint 3: keepalive, reconnect, metrics, persistence

## Порядок реализации

1. **Phase 1 (core types):** framing → tun interface → session/IP pool. Могут быть параллельны (нет зависимостей друг от друга). Проверка: unit-тесты (AC-004, AC-001, AC-009).
2. **Phase 2 (transport):** WS dial/accept + TLS config. Зависимость от Phase 1 только через импорт. Проверка: WS echo-тест (AC-002), TLS version (AC-003).
3. **Phase 3 (protocol):** handshake + auth. Зависит от Phase 2 (WS), session (IP pool). Проверка: AC-005, AC-006.
4. **Phase 4 (forwarding):** client forwarding loop + server forwarding loop. Зависит от Phase 1-3. Проверка: AC-007, AC-008.
5. **Phase 5 (integration):** main.go graceful shutdown с errgroup. Зависит от Phase 4. Проверка: AC-010.

Параллельно: Phase 1 (framing, tun, session) — все три задачи не зависят друг от друга.

## Риски

- **wireguard/tun Cgo:** сборка требует gcc, cross-compilation усложнена. Mitigation: Docker multi-stage уже настроен, бинарный хост-тест на Linux.
- **gorilla/websocket deprecation:** проект в archive mode. Mitigation: для MVP стабильности достаточно, миграция на nhooyr — отдельная задача.
- **Нет backpressure:** forwarding без контроля скорости. Mitigation: для localhost/LAN тестов достаточно, backpressure — Sprint 3.
- **TUN permissions:** /dev/net/tun требует root или CAP_NET_ADMIN. Mitigation: документировать, что `sudo` или `docker` обязательны.

## Rollout и compatibility

- Нет rollout-действий — бинарники собираются заново, сервер пока standalone.
- Изменения публичного API нет — всё internal.
- Конфиги client.yaml / server.yaml foundation уже содержат нужные поля — breaking change отсутствует.

## Проверка

- Unit-тесты: framing, session/IP pool, handshake (messages), auth (token check).
- Integration-тесты: WS dial+accept echo, TLS version check, handshake round-trip, packet forwarding.
- Manual: `ping <assigned_ip>` через туннель.
- `go test ./...` + race detector перед verify.

## Соответствие конституции

- нет конфликтов
- DDD через разделение пакетов сохранён
- Traceability маркеры (`@sk-task`, `@sk-test`) обязательны
- Никакого глобального состояния
