<!-- @sk-task docs-and-release#T4.2: russian translation of config (AC-004) -->

# Конфигурация

kvn-ws использует YAML-файлы конфигурации для сервера и клиента.

## Конфигурация сервера (`server.yaml`)

| Ключ | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `listen` | string | `:443` | Адрес и порт сервера |
| `tls.cert` | string | `cert.pem` | Путь к файлу TLS-сертификата |
| `tls.key` | string | `key.pem` | Путь к файлу приватного TLS-ключа |
| `tls.client_ca_file` | string | `""` | Путь к CA-сертификату для mTLS |
| `tls.client_auth` | string | `""` | Режим mTLS: `request`, `require`, `verify` (требует `client_ca_file`) |
| `network.pool_ipv4.subnet` | string | `10.10.0.0/24` | IPv4 подсеть пула адресов клиентов |
| `network.pool_ipv4.gateway` | string | `10.10.0.1` | IPv4 шлюз |
| `network.pool_ipv4.range_start` | string | subnet+1 | Первый выделяемый IPv4 адрес |
| `network.pool_ipv4.range_end` | string | broadcast-1 | Последний выделяемый IPv4 адрес |
| `network.pool_ipv6.subnet` | string | `fd00::/112` | IPv6 подсеть пула адресов клиентов |
| `network.pool_ipv6.gateway` | string | `fd00::1` | IPv6 шлюз |
| `network.pool_ipv6.range_start` | string | subnet+1 | Первый выделяемый IPv6 адрес |
| `network.pool_ipv6.range_end` | string | broadcast-1 | Последний выделяемый IPv6 адрес |
| `session.max_clients` | int | `100` | Максимальное количество одновременных сессий |
| `session.idle_timeout_sec` | int | `120` | Таймаут бездействия сессии в секундах (0 = отключён) |
| `session.expiry.session_ttl_sec` | int | `86400` | Абсолютный TTL сессии в секундах |
| `session.expiry.reclaim_interval_sec` | int | `10` | Интервал цикла очистки сессий |
| `auth.mode` | string | `token` | Режим аутентификации (`token`, `jwt`, `basic`) |
| `auth.tokens[].name` | string | — | Имя токена для идентификации |
| `auth.tokens[].secret` | string | — | Секретное значение токена |
| `auth.tokens[].bandwidth_bps` | int | `0` | Лимит пропускной способности в bps (0 = безлимит) |
| `auth.tokens[].max_sessions` | int | `0` | Максимум сессий на токен (0 = безлимит) |
| `rate_limiting.auth_burst` | int | `5` | Размер burst для rate-limiter аутентификации на IP |
| `rate_limiting.auth_per_minute` | int | `1` | Лимит запросов аутентификации в минуту на IP |
| `rate_limiting.packets_per_sec` | int | `1000` | Лимит пакетов в секунду на сессию |
| `origin.whitelist` | []string | `[]` | Разрешённые Origin/Referer заголовки (пусто = все) |
| `origin.allow_empty` | bool | `true` | Разрешить запросы без Origin-заголовка |
| `admin.enabled` | bool | `false` | Включить Admin API |
| `admin.listen` | string | `localhost:8443` | Адрес Admin API |
| `admin.token` | string | `""` | Токен аутентификации Admin API |
| `acl.deny_cidrs` | []string | `[]` | CIDR deny список (проверяется перед allow) |
| `acl.allow_cidrs` | []string | `[]` | CIDR allow список (пусто = все разрешены) |
| `ws_paths` | []string | `["/tunnel"]` | Разрешённые пути WebSocket endpoint (404 если не в списке) |
| `transport` | string | `""` | Транспорт: `quic` или пусто (TCP/WebSocket) |
| `obfuscation` | object | — | Настройки обфускации (anti-DPI). См. таблицу сервера |
| `obfuscation.enabled` | bool | `false` | Включить обфускацию |
| `obfuscation.utls.enabled` | bool | `false` | Включить uTLS (Chrome JA3 fingerprint) для WS |
| `obfuscation.utls.fallback` | bool | `true` | При ошибке uTLS — crypto/tls |
| `obfuscation.padding.enabled` | bool | `false` | Включить padding WS (фиксированный размер фреймов) |
| `obfuscation.padding.size` | int | `512` | Размер выравнивания padding |
| `multiplex` | bool | `false` | Включить мультиплексирование WebSocket |
| `mtu` | int | `1400` | MTU TUN-интерфейса |
| `crypto.enabled` | bool | `false` | Включить шифрование AES-256-GCM |
| `crypto.key` | string | `""` | 256-битный мастер-ключ, 64 hex символа (обязателен при enabled) |
| `bolt_db_path` | string | `""` | Путь к BoltDB для персистентности IP-пула (пусто = in-memory) |
| `logging.level` | string | `info` | Уровень логирования (`debug`, `info`, `warn`, `error`) |

