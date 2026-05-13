DEC-001: TUN через wireguard/tun (конституция)
DEC-002: WS через gorilla/websocket (roadmap)
DEC-003: Бинарный фрейм Type/Flags/Length/Payload, big-endian
DEC-004: Handshake в один round-trip: ClientHello → ServerHello
DEC-005: Auth через статический bearer-token
DEC-006: IP pool in-memory (map+sync.Mutex)
DEC-007: Forwarding через две goroutine на направление
DEC-008: errgroup для lifecycle всех горутин
Surfaces:
- tun-interface: src/internal/tun/tun.go
- ws-transport: src/internal/transport/websocket/websocket.go
- tls-config: src/internal/transport/tls/tls.go
- framing-protocol: src/internal/transport/framing/framing.go
- handshake-proto: src/internal/protocol/handshake/handshake.go
- auth-logic: src/internal/protocol/auth/auth.go
- session-mgr: src/internal/session/session.go
- client-main: src/cmd/client/main.go
- server-main: src/cmd/server/main.go
