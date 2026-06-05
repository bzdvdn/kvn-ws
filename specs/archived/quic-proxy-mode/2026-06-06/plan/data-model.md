# QUIC Proxy Mode — Data Model

No changes. `Transport` field in `ClientConfig` and handshake (ClientHello/ServerHello) already exists from the `quic-transport` spec. `tunnel.StreamConn` interface already exists.

## Entities

| Entity | Status |
|--------|--------|
| `config.ClientConfig.Transport` | already exists |
| `handshake.ClientHello.Transport` | already exists (TransportTag 0x0B) |
| `handshake.ServerHello.Transport` | already exists |
| `tunnel.StreamConn` interface | already exists |
| `websocket.WSConn` | already implements StreamConn |
| `quic.QUICConn` | already implements StreamConn |
| `proxy.Manager` | **changes**: field type `*websocket.WSConn` → `tunnel.StreamConn` |
| `proxy.Stream.ForwardToWS()` | **changes**: parameter `*websocket.WSConn` → `tunnel.StreamConn` |

No new types, no new fields, no new contracts.
