# kvn-ws 2.0

**kvn-ws 2.0** — v2-only VPN: TLS 1.3 + HTTP/2 TCP split, QUIC datagrams для UDP, TUN и локальный proxy. v1 удалён из текущего дерева; старые v1-сборки доступны в [GitHub Releases](https://github.com/bzdvdn/kvn-ws/releases).

## Компоненты

- `kvn-server` — v2-сервер (TCP + HTTP/2, optional QUIC UDP).
- `kvn-client` — standalone v2-клиент (TUN/proxy).
- `kvn-web` — v2-only web UI; конфиг в `~/.config/kvn/config.yaml`.
- `kvn-desktop` — нативный wrapper для `kvn-web`.
- Android-клиент `kvn-v2-client` — v2 VpnService на общем Go core.

## Быстрый старт

```bash
./scripts/build.sh both
cp configs/server.example.yaml configs/server.yaml
sudo ./bin/kvn-server --config configs/server.yaml
sudo ./bin/kvn-web --no-browser
```

`kvn-web`: http://127.0.0.1:2311

Сервер выдаёт каждому клиенту уникальный `tun_address` из пула; `client_id` генерируется клиентом автоматически, если его нет в JSON/QR.

## Установка из релиза 2.0.0

Скрипты рассчитаны на готовые артефакты из GitHub Releases. Для своего тега замените `v2.0.0` на актуальный.

### Сервер на Linux

```bash
curl -fsSL https://github.com/bzdvdn/kvn-ws/releases/download/v2.0.0/install-server.sh -o /tmp/install-server.sh
sudo bash /tmp/install-server.sh --port 443
```

Скрипт поставит `kvn-server`, создаст self-signed TLS и конфиг в `/etc/kvn-ws/server.yaml`, запустит systemd-сервис и выведет token.

### Клиент на Linux

```bash
curl -fsSL https://github.com/bzdvdn/kvn-ws/releases/download/v2.0.0/install-client.sh -o /tmp/install-client.sh
sudo bash /tmp/install-client.sh --server https://SERVER:443 --token TOKEN
```

Для proxy-режима добавьте `--proxy`; для своих direct-доменов — `--exclude-domains .ru,.we-on`.

### kvn-web + desktop

Linux и macOS:

```bash
curl -fsSL https://github.com/bzdvdn/kvn-ws/releases/download/v2.0.0/install-web.sh -o /tmp/install-web.sh
sudo bash /tmp/install-web.sh --start --desktop
```

Устанавливает `kvn-web`, `kvn-desktop`, systemd/launchd-сервис и ярлык приложения. Web UI: http://127.0.0.1:2311

Windows (PowerShell от администратора):

```powershell
iwr -useb https://github.com/bzdvdn/kvn-ws/releases/download/v2.0.0/install-web.ps1 -OutFile install-web.ps1
powershell -ExecutionPolicy Bypass -File .\install-web.ps1
```

По умолчанию ставится `kvn-desktop` (WebView) + `kvn-web` с автостартом. Старый браузерный режим: `-Browser`. Legacy v1 при установке удаляется автоматически.

Для self-signed сертификата включите `Allow insecure TLS` в настройках клиента; для production используйте доверенный сертификат.

## Сборка из исходников

```bash
# сервер (root, systemd, TLS, TUN pool)
sudo ./scripts/install-server.sh --port 443

# клиент (root, systemd, TUN)
sudo ./scripts/install-client.sh --server https://HOST:443 --token TOKEN

# web + desktop из локальной сборки
sudo ./scripts/install-web.sh --start --desktop
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
- [Roadmap](ROADMAP.md)
- [CHANGELOG](CHANGELOG.md)

v1 не поддерживается в 2.0.0. Для v1 используйте старый release и его документацию из тега `v1.4.1`.
