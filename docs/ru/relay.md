# Режим Relay (ретранслятор)

Режим relay позволяет запустить промежуточный узел, который принимает клиентские подключения и проксирует их на вышестоящий VPN-сервер. Relay работает как прозрачный pipe — не расшифровывает, не обфусцирует и не инспектирует туннельный трафик.

## Архитектура

```
                    ┌──────────────┐      WebSocket       ┌──────────────┐
  WS client ───────▶│              ├─────────────────────▶│              │
                    │    Relay     │                       │    Server    │
  QUIC client ─────▶│  (mode:relay)│      WebSocket       │  (upstream)  │
                    └──────────────┘                       └──────────────┘
```

- **Relay → Server**: всегда WebSocket (одно upstream-соединение на клиента)
- **Client → Relay**: WebSocket (TCP, всегда включён) или QUIC (UDP, опционально)
- Оба транспорта разделяют общий лимит подключений (`max_connections`) и bridge-логику

## Детали транспортов

### WebSocket listener
- HTTP-сервер с TLS, всегда активен
- Path allowlist (`ws_paths`) — запросы на неразрешённые пути возвращают 404
- Стандартный WebSocket upgrade (требуется заголовок `Upgrade: websocket`)

### QUIC listener
- UDP-слушатель на том же порту, что и WS (разные L4-протоколы)
- Активен только при наличии блока `relay.quic` в конфиге
- Без фильтрации path (в QUIC нет понятия HTTP-пути)
- Использует тот же TLS-конфиг, что и WebSocket (требуется TLS 1.3)

## Конфигурация

Минимальный конфиг relay:

```yaml
mode: relay
server: wss://vpn.example.com/tunnel
relay:
  listen: 0.0.0.0:8443
tls:
  verify_mode: insecure
```

С QUIC и кастомными путями:

```yaml
mode: relay
server: wss://vpn.example.com/tunnel
relay:
  listen: 0.0.0.0:8443
  ws_paths:
    - /tunnel
    - /api/v1/events
  max_connections: 200
  quic:
    keep_alive: 7
    idle_timeout: 60
tls:
  verify_mode: insecure
log:
  level: info
```

## Примеры подключения клиентов

### WebSocket клиент (через relay)

```yaml
mode: tun
server: wss://relay:8443/tunnel
auth:
  token: your-token
tls:
  verify_mode: insecure
```

### QUIC клиент (через relay)

```yaml
mode: tun
server: quic://relay:8443
transport: quic
auth:
  token: your-token
tls:
  verify_mode: insecure
```

## Запуск примера

```bash
cd examples/relay
bash run.sh
```

Запускает:
1. **server** — вышестоящий VPN-сервер (порт 443)
2. **relay** — узел ретрансляции с WS + QUIC (порт 8443 TCP+UDP)
3. **client** — WS-клиент, подключающийся через relay
4. **quic-client** — QUIC-клиент, подключающийся через relay

Оба клиента устанавливают туннель через relay к вышестоящему серверу.

## Режим Terminator

Terminator — режим relay, в котором узел выступает полноценным endpoint'ом VPN: принимает клиентов, выделяет IP из пула, поднимает TUN и маршрутизирует трафик. В отличие от bridge-режима (прозрачный pipe), terminator **расшифровывает** и **маршрутизирует** трафик клиента.

### Архитектура

```
                    ┌──────────────┐      Direct CIDR     ┌──────────────┐
                    │              │◀──── ─ ─ ─ ─ ─ ─ ─ ─▶│   Internet   │
  WS client ───────▶│  Terminator  │                       │  (via TUN)   │
                    │ (mode:term)  │      WebSocket       ┌──────────────┐
  QUIC client ─────▶│              ├─────────────────────▶│    Server    │
                    └──────────────┘      (upstream)      │  (upstream)  │
                                                           └──────────────┘
```

- **Direct CIDR** — трафик в указанные диапазоны идёт напрямую через TUN relay.
- **Upstream** — остальной трафик шифруется и отправляется на вышестоящий VPN-сервер.
- Relay выделяет клиентам IP из собственного пула (`relay.network.pool_ipv4`).

### Конфигурация terminator

```yaml
mode: relay
server: wss://vpn.example.com/tunnel
relay:
  mode: terminator
  listen: 0.0.0.0:8443
  routing:
    direct_ranges:
      - 10.0.0.0/8
      - 192.168.0.0/16
    direct_domains:
      - .internal.example
  network:
    pool_ipv4:
      subnet: 172.16.0.0/24
      gateway: 172.16.0.1
tls:
  verify_mode: insecure
```

### Требования

- Relay требует `NET_ADMIN` и `/dev/net/tun` (TUN-устройство).
- `relay.network.pool_ipv4` обязателен для terminator-режима.
- Для upstream-соединения relay использует переменную окружения `KVN_RELAY_AUTH_TOKEN`.

## Примечания

- Relay не требует TUN-устройства или root-прав (самому relay не нужен `NET_ADMIN`)
- TLS-сертификат для входящих подключений: если не настроен, генерируется self-signed
- Relay всегда подключается к upstream через WebSocket, независимо от транспорта клиента
- `idle_timeout` для QUIC обязателен (должен быть > 0) для предотвращения утечки ресурсов
- `keep_alive` по умолчанию 7 секунд, если не указан или равен 0
- WS-клиентам, подключающимся через relay, **необходимо отключить** `obfuscation.padding`: relay — прозрачный pipe и передаст padded-сообщения вышестоящему серверу, что сломает декодирование фреймов. QUIC-клиенты не имеют такого ограничения — они никогда не используют WS padding.
