DEC-001: sync.Pool для буферов encode/decode framing
DEC-002: TCP_NODELAY через UnderlyingConn() после upgrade WS
DEC-003: Batch writes — буферизация перед WriteMessage
DEC-004: MTU negotiation — обмен MTU в handshake
DEC-005: PMTU strategy — segmentation при превышении MTU
DEC-006: Permessage-deflate через EnableCompression / SetCompressionLevel
DEC-007: Multiplex через WebSocket subprotocol (feature-flag)
DEC-008: Load testing через отдельный конфиг configs/loadtest.yaml
Surfaces:
- frame-buffer: src/internal/transport/framing/framing.go
- ws-conn: src/internal/transport/websocket/websocket.go
- handshake: src/internal/protocol/handshake/
- config: src/internal/config/ (+ client.go + server.go)
- client-entry: src/cmd/client/main.go
- server-entry: src/cmd/server/main.go
- gatetest: src/cmd/gatetest/main.go
- loadtest-config: configs/loadtest.yaml
