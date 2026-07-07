<!-- @sk-task docs-and-release#T4.2: russian translation of architecture (AC-004) -->

# Архитектура

kvn-ws — VPN-туннель поверх HTTPS/WebSocket и QUIC, написанный на Go. Этот документ описывает системную архитектуру, компоненты и потоки данных.

## Обзор

```mermaid
flowchart TB
    subgraph Client["Хост клиента"]
        TUN["TUN IFace"]
        PROXY["Proxy / Routing"]
        WS_CLIENT["WebSocket Transport"]
        QUIC_CLIENT["QUIC Transport"]
        OBF["QUIC Obfuscation"]
        TUN <--> PROXY
        PROXY <--> WS_CLIENT
        PROXY <--> QUIC_CLIENT
        QUIC_CLIENT <--> OBF
    end

    subgraph Internet["Интернет"]
        WS_TUNNEL["TLS 1.3 / WebSocket Binary (TCP 443)"]
        QUIC_TUNNEL["QUIC / TLS 1.3 (UDP 443)"]
    end

    subgraph Server["Сервер"]
        TLS["TLS Listener (TCP)"]
        QUIC_LN["QUIC Listener (UDP)"]
        SESS["Session Manager"]
        IPPOOL["IP Pool"]
        AUTH["Auth"]
        NAT["NAT / Routing<br/>(nftables MASQUERADE)"]
        PHYS["Physical Network"]
        TLS --> SESS
        QUIC_LN --> SESS
        SESS --> IPPOOL
        SESS --> AUTH
        SESS --> NAT
        NAT --> PHYS
    end

    WS_CLIENT -- "WSS" --> WS_TUNNEL
    WS_TUNNEL --> TLS
    OBF -- "QUIC + obfuscation" --> QUIC_TUNNEL
    QUIC_TUNNEL --> QUIC_LN
```

## Компоненты

### Сервер

| Компонент | Пакет | Роль |
|-----------|-------|------|
| TLS Listener | `src/internal/transport/tls/` | Терминация TLS 1.3 (TCP) |
| WebSocket Acceptor | `src/internal/transport/websocket/` | WebSocket upgrade и ввод/вывод бинарных фреймов |
| QUIC Listener | `src/internal/transport/quic/` | QUIC (UDP) listener + ObfuscatedQUICConn |
| Bootstrap | `src/internal/bootstrap/` | Оркестрация сервера: TLS, QUIC, менеджер сессий |
| Session Manager | `src/internal/session/` | Жизненный цикл сессий, выделение/возврат IP, BoltDB |
| IP Pool | `src/internal/session/` | Динамическое выделение IPv4/IPv6 из подсетей |
| Auth | `src/internal/protocol/auth/` | Аутентификация по токену, JWT и basic |
| Control | `src/internal/protocol/control/` | PING/PONG keepalive, управляющие сообщения |
| Admin API | `src/internal/admin/` | HTTP API для управления сессиями и pprof |
| NAT | `src/internal/nat/` | nftables/iptables MASQUERADE (авто-определение) |
| DNS | `src/internal/dns/` | DNS-резолвер с TTL-кэшем в памяти |
| Metrics | `src/internal/metrics/` | Prometheus-метрики (active_sessions, throughput, errors) |
| Rate Limiter | `src/internal/ratelimit/` | IP-ограничитель скорости (token bucket) |
| ACL | `src/internal/acl/` | Контроль доступа по IP через CIDR-правила |

### Клиент

| Компонент | Пакет | Роль |
|-----------|-------|------|
| TUN Interface | `src/internal/tun/` | Абстракция виртуального сетевого интерфейса |
| Routing Engine | `src/internal/routing/` | RuleSet: server/direct, CIDR, домены, IP с ordered rules |
| Tunnel Session | `src/internal/tunnel/` | Туннельная сессия: связывает TUN, crypto, proxy, transport |
| Bootstrap | `src/internal/bootstrap/` | Оркестрация клиента: TUN, DNS, proxy, transport |
| Proxy Listener | `src/internal/proxy/` | SOCKS5 + HTTP CONNECT прокси для локального трафика |
| Transparent Proxy | `src/internal/transparent/` | Прозрачный прокси через iptables REDIRECT (Linux) |
| System Proxy | `src/internal/systemproxy/` | Управление системными прокси (Linux/macOS/Windows) |
| DNS Proxy | `src/internal/dnsproxy/` | Прокси-сервер DNS для трафика VPN |
| WebSocket Dialer | `src/internal/transport/websocket/` | Клиентское WebSocket-подключение с опциональным padding |
| uTLS Dialer | `src/internal/transport/tls/` | Браузерный TLS (uTLS, Chrome JA3), кастомный выбор SNI |
| QUIC Dialer | `src/internal/transport/quic/` | QUIC (UDP) dial + ObfuscatedQUICConn |
| DNS Resolver | `src/internal/dns/` | DNS-резолвер с TTL-кэшем |
| Crypto | `src/internal/crypto/` | Шифрование на уровне приложения (AES-256-GCM, per-session key) |
| Web UI | `src/internal/webui/` | Локальный веб-интерфейс (React + REST API для конфига/подключения) |

### Общие

