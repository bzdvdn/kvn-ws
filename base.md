# VPN over WebSocket (HTTP/HTTPS + WebSocket over TUN) на Go — Конституция проекта

## 1. Миссия проекта

Создать производительный, безопасный и расширяемый VPN-туннель, использующий **стандартный веб-трафик (HTTP/HTTPS + WebSocket)** как транспортный слой для передачи **IP/IPv4/IPv6 пакетов** между клиентом и сервером через **TUN-интерфейсы**.

### Основные цели:

- Маскировка под обычный HTTPS/WebSocket-трафик.
- Работа в сетях с ограничениями, где UDP/VPN-протоколы блокируются.
- Кроссплатформенность: Linux / Windows / macOS.
- Высокая расширяемость (MUX, шифрование, ACL, маршруты).
- Минимальный внешний dependency footprint.

### Не цели (MVP):

- Full mesh.
- GUI.
- QUIC/HTTP3.
- Obfuscation beyond standard TLS.
- Mobile support (позже).

---

# 2. Архитектурные принципы

## 2.1 Базовая модель

```text
[TUN Client] <-> [Client Core] <-> [TLS + WebSocket] <-> [Server Core] <-> [TUN/Router/NAT] <-> Internet/LAN
```

## 2.2 Транспорт:

- TCP 443
- TLS 1.3
- HTTP/1.1 Upgrade или HTTP/2 WebSocket (где возможно)
- WebSocket Binary Frames

## 2.3 Сетевой слой:

- Передача raw IP packets
- IPv4 + IPv6
- Optional packet compression
- Optional multiplexing channels

---

# 3. Репозиторная структура

```text
vpn-ws/
├── cmd/
│   ├── client/
│   │   └── main.go
│   └── server/
│       └── main.go
│
├── internal/
│   ├── config/
│   ├── tun/
│   ├── transport/
│   │   ├── websocket/
│   │   ├── tls/
│   │   └── framing/
│   ├── protocol/
│   │   ├── handshake/
│   │   ├── auth/
│   │   └── control/
│   ├── routing/
│   ├── nat/
│   ├── session/
│   ├── crypto/
│   ├── metrics/
│   └── logger/
│
├── pkg/
│   └── api/
│
├── configs/
│   ├── client.yaml
│   └── server.yaml
│
├── scripts/
├── tests/
├── docs/
└── go.mod
```

---

# 4. Основные компоненты

# 4.1 Client

## Ответственность:

- Создание/управление TUN.
- Захват IP пакетов.
- WebSocket dial через HTTPS.
- Аутентификация.
- Packet encapsulation.
- Keepalive.
- Route management.
- DNS override (опционально).

## Поток:

```text
TUN Read -> Frame Encode -> Encrypt (optional app-layer) -> WS Send
WS Receive -> Decode -> TUN Write
```

---

# 4.2 Server

## Ответственность:

- HTTPS endpoint.
- WebSocket upgrade.
- Client auth.
- Session lifecycle.
- Packet forwarding.
- NAT / IP forwarding.
- ACL.
- Observability.

## Поток:

```text
WS Receive -> Validate -> Route/NAT -> Internet
Internet Reply -> Route Session -> WS Send
```

---

# 5. Протокол уровня приложения

## 5.1 Handshake

### Этапы:

1. TLS established.
2. HTTP Upgrade → WebSocket.
3. Client Hello:

```json
{
  "version": 1,
  "token": "...",
  "device_id": "...",
  "features": ["ipv6", "compression"]
}
```

4. Server Hello:

```json
{
  "session_id": "uuid",
  "assigned_ipv4": "10.10.0.2/24",
  "assigned_ipv6": "fd00::2/64",
  "mtu": 1400
}
```

---

## 5.2 Типы сообщений

### Control Frame:

- AUTH
- PING
- PONG
- ROUTE_UPDATE
- ERROR
- CLOSE

### Data Frame:

- RAW_IP_PACKET

---

# 6. Формат кадра (framing)

```text
+--------+--------+--------+-------------------+
| Type   | Flags  | Length | Payload           |
+--------+--------+--------+-------------------+
| 1 byte | 1 byte | 2/4 b  | Variable          |
```

## Flags:

- Compression
- Encrypted payload
- Fragmented
- Priority

---

# 7. TUN слой

