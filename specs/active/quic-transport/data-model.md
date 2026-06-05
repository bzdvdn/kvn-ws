# QUIC Transport: Data Model

## No structural changes to core data model

The existing `config.ClientConfig`, `config.ServerConfig`, `protocol.Frame`, `handshake.Hello` structs remain unchanged.

## Additions

| Entity | Field | Type | Default | Description |
|--------|-------|------|---------|-------------|
| `config.ClientConfig` | `Transport` | `string` | `"tcp"` | Transport protocol: `"tcp"` or `"quic"` |
| `config.ServerConfig` | `Transport` | `string` | `"tcp"` | Transport protocol: `"tcp"` or `"quic"` |
| `handshake.ClientHello` | `Transport` | `string` | `"tcp"` | Transport preference sent by client |
| `handshake.ServerHello` | `Transport` | `string` | `"tcp"` | Transport confirmed by server |

## New interface

```go
// StreamConn abstracts transport-level message exchange.
// Both *websocket.WSConn and *quic.QUICConn implement this.
type StreamConn interface {
    ReadMessage() ([]byte, error)
    WriteMessage([]byte) error
    Close() error
}
```

## No changes to

- `protocol.Frame` types
- `tunnel.Session` logic
- crypto/encryption layer
- routing/proxy structs
