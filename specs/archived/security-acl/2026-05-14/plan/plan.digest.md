DEC-001: CIDR-фильтрация после TLS на http.Handler уровне (не pre-TLS)
DEC-002: Структурированная конфигурация токенов (map[string]TokenCfg вместо []string)
DEC-003: Конфигурируемый CheckOrigin через websocket.Accept с whitelist и glob-паттернами
DEC-004: Admin API на chi router с локальным HTTP-сервером (localhost:port)
DEC-005: Bandwidth quota — байтовый rate.Limiter на write path (TUN→WS)
DEC-006: mTLS — опционально через tls.ClientAuth в конфиге TLS
Surfaces:
- acl-package: src/internal/acl/acl.go — CIDR matcher, allow/deny lists
- token-config: src/internal/config/server.go, src/internal/config/server_test.go — TokenCfg struct
- token-config-migration: src/internal/protocol/auth/auth.go — ValidateToken под новую структуру
- websocket-upgrade: src/internal/transport/websocket/websocket.go — CheckOrigin как параметр Accept
- admin-api: src/cmd/server/main.go — Admin HTTP server (chi), handlers, X-Admin-Token middleware
- bandwidth-limiter: src/internal/session/bandwidth.go — per-token bandwidth rate.Limiter
- tls-mtls: src/internal/transport/tls/tls.go — ClientCAFile, ClientAuth
