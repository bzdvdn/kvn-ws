<!-- @sk-task docs-and-release#T1.1: root README with badges and quickstart (AC-006) -->
<!-- @sk-task docs-and-release#T4.1: full README with links to all docs (AC-006) -->

# kvn-ws

[![Go Version](https://img.shields.io/badge/go-1.25%2B-blue)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Release](https://img.shields.io/github/v/release/bzdvdn/kvn-ws)](https://github.com/bzdvdn/kvn-ws/releases)

**kvn-ws** — VPN-туннель через WebSocket/QUIC с маскировкой под обычный веб-трафик. Написан на Go, работает в двух режимах: TUN (VPN) и локальный SOCKS5/HTTP CONNECT прокси. Включает веб-интерфейс (`kvn-web`) для управления конфигурацией и мониторинга.

- Сервер и клиент в одном Docker-образе (multi-stage build)
- TLS 1.3 + mTLS + WebSocket Binary Frames / QUIC
- TUN (VPN) и Proxy (SOCKS5/HTTP CONNECT) режимы
- Transparent proxy (iptables REDIRECT) + DNS proxy (Linux)
- System proxy — автоустановка/восстановление HTTP_PROXY (Linux, macOS, Windows)
- Одноифскация трафика: uTLS (TLS fingerprint), WebSocket padding, QUIC XOR-obfuscation
- SNI rotation — случайный SNI из белого списка при каждом подключении
- Маршрутизация: server/direct, CIDR, DNS-имена, отдельные IP с ordered rules
- DNS-роутинг — маршрутизация DNS-запросов по суффиксу домена
- App-layer шифрование (AES-256-GCM, per-session key derivation)
- Split-tunnel с kill-switch при потере соединения (nftables)
- Dual-stack IPv4/IPv6
- QUIC transport с автоматическим fallback на TCP
- MaxMessageSize защита от OOM в QUIC-транспорте
- Web UI (`kvn-web`) — конфигурация, логи, импорт/экспорт/QR, статус соединения
- Prometheus-метрики, сессионный менеджмент с BoltDB-персистентностью
- CIDR ACL, rate limiting, per-token bandwidth management
- Netlink API для управления маршрутами (без exec.Command)
- SIGHUP hot-reload конфига
- Graceful shutdown, health endpoints (/livez, /readyz, /health)
- Кроссплатформенный клиент: Linux, macOS, Windows (Web UI)

## Quick start (30 sec)

```bash
git clone https://github.com/bzdvdn/kvn-ws.git
cd kvn-ws && cp -r examples/* .
bash examples/run.sh
```

Web UI: http://127.0.0.1:2311

Подробнее: [docs/en/quickstart.md](docs/en/quickstart.md) · [docs/ru/quickstart.md](docs/ru/quickstart.md)

## Documentation

| English                                 | Русский                                |
| --------------------------------------- | -------------------------------------- |
| [Quickstart](docs/en/quickstart.md)     | [Быстрый старт](docs/ru/quickstart.md) |
| [Deployment](docs/en/deployment.md)     | [Развёртывание](docs/ru/deployment.md) |
| [Configuration](docs/en/config.md)      | [Конфигурация](docs/ru/config.md)      |
| [Relay Mode](docs/en/relay.md)          | [Режим Relay](docs/ru/relay.md)        |
| [Architecture](docs/en/architecture.md) | [Архитектура](docs/ru/architecture.md) |

## Installation

### Server (Linux) — одна команда

Устанавливает бинарник, генерирует конфиг со случайным токеном, настраивает systemd-сервис и nftables.

```bash
sudo bash -c "$(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-server.sh)"
```

С опциями (порт, подсеть, IPv6):
```bash
sudo bash -c "$(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-server.sh)" -- --listen :8443 --subnet 10.20.0.0/16 --gateway 10.20.0.1
```

### Client (Linux)

```bash
# Install from release
sudo bash -c "$(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-client.sh)" \
  -- -s wss://vpn.example.com/tunnel -t your-token
```

### Client (Windows)

```powershell
# PowerShell (admin)
iwr -useb https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-client.ps1 -OutFile install-client.ps1
.\install-client.ps1 -Server "wss://vpn.example.com/tunnel" -Token "your-token" -RegisterTask
```

### Web UI (kvn-web) — одна команда

Устанавливает бинарник, регистрирует сервис автозапуска (systemd/launchd/Windows Service) и запускает Web UI на порту 2311.

**Linux / macOS:**
```bash
sudo bash -c "$(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-web.sh)" -- --start
```

**Windows (PowerShell Admin):**
```powershell
iwr -useb https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-web.ps1 -OutFile install-web.ps1; .\install-web.ps1 -Start
```

Web UI: http://127.0.0.1:2311

## Configuration

Минимальный `client.yaml`:

```yaml
server: wss://vpn.example.com/tunnel
auth:
  token: your-token
```

Детальная конфигурация — [docs/ru/config.md](docs/ru/config.md).

## Web UI features

- Редактирование конфигурации через браузер
- Поддержка TUN и Proxy режимов
- Import/Export конфига (JSON, QR-код)
- Мониторинг логов соединения с фильтром по уровню и поиском
- Статус подключения в реальном времени
- Transparent proxy + DNS proxy настройки (Linux)
- System proxy toggle

## Obfuscation

Трафик маскируется под обычный HTTPS несколькими уровнями:

- **uTLS** — подмена TLS fingerprint (Chrome HelloChrome_Auto)
- **Padding** — дополнение WebSocket-сообщений случайными данными до кратного размера
- **QUIC obfuscation** — XOR-обфускация QUIC-трафика с nonce через TLS Exporter
- **SNI rotation** — случайный домен из списка `tls.sni`

## Transport

Два транспорта: TCP (WebSocket) и QUIC (UDP). QUIC используется по умолчанию, при недоступности — автоматический fallback на TCP.

## Relay Mode

Режим ретрансляции (relay) позволяет запустить промежуточный узел, который принимает клиентские подключения и проксирует их на upstream-сервер. Relay работает как прозрачный pipe:

- **WebSocket** (TCP) — всегда активен, поддерживает path allowlist
- **QUIC** (UDP) — опционально, без path filter, разделяет semaphore с WS

Оба транспорта используют общий bridge (`bridgeRelayConn`) и единый лимит подключений (`max_connections`).

### Пример relay

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
```

### Клиент через WS relay

```yaml
mode: tun
server: wss://relay:8443/tunnel
# Obfuscation/padding ОБЯЗАТЕЛЬНО отключить — relay прозрачный pipe,
# padding сломает декодирование фреймов на upstream-сервере.
auth:
  token: your-token
tls:
  verify_mode: insecure
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

Подробнее: [docs/en/relay.md](docs/en/relay.md) · [docs/ru/relay.md](docs/ru/relay.md)

## Examples

Готовые к запуску примеры в [examples/](examples/):

- `docker-compose.yml` — сервер + клиент (WS и QUIC)
- `server.yaml` / `client.yaml` — конфиги
- `relay/docker-compose.yml` — relay-пример (WS + QUIC клиенты)
- `run.sh` — генерация TLS-сертификата и запуск

## Changelog

[CHANGELOG.md](CHANGELOG.md)

## License

MIT