### Пример конфигурации сервера

```yaml
listen: :443
ws_paths:
  - /tunnel
  - /api/v1/events
obfuscation:
  enabled: true
  padding:
    enabled: true
    size: 512
tls:
  cert: /etc/kvn-ws/cert.pem
  key: /etc/kvn-ws/key.pem
  min_version: "1.3"
network:
  pool_ipv4:
    subnet: 10.10.0.0/24
    gateway: 10.10.0.1
session:
  max_clients: 100
  idle_timeout_sec: 120
auth:
  mode: token
  tokens:
    - name: default
      secret: your-token-here
logging:
  level: info
```

## Конфигурация клиента (`client.yaml`)

| Ключ | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `server` | string | — | URL WebSocket-сервера (например `wss://example.com/tunnel`) |
| `mode` | string | `tun` | Режим клиента: `tun` (VPN) или `proxy` (локальный SOCKS5/HTTP прокси) |
| `transport` | string | `""` | Транспорт: `quic` или пусто (TCP/WebSocket) |
| `obfuscation` | object | — | Настройки обфускации (anti-DPI). См. таблицу сервера |
| `obfuscation.enabled` | bool | `false` | Включить обфускацию |
| `obfuscation.utls.enabled` | bool | `false` | Включить uTLS (Chrome JA3 fingerprint) для WS |
| `obfuscation.utls.fallback` | bool | `true` | При ошибке uTLS — crypto/tls |
| `obfuscation.padding.enabled` | bool | `false` | Включить padding WS (фиксированный размер фреймов) |
| `obfuscation.padding.size` | int | `512` | Размер выравнивания padding |
| `auth.token` | string | — | Токен аутентификации (должен совпадать с серверным) |
| `tls.verify_mode` | string | `verify` | Режим проверки TLS: `verify`, `insecure` |
| `tls.ca_file` | string | `""` | Файл CA-сертификата (опционально) |
| `tls.server_name` | string | `""` | TLS SNI имя сервера (опционально) |
| `tls.sni` | []string | — | Список SNI доменов (случайный выбор при connect, требуется `verify_mode: insecure`) |
| `mtu` | int | `1400` | MTU TUN-интерфейса |
| `ipv6` | bool | `false` | Включить поддержку IPv6 |
| `auto_reconnect` | bool | `true` | Автоматическое переподключение при разрыве |
| `multiplex` | bool | `false` | Включить мультиплексирование WebSocket |
| `crypto.enabled` | bool | `false` | Включить шифрование AES-256-GCM |
| `crypto.key` | string | `""` | 256-битный мастер-ключ, 64 hex символа (должен совпадать с серверным) |
| `proxy_listen` | string | `127.0.0.1:2310` | Адрес SOCKS5/HTTP прокси (только режим proxy) |
| `proxy_auth.username` | string | `""` | Имя пользователя прокси (опционально) |
| `proxy_auth.password` | string | `""` | Пароль прокси (опционально) |
| `routing.default_route` | string | `server` | Режим маршрутизации по умолчанию (`server`, `direct`) |
| `routing.include_ranges` | []string | `[]` | CIDR-диапазоны для маршрутизации через VPN |
| `routing.exclude_ranges` | []string | `[]` | CIDR-диапазоны для исключения из VPN |
| `routing.include_ips` | []string | `[]` | Конкретные IP для маршрутизации через VPN |
| `routing.exclude_ips` | []string | `[]` | Конкретные IP для исключения из VPN |
| `routing.include_domains` | []string | `[]` | Домены для маршрутизации через VPN |
| `routing.exclude_domains` | []string | `[]` | Домены для исключения из VPN |
| `max_message_size` | int | `10485760` | Максимальный размер QUIC-сообщения в байтах (10 MB). Защита от OOM. 0 = по умолчанию |
| `tunnel_timeout` | int | `30` | Таймаут бездействия туннеля в секундах (0 = по умолчанию) |
| `proxy_max_concurrency` | int | `1000` | Максимум одновременных прокси-соединений (только mode=proxy, 0 = по умолч.) |
| `kill_switch.enabled` | bool | `false` | Блокировать весь трафик при разрыве (nftables) |
| `reconnect.min_backoff_sec` | int | `1` | Минимальная задержка переподключения в секундах |
| `reconnect.max_backoff_sec` | int | `30` | Максимальная задержка переподключения в секундах |
| `system_proxy` | bool | `false` | Автоуправление системными прокси (Linux/macOS/Windows) |
| `transparent` | bool | `false` | Прозрачный прокси через iptables REDIRECT (только Linux) |
| `dns_proxy.listen` | string | `127.0.0.54:53` | Адрес DNS-прокси |
| `dns_proxy.upstream` | string | `1.1.1.1:53` | Вышестоящий DNS-сервер |
| `log.level` | string | `info` | Уровень логирования (`debug`, `info`, `warn`, `error`) |

