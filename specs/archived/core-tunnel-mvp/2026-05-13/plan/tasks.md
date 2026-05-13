# Core Tunnel MVP — Задачи

## Phase Contract

Inputs: spec, plan, plan.digest, data-model (no-change).
Outputs: упорядоченные исполнимые задачи с Touches и AC-покрытием.
Stop if: задачи расплывчаты или AC не покрыты.

## Implementation Context

- **Цель MVP:** TUN → WSS/TLS → handshake → auth → packet forwarding → IP pool. Gate: `ping <assigned_ip>`.
- **Инварианты:**
  - Frame: Type(1B) + Flags(1B) + Length(2B big-endian) + Payload(N)
  - Handshake: ClientHello(proto_version, token_len, token) → ServerHello(session_id, assigned_ip) или AuthError
  - IP pool: map+sync.Mutex, подсеть из server.yaml
  - Forwarding: горутина читает TUN→пишет WS; другая читает WS→пишет TUN
  - errgroup для lifecycle; отмена контекста завершает все горутины
- **Границы scope:** не делаем routing/rules, NAT, keepalive, reconnect, metrics, persistence, multi-client
- **Proof signals:** `go test ./... -race`, `ping <assigned_ip>` проходит, graceful shutdown без ошибок
- **References:** DEC-001–DEC-008, all data in-memory (data-model: no-change)

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/transport/framing/framing.go | T1.1, T5.1 |
| src/internal/tun/tun.go | T1.2, T4.1, T5.1 |
| src/internal/session/session.go | T1.3, T5.1 |
| src/internal/transport/websocket/websocket.go | T2.1, T4.1 |
| src/internal/transport/tls/tls.go | T2.2 |
| src/internal/protocol/handshake/handshake.go | T3.1, T5.1 |
| src/internal/protocol/auth/auth.go | T3.2, T5.1 |
| src/cmd/client/main.go | T4.1, T4.2 |
| src/cmd/server/main.go | T4.1, T4.2 |

## Фаза 1: Core types

Цель: реализовать базовые типы данных — фрейм, TUN-интерфейс, IP pool. Все три задачи независимы и могут выполняться параллельно.

- [x] T1.1 Реализовать бинарный фрейм-протокол: структура Frame (Type, Flags, Length, Payload), методы Encode/Decode, big-endian длина, max payload 65535. Touches: src/internal/transport/framing/framing.go
- [x] T1.2 Реализовать интерфейс TunDevice (Open, Close, Read, Write, SetIP, SetMTU) и адаптер через wireguard/tun. Задать IP/MTU через syscall. Touches: src/internal/tun/tun.go
- [x] T1.3 Реализовать Session store + in-memory IP Pool Manager (Allocate/Release/Resolve) через map+sync.Mutex. Подсеть из ServerConfig.Network.PoolIPv4. Touches: src/internal/session/session.go

## Фаза 2: Transport

Цель: реализовать транспортный слой — WS dial/accept и TLS 1.3.

- [x] T2.1 Реализовать WebSocket обёртку: Dial + Accept. WSConn оборачивает gorilla/websocket.Conn. Touches: src/internal/transport/websocket/websocket.go
- [x] T2.2 Реализовать TLS 1.3 server config (NewServerTLSConfig) и client config (NewClientTLSConfig). Touches: src/internal/transport/tls/tls.go

## Фаза 3: Protocol

Цель: реализовать handshake и auth.

- [x] T3.1 Реализовать ClientHello (proto_version + bearer-token) и ServerHello (session_id + assigned_ip) сообщения, encode/decode. Touches: src/internal/protocol/handshake/handshake.go
- [x] T3.2 Реализовать Bearer-token аутентификацию: ValidateToken. Touches: src/internal/protocol/auth/auth.go

## Фаза 4: Forwarding и graceful shutdown

Цель: соединить все компоненты в работающий туннель.

- [x] T4.1 Реализовать forwarding loops: клиент — горутина TUN→WS и горутина WS→TUN; сервер — горутина WS→TUN и горутина TUN→WS. Каждая читает из источника, фреймирует/дефреймирует, пишет в приёмник. Интегрировать в main.go с errgroup. Touches: src/cmd/client/main.go, src/cmd/server/main.go, src/internal/transport/websocket/websocket.go, src/internal/tun/tun.go, src/internal/transport/framing/framing.go
- [x] T4.2 Реализовать graceful shutdown: SIGTERM/SIGINT → cancel контекста → errgroup.Wait() → закрытие TUN, WS, TLS listener. Touches: src/cmd/client/main.go, src/cmd/server/main.go

## Фаза 5: Тесты

Цель: automated coverage для всех AC.

- [x] T5.1 Написать unit-тесты: framing round-trip (AC-004), session/IP pool allocate/release/exhaustion (AC-009), handshake message encode/decode (AC-005), auth token validation/rejection (AC-006). Touches: src/internal/transport/framing/framing_test.go, src/internal/session/session_test.go, src/internal/protocol/handshake/handshake_test.go, src/internal/protocol/auth/auth_test.go
- [x] T5.2 Написать integration-тесты: WS echo (AC-002), TLS version check (AC-003), handshake round-trip (AC-005), packet forwarding client→server (AC-007), server→client (AC-008), graceful shutdown (AC-010). Touches: src/internal/transport/websocket/websocket_test.go, src/internal/transport/tls/tls_test.go, src/internal/tun/tun_test.go

## Покрытие критериев приемки

- AC-001 -> T1.2, T4.1, T5.1
- AC-002 -> T2.1, T5.2
- AC-003 -> T2.2, T5.2
- AC-004 -> T1.1, T5.1
- AC-005 -> T3.1, T4.1, T5.1, T5.2
- AC-006 -> T3.2, T5.1
- AC-007 -> T4.1, T5.2
- AC-008 -> T4.1, T5.2
- AC-009 -> T1.3, T5.1
- AC-010 -> T4.1, T4.2, T5.2

## Заметки

- Фаза 1 задачи независимы (T1.1, T1.2, T1.3 можно параллелить).
- Фаза 2 зависит от Фазы 1 только через импорт типов (Frame, TunDevice).
- Фаза 3 зависит от Фазы 2 (WS transport) и T1.3 (IP pool).
- Фаза 4 зависит от всех предыдущих фаз.
- Фаза 5 может выполняться параллельно с Фазой 4 (unit-тесты с T5.1 сразу после Фазы 1-3; integration после T4.1).
- Каждая задача самодостаточна: implement-агент читает только задачи + файлы из Touches.
- Trace-маркеры `@sk-task` и `@sk-test` обязательны на каждом добавлении.
