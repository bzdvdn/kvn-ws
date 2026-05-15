<!-- @sk-task docs-and-release#T1.1: root README with badges and quickstart (AC-006) -->
<!-- @sk-task docs-and-release#T4.1: full README with links to all docs (AC-006) -->

# kvn-ws

[![Go Version](https://img.shields.io/badge/go-1.25%2B-blue)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Release](https://img.shields.io/github/v/release/bzdvdn/kvn-ws)](https://github.com/bzdvdn/kvn-ws/releases)

**kvn-ws** — VPN-туннель через HTTPS/WebSocket с маскировкой под обычный веб-трафик. Написан на Go, работает через TUN-интерфейс.

- Сервер и клиент в одном Docker-образе (multi-stage build)
- TLS 1.3 + WebSocket Binary Frames
- Маршрутизация: server/direct, CIDR, DNS-имена, отдельные IP с ordered rules
- Prometheus-метрики, сессионный менеджмент с IP-пулом

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
| [Configuration](docs/en/config.md)      | [Конфигурация](docs/ru/config.md)      |
| [Architecture](docs/en/architecture.md) | [Архитектура](docs/ru/architecture.md) |

## Examples

Готовые к запуску примеры в [examples/](examples/):

- `docker-compose.yml` — сервер + клиент
- `server.yaml` / `client.yaml` — конфиги
- `run.sh` — генерация TLS-сертификата и запуск

## Changelog

[CHANGELOG.md](CHANGELOG.md)

## License

MIT
