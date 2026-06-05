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
| Session Manager | `src/internal/session/` | Жизненный цикл сессий, выделение/возврат IP, BoltDB |
| IP Pool | `src/internal/session/` | Динамическое выделение IPv4/IPv6 из подсетей |
| Auth | `src/internal/protocol/auth/` | Аутентификация по токену, JWT и basic |
| Control | `src/internal/protocol/control/` | PING/PONG keepalive, управляющие сообщения |
| NAT | `src/internal/nat/` | nftables MASQUERADE для проброса трафика |
| DNS | `src/internal/dns/` | DNS-резолвер с TTL-кэшем в памяти |
| Metrics | `src/internal/metrics/` | Prometheus-метрики (active_sessions, throughput, errors) |

### Клиент

| Компонент | Пакет | Роль |
|-----------|-------|------|
| TUN Interface | `src/internal/tun/` | Абстракция виртуального сетевого интерфейса |
| Routing Engine | `src/internal/routing/` | RuleSet: server/direct, CIDR, домены, IP с ordered rules |
| Proxy Listener | `src/internal/proxy/` | SOCKS5 + HTTP CONNECT прокси для локального трафика |
| WebSocket Dialer | `src/internal/transport/websocket/` | Клиентское WebSocket-подключение |
| QUIC Dialer | `src/internal/transport/quic/` | QUIC (UDP) dial + ObfuscatedQUICConn |
| DNS Resolver | `src/internal/dns/` | DNS-резолвер с TTL-кэшем |
| Crypto | `src/internal/crypto/` | Шифрование на уровне приложения (AES-256-GCM, per-session key) |

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
2. Клиент устанавливает TLS 1.3 соединение с сервером
3. Клиент отправляет WebSocket upgrade request на путь `/ws`
4. Сервер принимает WebSocket, переключается в бинарный режим
5. Клиент отправляет `ClientHello` (версия протокола, поддерживаемые возможности)
6. Сервер отвечает `ServerHello` (ID сессии, назначенный IP, возможности)
7. Клиент настраивает TUN-интерфейс с полученным IP
8. Routing engine начинает обработку пакетов

#### QUIC (UDP)
1. Клиент читает конфиг из `client.yaml`
2. Клиент открывает QUIC-соединение (встроенный TLS 1.3 handshake) с сервером
3. Клиент открывает единственный QUIC stream
4. Если `obfuscation: true`, клиент генерирует 8-байт nonce и отправляет его первыми байтами stream'а
5. Клиент отправляет `ClientHello` (с XOR-обфускацией length prefix если obfuscation включён)
6. Сервер отвечает `ServerHello` (с XOR-обфускацией)
7. Клиент настраивает TUN-интерфейс с полученным IP
8. Routing engine начинает обработку пакетов

### Передача данных

1. Приложение на клиенте отправляет пакет в TUN-интерфейс
2. Routing engine проверяет правила (ordered): direct или tunnel
3. Для tunnel: пакет инкапсулируется в фрейм (length-prefix + payload), опционально шифруется
4. Если `transport: quic` с `obfuscation: true`, length prefix XOR'ится с nonce перед отправкой
5. Сервер получает фрейм, расшифровывает при необходимости, извлекает пакет
6. Сервер применяет NAT (MASQUERADE) и пересылает пакет получателю
7. Ответ следует обратным путём: сервер получает → инкапсулирует → отправляет (с XOR-обфускацией если включена) → клиент извлекает → инжектирует в TUN

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
│   ├── gatetest/main.go     # Утилита gate-тестирования
│   └── stability/main.go    # Утилита stability/soak-тестирования
├── internal/
│   ├── config/              # YAML конфиг (viper)
│   ├── crypto/              # Шифрование приложения
│   ├── dns/                 # DNS-резолвер + кэш
│   ├── logger/              # Структурированное логирование (zap)
│   ├── metrics/             # Prometheus-метрики
│   ├── nat/                 # nftables MASQUERADE
│   ├── protocol/
│   │   ├── auth/            # Token/JWT/basic аутентификация
│   │   ├── control/         # PING/PONG keepalive
│   │   └── handshake/       # Client/Server Hello
│   ├── proxy/               # SOCKS5 + HTTP CONNECT
│   ├── routing/             # RuleSet engine
│   ├── session/             # Сессии + IP pool + BoltDB
│   ├── transport/
│   │   ├── framing/         # Протокол бинарных фреймов
│   │   ├── quic/            # QUIC dial/listen + ObfuscatedQUICConn
│   │   ├── tls/             # TLS конфиг
│   │   └── websocket/       # WebSocket dial/accept
│   └── tun/                 # TUN-интерфейс
└── pkg/
    └── api/                 # Публичное API (расширяемое)
```