| Компонент | Пакет | Роль |
|-----------|-------|------|
| Config | `src/internal/config/` | Парсинг YAML через viper с переопределением через env |
| Logger | `src/internal/logger/` | Структурированное JSON-логирование через zap |
| Framing | `src/internal/transport/framing/` | Протокол бинарных фреймов (сообщения с префиксом длины) |
| Handshake | `src/internal/protocol/handshake/` | Client/Server Hello для согласования протокола |

## Потоки данных

### Установка соединения (handshake)

Выбор транспорта определяется полем `transport` в конфиге:
- `""` (пусто) или `"tcp"` — WebSocket поверх TLS 1.3 (TCP 443)
- `"quic"` — QUIC поверх TLS 1.3 (UDP 443)

#### WebSocket (TCP)
1. Клиент читает конфиг из `client.yaml`
2. Клиент устанавливает TLS 1.3 соединение с сервером; если `uTLS.enabled` — использует Chrome JA3 fingerprint через `utls.HelloChrome_Auto`; если задан `tls.sni` — выбирает случайный домен при каждом connect
3. Клиент отправляет WebSocket upgrade request с путём из URL (напр. `/api/v1/events`, а не жёстко `/ws`)
4. Сервер проверяет path по `ws_paths` allowlist (404 если неизвестен), принимает WebSocket upgrade в бинарный режим
5. Клиент отправляет `ClientHello` (версия протокола, поддерживаемые возможности)
6. Сервер отвечает `ServerHello` (ID сессии, назначенный IP, возможности)
7. Клиент настраивает TUN-интерфейс с полученным IP
8. Routing engine начинает обработку пакетов

#### QUIC (UDP)
1. Клиент читает конфиг из `client.yaml`
2. Клиент открывает QUIC-соединение (встроенный TLS 1.3 handshake) с сервером; если задан `tls.sni` — выбирает случайный домен при каждом connect
3. Клиент открывает единственный QUIC stream
4. После handshake обе стороны вычисляют 8-байт nonce через TLS Exporter (`ExportKeyingMaterial("kvn-obfuscation", nil, 8)`) — 0 байт на wire
5. Клиент отправляет `ClientHello` с XOR-обфускацией всего payload (не только length prefix)
6. Сервер отвечает `ServerHello` (XOR всего payload)
7. Клиент настраивает TUN-интерфейс с полученным IP
8. Routing engine начинает обработку пакетов

### Передача данных

1. Приложение на клиенте отправляет пакет в TUN-интерфейс
2. Routing engine проверяет правила (ordered): direct или tunnel
3. Для tunnel: пакет инкапсулируется в фрейм (length-prefix + payload), опционально шифруется; для WS транспорта с `padding.enabled: true` — фрейм оборачивается в `[4B длина][payload][random padding]` с выравниванием до `padding.size`
4. Если `transport: quic` с `obfuscation: true`, весь payload XOR'ится с nonce от TLS Exporter (не только length prefix)
5. Сервер получает фрейм, отбрасывает WS padding если включён, расшифровывает при необходимости, извлекает пакет
6. Сервер применяет NAT (MASQUERADE) и пересылает пакет получателю
7. Ответ следует обратным путём: сервер получает → инкапсулирует → отправляет (с XOR всего payload если включена обфускация) → клиент извлекает → инжектирует в TUN

### Keepalive

- Клиент периодически отправляет PING-фреймы
- Сервер отвечает PONG
- При отсутствии активности в течение `session.idle_timeout_sec` сервер завершает сессию

## Структура кода

```
src/
├── cmd/
│   ├── client/main.go       # Точка входа клиента
│   ├── server/main.go       # Точка входа сервера
│   ├── web/main.go          # Точка входа Web UI (kvn-web)
│   ├── gatetest/main.go     # Утилита gate-тестирования
│   └── stability/main.go    # Утилита stability/soak-тестирования
├── internal/
│   ├── acl/                 # Контроль доступа по CIDR
│   ├── admin/               # Admin HTTP API (сессии, pprof)
│   ├── bootstrap/           # Оркестрация клиента/сервера
│   ├── config/              # YAML конфиг (viper)
│   ├── crypto/              # Шифрование приложения
│   ├── dns/                 # DNS-резолвер + кэш
│   ├── dnsproxy/            # Прокси-сервер DNS
│   ├── logger/              # Структурированное логирование (zap)
│   ├── metrics/             # Prometheus-метрики
│   ├── nat/                 # nftables/iptables MASQUERADE
│   ├── protocol/
│   │   ├── auth/            # Token/JWT/basic аутентификация
│   │   ├── control/         # PING/PONG keepalive
│   │   └── handshake/       # Client/Server Hello
│   ├── proxy/               # SOCKS5 + HTTP CONNECT
│   ├── ratelimit/           # IP-ограничитель скорости (token bucket)
│   ├── routing/             # RuleSet engine
│   ├── session/             # Сессии + IP pool + BoltDB
│   ├── systemproxy/         # Управление системными прокси
│   ├── transparent/         # Прозрачный прокси через iptables (Linux)
│   ├── transport/
│   │   ├── framing/         # Протокол бинарных фреймов
│   │   ├── quic/            # QUIC dial/listen + ObfuscatedQUICConn
│   │   ├── tls/             # TLS конфиг
│   │   └── websocket/       # WebSocket dial/accept
│   ├── tun/                 # TUN-интерфейс
│   ├── tunnel/              # Туннельная сессия VPN
│   └── webui/               # Web UI (React + REST API)
└── pkg/
    └── api/                 # Публичное API (расширяемое)
```
