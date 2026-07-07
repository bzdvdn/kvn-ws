# Режим Relay — Split-Tunnel Terminator

Relay работает как **terminator**: полноценный VPN-endpoint, который принимает клиентские подключения, расшифровывает трафик, маршрутизирует пакеты (direct vs upstream) и управляет TUN-устройством. Это split-tunnel шлюз: трафик в direct CIDR/домены идёт через сеть relay, остальное — через вышестоящий VPN-сервер.

## Архитектура

```
                    ┌──────────────┐      Direct CIDR
                    │              │◀──── ─ ─ ─ ─ ─ ─ ─    Интернет
  WS client ───────▶│  Terminator  │      (через TUN relay)
                    │              │
  QUIC client ─────▶│  (mode:term) │      WebSocket/QUIC   ┌──────────────┐
                    │              ├────────────────────────▶    Server    │
                    │   TUN + NAT  │       (upstream)       │  (upstream)  │
                    └──────────────┘                        └──────────────┘
```

- **Direct CIDR** — трафик в указанные диапазоны идёт напрямую через TUN relay в интернет.
- **Upstream** — остальной трафик шифруется и отправляется на вышестоящий VPN-сервер.
- **NAT** — userspace SNAT/DNAT (`nat.go`): пакеты клиента на direct IP получают source-NAT на IP TUN relay; ответные пакеты проходят reverse-NAT обратно клиенту.

## Детали транспортов

### Клиент → Relay

Клиенты подключаются через WebSocket (TCP, всегда включён) или QUIC (UDP, требуется блок `relay.quic`). Оба транспорта разделяют TLS-конфиг и лимит подключений.

- **WebSocket listener**: HTTP-сервер с TLS, path allowlist (`ws_paths`), стандартный WS upgrade.
- **QUIC listener**: UDP на том же порту, требует TLS 1.3, без фильтрации path.

### Relay → Server (Upstream)

Relay открывает один upstream-туннель к `server`. Транспорт upstream независим от транспорта клиента — задаётся через `transport: quic` или `transport: tcp` (по умолчанию). При падении QUIC — fallback на TCP.

Когда `obfuscation.enabled: true`, upstream QUIC-соединение оборачивается в `ObfuscatedQUICConn` (XOR через TLS keying material). Сервер также должен иметь `obfuscation.enabled: true`.

## Конфигурация

Полный конфиг terminator:

```yaml
mode: relay
server: wss://vpn.example.com/tunnel
transport: quic               # upstream transport: tcp or quic
upstream_token: your-token    # токен для upstream (или env KVN_RELAY_AUTH_TOKEN)

relay:
  mode: terminator
  listen: 0.0.0.0:8443
  ws_paths:
    - /tunnel
  max_connections: 200
  quic:
    keep_alive: 7
    idle_timeout: 60
  routing:
    direct_ranges:
      - 10.0.0.0/8
      - 192.168.0.0/16
    direct_domains:
      - .internal.example
      - .local
    dns:
      upstream: "1.1.1.1:53"
      cache_ttl: 60
      transparent: false
  network:
    pool_ipv4:
      subnet: 172.16.0.0/24
      gateway: 172.16.0.1

obfuscation:
  enabled: true
  padding:
    enabled: true
    size: 512
crypto:
  key: relay-master-key

auth:
  tokens:
    - name: default
      secret: your-token-here
tls:
  verify_mode: insecure
log:
  level: info
```

### DNS Interception

Terminator перехватывает **все** DNS-запросы клиентов (не только direct-доменов):

1. Клиент шлёт DNS-запрос (UDP порт 53) через туннель.
2. Terminator извлекает имя домена через `routing.ParseDNSQuestion`.
3. Если домен совпадает с direct-правилом → форвардится на `dns.upstreams` (по умолчанию `["1.1.1.1:53"]`), resolved IP кэшируются, ответ возвращается напрямую.
4. Если домен НЕ direct → резолвится локально, ответ возвращается напрямую, **без кэширования** (последующие пакеты уходят в upstream).
5. Закэшированные IP bypass'ят upstream-маршрутизацию на время TTL кэша.

