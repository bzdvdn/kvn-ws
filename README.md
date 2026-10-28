# kvn-ws 2.0

**kvn-ws 2.0** — v2-only VPN: TLS 1.3 + HTTP/2 TCP split, QUIC datagrams для UDP, TUN и локальный proxy. v1 удалён из текущего дерева; старые v1-сборки доступны в [GitHub Releases](https://github.com/bzdvdn/kvn-ws/releases).

## Компоненты

- `kvn-server` — v2-сервер (TCP + HTTP/2, optional QUIC UDP).
- `kvn-client` — standalone v2-клиент (TUN/proxy).
- `kvn-web` — v2-only web UI; конфиг в `~/.config/kvn/config.yaml`.
- `kvn-desktop` — нативный wrapper для `kvn-web`.
- Android-клиент — переводится на v2 в рамках 2.0.0.

## Быстрый старт

```bash
./scripts/build.sh both
cp configs/server.example.yaml configs/server.yaml
sudo ./bin/kvn-server --config configs/server.yaml
sudo ./bin/kvn-web --no-browser
```

`kvn-web`: http://127.0.0.1:2311

Сервер выдаёт каждому клиенту уникальный `tun_address` из пула; `client_id` генерируется клиентом автоматически, если его нет в JSON/QR.

## Установка

```bash
# сервер (root, systemd, TLS, TUN pool)
sudo ./scripts/install-server.sh --port 443

# клиент (root, systemd, TUN)
sudo ./scripts/install-client.sh --server https://HOST:443 --token TOKEN
```

Скрипты также умеют скачивать `kvn-server`/`kvn-client` из GitHub Releases, если локального бинаря нет.

## Конфигурация

Единый конфиг 2.0: `~/.config/kvn/config.yaml`. При миграции старый v1-файл сохраняется как `config.yaml.v1.bak`; старый `~/.config/kvn/v2/config.yaml` импортируется один раз.

Standalone:

```bash
cp configs/client.example.yaml configs/client.yaml
sudo ./bin/kvn-client --config configs/client.yaml
```

## Документация

- [v2 RU](docs/ru/v2.md) · [v2 EN](docs/en/v2.md)
- [v1 legacy: скачать и запустить старый релиз](docs/ru/v1-legacy.md) · [EN](docs/en/v1-legacy.md)
- [Release checklist 2.0.0](docs/ru/v2-release.md) · [EN](docs/en/v2-release.md)
- [DPI lab report](docs/ru/v2-dpi-report.md) · [EN](docs/en/v2-dpi-report.md)
- [CHANGELOG](CHANGELOG.md)

v1 не поддерживается в 2.0.0. Для v1 используйте старый release и его документацию из тега `v1.4.1`.
