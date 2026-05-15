<!-- @sk-task docs-and-release#T1.1: root README with badges and quickstart (AC-006) -->
<!-- @sk-task docs-and-release#T4.1: full README with links to all docs (AC-006) -->

# kvn-ws

[![Go Version](https://img.shields.io/badge/go-1.25%2B-blue)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Release](https://img.shields.io/github/v/release/bzdvdn/kvn-ws)](https://github.com/bzdvdn/kvn-ws/releases)

**kvn-ws** — VPN-туннель через HTTPS/WebSocket с маскировкой под обычный веб-трафик. Написан на Go, работает через TUN-интерфейс. Поддерживает два режима: TUN (VPN) и локальный SOCKS5/HTTP CONNECT прокси.

- Сервер и клиент в одном Docker-образе (multi-stage build)
- TLS 1.3 + mTLS + WebSocket Binary Frames
- Маршрутизация: server/direct, CIDR, DNS-имена, отдельные IP с ordered rules
- App-layer шифрование (AES-256-GCM, per-session key derivation)
- Split-tunnel с kill-switch при потере соединения (nftables)
- Dual-stack IPv4/IPv6
- Prometheus-метрики, сессионный менеджмент с BoltDB-персистентностью
- SOCKS5 + HTTP CONNECT proxy mode
- CIDR ACL, rate limiting, per-token bandwidth management
- SIGHUP hot-reload конфига
- Graceful shutdown, health endpoints (/livez, /readyz, /health)

## Quick start (30 sec)

```bash
git clone https://github.com/bzdvdn/kvn-ws.git
cd kvn-ws && cp -r examples/* .
bash examples/run.sh
```

Подробнее: [docs/en/quickstart.md](docs/en/quickstart.md) · [docs/ru/quickstart.md](docs/ru/quickstart.md)

## Documentation

| English                                 | Русский                                |
| --------------------------------------- | -------------------------------------- |
| [Quickstart](docs/en/quickstart.md)     | [Быстрый старт](docs/ru/quickstart.md) |
| [Deployment](docs/en/deployment.md)     | [Развёртывание](docs/ru/deployment.md) |
| [Configuration](docs/en/config.md)      | [Конфигурация](docs/ru/config.md)      |
| [Architecture](docs/en/architecture.md) | [Архитектура](docs/ru/architecture.md) |

### Server quick install (Linux)

```bash
sudo bash -c "$(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-server.sh)"
```

### Client quick install (Windows)

```powershell
# PowerShell (admin)
iwr -useb https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-client.ps1 -OutFile install-client.ps1
.\install-client.ps1 -Server "wss://vpn.example.com/tunnel" -Token "your-token" -RegisterTask
```

Подробнее: [docs/en/deployment.md](docs/en/deployment.md)

## Examples

Готовые к запуску примеры в [examples/](examples/):

- `docker-compose.yml` — сервер + клиент
- `server.yaml` / `client.yaml` — конфиги
- `run.sh` — генерация TLS-сертификата и запуск

## Changelog

[CHANGELOG.md](CHANGELOG.md)

## License

MIT