Это гарантирует, что direct-доменный трафик никогда не попадёт в upstream-туннель.

### Маршрутизация

Для каждого исходящего пакета `routeOutgoing()` в `handler.go`:

1. **DNS check** — порт 53 → resolve локально (см. выше).
2. **Cache check** — destination IP совпадает с закэшированным direct-domain IP → direct.
3. **CIDR/domain RuleSet** — `ruleSet.Route(ip)` → direct или server.
4. **Direct** → SNAT + запись в TUN.
5. **Server** → SNAT + `upstream.Send()`.

### Userspace NAT

Весь NAT выполняется в userspace (`nat.go`), без iptables:

- **SNAT** в `routeOutgoing`: source IP клиента заменяется на gateway IP TUN relay, назначается случайный порт, сохраняется mapping.
- **DNAT** в `receiveLoop`: на входящие ответы из TUN — поиск оригинального IP:port клиента и обратный mapping.
- Поддерживает TCP, UDP и ICMP.

### Upstream Reconnect

При падении upstream-соединения (`isClosed()` или ошибка `Send`) relay запускает асинхронный reconnect через `reconnectUpstream()`. Reconnect сериализуется `upstreamMu` для предотвращения "thundering herd". Во время переподключения direct-трафик продолжает работать; upstream-трафик дропается с предупреждением.

### Lazy Connect

Relay не падает при недоступности upstream на старте — пишет warn и пробует подключиться при первом клиенте. Клиенты могут подключаться и использовать direct-маршрутизацию сразу; upstream-трафик ждёт установки туннеля.

### Поддержка IPv6

Relay поддерживает IPv6, если настроен `relay.network.pool_ipv6`. Для работы IPv6 нужен и клиент, и relay:

```yaml
relay:
  network:
    pool_ipv4:
      subnet: 172.16.0.0/24
      gateway: 172.16.0.1
    pool_ipv6:
      subnet: "fd00::/112"
      gateway: "fd00::1"
```

Клиенты запрашивают IPv6 через `ipv6: true` в конфиге. Если у relay нет IPv6-пула, клиент работает только по IPv4.

**Важно:** Если клиенту не нужен IPv6, укажите `ipv6: false` — это предотвратит попытки подключения по IPv6 (например curl сначала пробует `2a00:1450:4010:c0f::65:80`, потом IPv4) и ошибку "Сеть недоступна".

## Примеры подключения клиентов

### WebSocket клиент (через relay)

```yaml
mode: tun
server: wss://relay:8443/tunnel
auth:
  token: your-token-here
tls:
  verify_mode: insecure
```

### QUIC клиент (через relay)

```yaml
mode: tun
server: quic://relay:8443
transport: quic
auth:
  token: your-token-here
tls:
  verify_mode: insecure
```

## Запуск примера

```bash
cd examples/relay-terminator
bash run.sh
```

Запускает:
1. **server** — вышестоящий VPN-сервер (порт 443, QUIC-ready)
2. **relay** — terminator с WS + QUIC (порт 8443 TCP+UDP)
3. **client** — WS клиент через relay
4. **quic-client** — QUIC клиент через relay

## Bridge Mode (Legacy)

Существует legacy **bridge**-режим (`relay.mode: bridge`) — прозрачный pipe, который пересылает зашифрованные фреймы без расшифровки. Не требует TUN или NET_ADMIN. Bridge-режим остаётся в `cmd/relay` для обратной совместимости, но не развивается. Для новых развёртываний используйте `terminator`.

## Требования

- `NET_ADMIN` capability и `/dev/net/tun` (TUN-устройство).
- `relay.network.pool_ipv4` обязателен для terminator-режима.
- Для upstream-авторизации: `upstream_token` в конфиге или `KVN_RELAY_AUTH_TOKEN` env.
- Сервер должен иметь `obfuscation.enabled: true`, если relay использует QUIC upstream с обфускацией.
- `net.ipv4.ip_forward=1` включается relay автоматически (для DNAT-ответов ядро должно форвардить TUN→публичный интерфейс).