## Linux:

- `/dev/net/tun`
- `ioctl(TUNSETIFF)`

## macOS:

- utun

## Windows:

- Wintun

## Абстракция:

```go
type TunDevice interface {
    ReadPacket() ([]byte, error)
    WritePacket([]byte) error
    Close() error
    MTU() int
}
```

---

# 8. Безопасность

## Обязательно:

- TLS 1.3 only
- Mutual auth optional
- Token/JWT or cert auth
- Replay protection
- Session expiration
- Rate limiting
- Origin validation
- Header camouflage

## Желательно:

- Certificate pinning
- Domain fronting compatibility (если допустимо инфраструктурой)
- Padding

---

# 9. Маршрутизация

## Режимы:

### Full Tunnel:

- Default route через VPN

### Split Tunnel:

- Только выбранные CIDR

## Server:

- iptables/nftables
- IPv6 forwarding
- MASQUERADE/SNAT

---

# 10. Производительность

## Узкие места:

- WebSocket frame overhead
- TCP-over-TCP meltdown
- MTU fragmentation
- GC pressure

## Решения:

- sync.Pool buffers
- Zero-copy where possible
- Batch writes
- PMTU strategy
- Disable Nagle (`TCP_NODELAY`)
- Optional permessage-deflate benchmarking

---

# 11. Конфигурация

## Client YAML:

```yaml
server: https://example.com/ws
auth:
  type: token
  token: secret
  username: client01
mtu: 1400
ipv6: true
auto_reconnect: true
kill_switch: false
log:
  level: info
  file: ./client.log
split_tunnel:
  - 10.0.0.0/8
  - 192.168.1.0/24
dns:
  - 1.1.1.1
  - 8.8.8.8
```

## Server YAML:

```yaml
listen: :443
tls:
  cert: cert.pem
  key: key.pem
  min_version: 1.3
network:
  pool_ipv4:
    subnet: 10.10.0.0/24
    gateway: 10.10.0.1
    range_start: 10.10.0.10
    range_end: 10.10.0.250
  pool_ipv6:
    subnet: fd00::/64
    gateway: fd00::1
session:
  max_clients: 5000
  idle_timeout_sec: 120
  session_ttl_sec: 86400
auth:
  mode: token
  tokens:
    - secret1
    - secret2
  users:
    - username: admin-client
      password_hash: bcrypt_hash_here
      static_ip: 10.10.0.50
acl:
  allow_subnets:
    - 0.0.0.0/0
  deny_subnets:
    - 10.0.0.0/8
logging:
  level: info
  format: json
  file: /var/log/vpn-ws/server.log
metrics:
  prometheus: true
  listen: 127.0.0.1:9090
admin_api:
  enabled: true
  listen: 127.0.0.1:8080
```

## Принципы конфигурации:

- Dynamic reload (SIGHUP)
- Env override support
- Secret separation
- Validation before startup
- Default safe values

---

# 12. Управление IP-пулом клиентов

## Требования:

- Динамическое выделение IPv4/IPv6 адресов.
- Статические IP для отдельных клиентов.
- Lease tracking.
- Reclaim disconnected sessions.
- Conflict prevention.
- Persist state across restarts (BoltDB/SQLite).

## Архитектура:

```text
IP Pool Manager
 ├── Available Pool
 ├── Active Leases
 ├── Reserved Static Assignments
 └── Expired/Reclaimed
```

## Интерфейс:

```go
type IPPoolManager interface {
    Allocate(clientID string) (Lease, error)
    Release(clientID string) error
    Reserve(clientID string, ip net.IP) error
    GetLease(clientID string) (*Lease, error)
    ListActive() []Lease
}
```

## Lease metadata:

- Client ID
- Username
- Assigned IP
- Connected Since
- Last Activity
- Bytes In/Out

---

# 13. Авторизация и управление доступом

## Поддерживаемые режимы:

### MVP:

- Bearer token
- Static token list
- Username/password

### Future:

- mTLS
- OAuth2/OIDC
- SSO
- LDAP

## RBAC роли:

- admin
- operator
- client
- readonly

## ACL:

- CIDR restrictions
- Per-user route policy
- Bandwidth quotas
- Session limits
- Device binding

## Security controls:

- bcrypt/argon2 hashes
- Rate limiting login attempts
- Lockout policy
- Audit trail
- Revocation list

