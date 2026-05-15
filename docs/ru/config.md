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
| `compression` | bool | `false` | Включить сжатие WebSocket per-message |
| `multiplex` | bool | `false` | Включить мультиплексирование WebSocket |
| `mtu` | int | `1400` | MTU TUN-интерфейса |
| `crypto.enabled` | bool | `false` | Включить шифрование AES-256-GCM |
| `crypto.key` | string | `""` | 256-битный мастер-ключ, 64 hex символа (обязателен при enabled) |
| `bolt_db_path` | string | `""` | Путь к BoltDB для персистентности IP-пула (пусто = in-memory) |
| `logging.level` | string | `info` | Уровень логирования (`debug`, `info`, `warn`, `error`) |

### Пример конфигурации сервера

```yaml
listen: :443
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
| `auth.token` | string | — | Токен аутентификации (должен совпадать с серверным) |
| `tls.verify_mode` | string | `verify` | Режим проверки TLS: `verify`, `skip` |
| `tls.ca_file` | string | `""` | Файл CA-сертификата (опционально) |
| `tls.server_name` | string | `""` | TLS SNI имя сервера (опционально) |
| `mtu` | int | `1400` | MTU TUN-интерфейса |
| `ipv6` | bool | `false` | Включить поддержку IPv6 |
| `auto_reconnect` | bool | `true` | Автоматическое переподключение при разрыве |
| `compression` | bool | `false` | Включить сжатие WebSocket per-message |
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
| `kill_switch.enabled` | bool | `false` | Блокировать весь трафик при разрыве (nftables) |
| `reconnect.min_backoff_sec` | int | `1` | Минимальная задержка переподключения в секундах |
| `reconnect.max_backoff_sec` | int | `30` | Максимальная задержка переподключения в секундах |
| `log.level` | string | `info` | Уровень логирования (`debug`, `info`, `warn`, `error`) |

### Пример конфигурации клиента

```yaml
server: wss://vpn.example.com/tunnel
auth:
  token: your-token-here
tls:
  verify_mode: verify
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
```

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
