DEC-001: offset=4 для tun.Device.Write() — 4 байта AF_INET header для Linux TUN
DEC-002: Single-buf Read вместо batch — устраняем коллизию buffer offset
DEC-003: MockTunDevice для unit-тестов — изолированные тесты без /dev/net/tun
Surfaces:
- src/internal/tun/tun.go: fix Write offset, fix Read single-buf
- src/internal/tun/tun_test.go: new file — MockTunDevice + unit tests
- src/cmd/client/main.go: tunToWS buffer with headroom
- src/cmd/server/main.go: serverWSToTun buffer with headroom