### Пример конфигурации клиента

```yaml
server: wss://vpn.example.com/api/v1/events
auth:
  token: your-token-here
obfuscation:
  enabled: true
  utls:
    enabled: true
    fallback: true
  padding:
    enabled: true
    size: 512
tls:
  verify_mode: insecure
  sni:
    - www.cloudflare.com
    - www.google.com
mtu: 1400
ipv6: false
auto_reconnect: true
log:
  level: info
routing:
  default_route: server
  include_ranges:
    - 10.0.0.0/8
  exclude_domains:
    - example.com
max_message_size: 10485760
tunnel_timeout: 30
proxy_max_concurrency: 1000
```

## Конфигурация relay (`mode: relay`)

При `mode: relay` клиент работает как промежуточный узел, принимающий WebSocket (TCP) и опционально QUIC (UDP) подключения и проксирующий их на вышестоящий сервер.

| Ключ | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `relay.listen` | string | — | Адрес и порт для входящих подключений (обязателен) |
| `relay.ws_paths` | []string | `["/tunnel"]` | Разрешённые WebSocket path (404 если не в списке) |
| `relay.max_connections` | int | `100` | Максимум одновременных входящих подключений (общий для WS и QUIC) |
| `relay.tls.cert` | string | auto (self-signed) | Путь к TLS сертификату для WS и QUIC |
| `relay.tls.key` | string | auto (self-signed) | Путь к приватному ключу |
| `relay.quic.keep_alive` | int | `7` | KeepAlive период QUIC в секундах |
| `relay.quic.idle_timeout` | int | — | Idle timeout QUIC в секундах (обязателен, >0) |

### Пример

```yaml
mode: relay
server: wss://vpn.example.com/tunnel
relay:
  listen: 0.0.0.0:8443
  ws_paths:
    - /tunnel
  max_connections: 200
  quic:
    keep_alive: 7
    idle_timeout: 60
tls:
  verify_mode: insecure
log:
  level: info
```

### Клиент через QUIC relay

```yaml
mode: tun
server: quic://relay:8443
transport: quic
auth:
  token: your-token
tls:
  verify_mode: insecure
```

Подробнее: [docs/ru/relay.md](relay.md)

## Переменные окружения

Все ключи конфигурации можно переопределить переменными окружения с префиксом `KVN_SERVER_` / `KVN_CLIENT_`, где точки заменены на подчёркивания:

```bash
KVN_SERVER_LISTEN=:8443
KVN_SERVER_CRYPTO_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
KVN_CLIENT_AUTH_TOKEN=my-secret-token
KVN_CLIENT_LOG_LEVEL=debug
```

Для токенов аутентификации в production используйте JSON-массив через переменную окружения:

```bash
KVN_SERVER_AUTH_TOKENS_JSON='[{"name":"admin","secret":"a1b2c3"},{"name":"guest","secret":"x9y8z7"}]'
```

## Конфигурация Web UI (`webui.yaml`)

kvn-web хранит конфигурацию в `~/.config/kvn-ws/webui.yaml`. Формат расширяет `ClientConfig` поддержкой нескольких серверов:

```yaml
# Глобальные настройки (применяются ко всем серверам)
mode: proxy
proxy_listen: 127.0.0.1:2310
mtu: 1400
log:
  level: info
routing:
  default_route: server
  exclude_ranges:
    - 10.0.0.0/8
    - 172.16.0.0/12
    - 192.168.0.0/16

# Активный сервер
active_server: Work

# Конфигурации серверов
servers:
  - name: Work
    server: wss://vpn.company.com/tunnel
    auth:
      token: work-token
    transport: quic
    tls:
      verify_mode: verify
      server_name: vpn.company.com
    routing:
      default_route: direct

  - name: Home
    server: wss://vpn.example.com/tunnel
    auth:
      token: home-token
    tls:
      verify_mode: insecure
```

| Ключ | Тип | По умолч. | Описание |
|------|-----|-----------|----------|
| `active_server` | string | имя первого сервера | Имя выбранного сервера |
| `servers[].name` | string | — | Имя сервера (уникальный идентификатор) |
| `servers[].*` | — | — | Все поля `ClientConfig` (server, auth, tls, routing и т.д.) |
