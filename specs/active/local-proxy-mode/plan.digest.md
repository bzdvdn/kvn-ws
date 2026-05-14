DEC-001: SOCKS5 через net — stdlib, без внешних зависимостей
DEC-002: FrameTypeProxy + streamID — мультиплексирование TCP-потоков
DEC-003: Серверный TCP-forwarder — горутина на каждый прокси-поток
DEC-004: Exclusion через Route() — переиспользование существующего routing
Surfaces:
- frame-type: src/internal/transport/framing/framing.go (+ FrameTypeProxy)
- proxy-listener: src/internal/proxy/ (новый пакет — SOCKS5 + CONNECT)
- proxy-stream: src/internal/proxy/stream.go (streamID, forwarding)
- mode-config: src/internal/config/client.go (+ Mode, ProxyListen, ProxyAuth)
- client-entry: src/cmd/client/main.go (mode switch)
- server-entry: src/cmd/server/main.go (FrameTypeProxy handler)