---

# 14. Логи, аудит и observability

## Типы логов:

### System:

- Startup/shutdown
- TLS cert load
- Config reload
- Interface state

### Security:

- Login success/failure
- Token rejection
- ACL deny
- Abuse detection

### Session:

- Connect/disconnect
- Assigned IP
- Throughput
- Session duration

## Формат:

```json
{
  "timestamp": "...",
  "level": "info",
  "session_id": "...",
  "client_id": "...",
  "event": "session_connected"
}
```

## Возможности:

- JSON logs
- Rotation
- Remote syslog
- Loki/ELK integration
- Prometheus + Grafana

## Admin Dashboard API:

- Active clients
- IP assignments
- Session durations
- GeoIP (optional)
- Ban/unban
- Disconnect client

---

# 15. Библиотеки Go (рекомендуемые)

Библиотеки Go (рекомендуемые)

## WebSocket:

- `github.com/gorilla/websocket` (стабильно)
- или `nhooyr.io/websocket`

## Config:

- `spf13/viper`

## Logging:

- `uber-go/zap`

## Metrics:

- `prometheus/client_golang`

## TUN:

- `golang.zx2c4.com/wireguard/tun`

---

# 16. Конкурентная модель

## Goroutines:

- tunReader
- tunWriter
- wsReader
- wsWriter
- keepaliveLoop
- controlLoop

## Каналы:

```go
packetsToWS chan []byte
packetsToTun chan []byte
controlChan chan ControlMessage
```

## Правило:

**Один writer на сокет.**

---

# 17. Ошибки и отказоустойчивость

## Retry strategy:

- Exponential backoff
- Jitter
- Session resume (future)

## Kill-switch:

- При падении VPN блокировать route leakage (опционально)

---

# 18. Наблюдаемость

## Метрики:

- Active sessions
- RTT
- Packet loss
- Throughput
- Reconnect count
- Auth failures

## Логи:

- Structured JSON
- Session-scoped IDs

---

# 19. Тестирование

## Unit:

- Frame encode/decode
- Auth
- Routing
- Config parsing

## Integration:

- Client ↔ Server
- IPv4/IPv6
- Reconnect
- MTU edge cases

## Load:

- 1000+ sessions
- Long-lived idle connections

## Security:

- Fuzz framing
- Malformed packets
- Auth brute force

---

# 20. CI/CD

## Pipeline:

- `go test ./...`
- race detector
- golangci-lint
- fuzz
- cross-build:
  - linux/amd64
  - linux/arm64
  - windows/amd64
  - darwin/arm64

---

# 21. Roadmap

## Phase 1 (MVP)

- TUN
- WebSocket transport
- TLS
- Auth token
- IPv4
- Full tunnel

## Phase 2

- IPv6
- Split tunnel
- Compression
- Prometheus

## Phase 3

- Multiplex channels
- UDP-over-stream strategies
- DPI resistance improvements
- GUI

---

# 22. Кодовые стандарты

## Принципы:

- Context everywhere
- No global mutable state
- Interface-first design
- Graceful shutdown
- Dependency injection
- Structured errors

## Naming:

- `NewClient()`
- `NewServer()`
- `Run(ctx)`

---

# 23. Юридические и operational замечания

- Соблюдать местные законы.
- Не обещать bypass censorship как primary marketing.
- Хранить минимум логов.
- Secrets только через env/secret manager.
- Регулярная ротация сертификатов.

---

# 24. MVP Definition of Done

Проект считается рабочим, когда:

- Клиент поднимает TUN.
- Клиент соединяется по HTTPS/WSS.
- Сервер выдает IP.
- IPv4 трафик идет через туннель.
- DNS не течет (если full tunnel).
- Auto reconnect работает.
- Linux server deploy documented.

---

# 25. Первая реализация (порядок разработки)

## Sprint 1:

- config
- logger
- TUN abstraction

## Sprint 2:

- websocket transport
- framing

## Sprint 3:

- auth + handshake

## Sprint 4:

- routing + NAT

## Sprint 5:

- hardening + metrics

---

# 26. Главный принцип

**Простота транспорта + строгая безопасность + модульность > premature complexity**

Сначала надежный VPN через стандартный WebSocket, затем улучшения маскировки и производительности.
