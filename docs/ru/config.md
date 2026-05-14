<!-- @sk-task docs-and-release#T4.2: russian translation of config (AC-004) -->

# Конфигурация

kvn-ws использует YAML-файлы конфигурации для сервера и клиента.

## Конфигурация сервера (`server.yaml`)

| Ключ | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `listen` | string | `:443` | Адрес и порт сервера |
| `tls.cert` | string | `cert.pem` | Путь к файлу TLS-сертификата |
| `tls.key` | string | `key.pem` | Путь к файлу приватного TLS-ключа |
| `tls.min_version` | string | `"1.3"` | Минимальная версия TLS |
| `network.pool_ipv4.subnet` | string | `10.10.0.0/24` | IPv4 подсеть пула адресов клиентов |
| `network.pool_ipv4.gateway` | string | `10.10.0.1` | IPv4 шлюз |
| `network.pool_ipv6.subnet` | string | `fd00::/112` | IPv6 подсеть пула адресов клиентов |
| `network.pool_ipv6.gateway` | string | `fd00::1` | IPv6 шлюз |
| `session.max_clients` | int | `100` | Максимальное количество одновременных сессий |
| `session.idle_timeout_sec` | int | `120` | Таймаут бездействия сессии в секундах |
| `auth.mode` | string | `token` | Режим аутентификации (`token`, `jwt`, `basic`) |
| `auth.tokens[].name` | string | — | Имя токена для идентификации |
| `auth.tokens[].secret` | string | — | Секретное значение токена |
| `auth.tokens[].bandwidth_bps` | int | `0` | Лимит пропускной способности в bps (0 = безлимит) |
| `auth.tokens[].max_sessions` | int | `0` | Максимум сессий на токен (0 = безлимит) |
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
| `server` | string | — | URL WebSocket-сервера (например `https://example.com/ws`) |
| `auth.token` | string | — | Токен аутентификации (должен совпадать с серверным) |
| `mtu` | int | `1400` | MTU TUN-интерфейса |
| `ipv6` | bool | `false` | Включить поддержку IPv6 |
| `auto_reconnect` | bool | `true` | Автоматическое переподключение при разрыве |
| `log.level` | string | `info` | Уровень логирования (`debug`, `info`, `warn`, `error`) |
| `routing.default_route` | string | `server` | Режим маршрутизации по умолчанию (`server`, `direct`) |
| `routing.include_ranges` | []string | `[]` | CIDR-диапазоны для маршрутизации через VPN |
| `routing.exclude_ranges` | []string | `[]` | CIDR-диапазоны для исключения из VPN |
| `routing.include_ips` | []string | `[]` | Конкретные IP для маршрутизации через VPN |
| `routing.exclude_ips` | []string | `[]` | Конкретные IP для исключения из VPN |
| `routing.include_domains` | []string | `[]` | Домены для маршрутизации через VPN |
| `routing.exclude_domains` | []string | `[]` | Домены для исключения из VPN |

### Пример конфигурации клиента

```yaml
server: https://vpn.example.com/ws
auth:
  token: your-token-here
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

Все ключи конфигурации можно переопределить переменными окружения с префиксом `KVN_`, где точки заменены на подчёркивания:

```bash
KVN_LISTEN=:8443
KVN_AUTH_TOKEN=my-secret-token
KVN_LOG_LEVEL=debug
```
