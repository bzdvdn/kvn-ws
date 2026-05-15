# Руководство по развёртыванию

В этом руководстве описаны production-сценарии развёртывания kvn-ws.

## Содержание

- [Установка сервера](#установка-сервера)
  - [Вариант A: systemd (Linux VM, рекомендуется)](#вариант-a-systemd-linux-vm-рекомендуется)
  - [Вариант B: Docker](#вариант-b-docker)
- [Настройка клиента](#настройка-клиента)
  - [Linux — Режим TUN (VPN)](#linux--режим-tun-vpn)
  - [Linux — Режим прокси (SOCKS5/HTTP)](#linux--режим-прокси-socks5http)
  - [Windows — Режим прокси](#windows--режим-прокси)
- [TLS-сертификаты](#tls-сертификаты)
- [Файрвол и NAT](#файрвол-и-nat)
- [Мониторинг](#мониторинг)

---

## Установка сервера

### Вариант A: systemd (Linux VM, рекомендуется)

#### Быстрая установка (одна команда)

```bash
sudo bash -c "$(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-server.sh)"
```

Или скачайте и запустите вручную:

```bash
curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-server.sh -o install-server.sh
chmod +x install-server.sh
sudo ./install-server.sh
```

Скрипт автоматически:

1. Определит вашу ОС/архитектуру и скачает соответствующий релиз
2. Проверит SHA256-контрольную сумму
3. Установит бинарник в `/usr/local/bin/kvn-server`
4. Создаст конфиг по умолчанию в `/etc/kvn-ws/server.yaml` со случайным токеном
5. Установит systemd-юнит в `/etc/systemd/system/kvn-server.service`

#### Установка вручную

```bash
# 1. Скачайте релиз
curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/kvn-ws-server-linux-amd64.tar.gz \
  -o /tmp/kvn-server.tar.gz
tar -xzf /tmp/kvn-server.tar.gz -C /tmp
sudo install -m 0755 /tmp/server /usr/local/bin/kvn-server

# 2. Создайте директорию конфига и TLS-сертификаты
sudo mkdir -p /etc/kvn-ws/certs
sudo openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout /etc/kvn-ws/certs/key.pem \
  -out /etc/kvn-ws/certs/cert.pem \
  -subj "/CN=$(hostname)"
```

Создайте `/etc/kvn-ws/server.yaml`:

```yaml
listen: :443
tls:
  cert: /etc/kvn-ws/certs/cert.pem
  key: /etc/kvn-ws/certs/key.pem
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
      secret: $(openssl rand -hex 24)
logging:
  level: info
```

Создайте `/etc/systemd/system/kvn-server.service`:

```ini
[Unit]
Description=kvn-ws VPN server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/kvn-server --config /etc/kvn-ws/server.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN
DeviceAllow=/dev/net/tun
StateDirectory=kvn-ws
WorkingDirectory=/etc/kvn-ws

[Install]
WantedBy=multi-user.target
```

Запустите сервис:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now kvn-server
sudo journalctl -u kvn-server -f
```

Ожидаемый вывод:

```
"msg":"starting server"  "listen":":443"
"msg":"listening"        "addr":":443"
```

#### Требования к VM

| Требование | Минимум | Рекомендуется |
|------------|---------|---------------|
| CPU | 1 ядро | 2 ядра |
| RAM | 256 MB | 512 MB+ |
| Диск | 1 GB | 10 GB (под логи) |
| ОС | Linux (kernel 4.18+) | Ubuntu 22.04+, Debian 12, Fedora 38+ |
| TUN | `/dev/net/tun` должен существовать | — |
| nftables | `nft` | пакет `nftables` |
| Capabilities | `NET_ADMIN`, `SYS_ADMIN` | — |

Включите IP-forwarding:

```bash
echo 'net.ipv4.ip_forward=1' | sudo tee /etc/sysctl.d/99-kvn.conf
echo 'net.ipv6.conf.all.forwarding=1' | sudo tee -a /etc/sysctl.d/99-kvn.conf
sudo sysctl -p /etc/sysctl.d/99-kvn.conf
```

---

### Вариант B: Docker

#### С docker-compose (локальная разработка)

```bash
git clone https://github.com/bzdvdn/kvn-ws.git
cd kvn-ws

# Скопируйте примеры
cp -r examples/* .

# Сгенерируйте самоподписанный TLS-сертификат и запустите
bash examples/run.sh
```

#### Production Docker с systemd-сервисом

Создайте `/opt/kvn-ws/docker-compose.yml`:

```yaml
services:
  server:
    image: ghcr.io/bzdvdn/kvn-ws:latest
    ports:
      - "443:443"
    cap_add:
      - NET_ADMIN
      - SYS_ADMIN
    volumes:
      - /dev/net/tun:/dev/net/tun
      - ./server.yaml:/etc/kvn-ws/server.yaml
      - ./certs:/etc/kvn-ws/certs:ro
    command: ["--config", "/etc/kvn-ws/server.yaml"]
    restart: unless-stopped
    logging:
      driver: journald
```

Создайте `/etc/systemd/system/kvn-docker.service`:

```ini
[Unit]
Description=kvn-ws Docker server
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/kvn-ws
ExecStart=/usr/bin/docker compose up -d
ExecStop=/usr/bin/docker compose down
ExecReload=/usr/bin/docker compose restart

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now kvn-docker
```

---

## Настройка клиента

### Linux — Режим TUN (VPN)

Полный VPN-туннель: весь (или выборочный) трафик маршрутизируется через удалённый сервер.

#### 1. Скачайте клиент

```bash
curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/kvn-ws-client-linux-amd64.tar.gz \
  -o /tmp/kvn-client.tar.gz
tar -xzf /tmp/kvn-client.tar.gz -C /tmp
sudo install -m 0755 /tmp/client /usr/local/bin/kvn-client
```

#### 2. Создайте конфиг

`/etc/kvn-ws/client.yaml`:

```yaml
server: wss://vpn.example.com:443/tunnel
auth:
  token: ваш-токен-аутентификации
tls:
  verify_mode: verify
  server_name: vpn.example.com
mtu: 1400
auto_reconnect: true
log:
  level: info
crypto:
  enabled: false
routing:
  default_route: server
  exclude_ranges:
    - 10.0.0.0/8
    - 172.16.0.0/12
    - 192.168.0.0/16
  exclude_domains:
    - example.com
```

#### 3. Запустите как systemd-сервис

Создайте `/etc/systemd/system/kvn-client.service`:

```ini
[Unit]
Description=kvn-ws VPN client (TUN mode)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/kvn-client --config /etc/kvn-ws/client.yaml
Restart=always
RestartSec=5
AmbientCapabilities=CAP_NET_ADMIN
DeviceAllow=/dev/net/tun

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now kvn-client
sudo journalctl -u kvn-client -f
```

Ожидаемый вывод:

```
"msg":"handshake complete"  "session":"..."  "ip":"10.10.0.2"
```

#### 4. Split-tunnel с kill-switch

```yaml
# /etc/kvn-ws/client.yaml
routing:
  default_route: server
  include_ranges:
    - 0.0.0.0/0        # весь трафик через VPN
  exclude_ranges:
    - 192.168.0.0/16   # кроме локальной сети
kill_switch:
  enabled: true         # блокировать весь трафик при разрыве
```

> **Важно:** Kill-switch требует `nftables`: `sudo apt install nftables`.

---

### Linux — Режим прокси (SOCKS5/HTTP)

Локальный прокси — не требует TUN-устройства, не требует root.

#### 1. Скачайте (тот же бинарник)

```bash
sudo install -m 0755 /tmp/client /usr/local/bin/kvn-client
```

#### 2. Создайте конфиг

`~/.config/kvn-ws/client.yaml`:

```yaml
mode: proxy
proxy_listen: 127.0.0.1:2310
server: wss://vpn.example.com:443/tunnel
auth:
  token: ваш-токен
tls:
  verify_mode: verify
  server_name: vpn.example.com
log:
  level: info

# Опционально: исключить определённый трафик из прокси
routing:
  default_route: server
  exclude_ranges:
    - 10.0.0.0/8
    - 192.168.0.0/16
  exclude_domains:
    - example.com
    - internal.corp
```

#### 3. Запустите

```bash
kvn-client --config ~/.config/kvn-ws/client.yaml
```

#### 4. Используйте прокси

```bash
# HTTP — установите переменные окружения
export HTTP_PROXY=socks5://127.0.0.1:2310
export HTTPS_PROXY=socks5://127.0.0.1:2310

# curl
curl --proxy socks5://127.0.0.1:2310 https://ifconfig.me

# Firefox: Настройки → Сеть → Настройки прокси → Вручную → SOCKS v5 → 127.0.0.1:2310
# Telegram: Настройки → Дополнительно → Тип подключения → Использовать прокси → SOCKS5
```

#### 5. systemd user-сервис (без root)

Создайте `~/.config/systemd/user/kvn-client.service`:

```ini
[Unit]
Description=kvn-ws VPN client (proxy mode)

[Service]
Type=simple
ExecStart=/usr/local/bin/kvn-client --config %h/.config/kvn-ws/client.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now kvn-client
systemctl --user status kvn-client
```

---

### Windows — Режим прокси

#### Быстрая установка (одна команда)

Запустите в PowerShell (от администратора):

```powershell
Set-ExecutionPolicy RemoteSigned -Scope Process -Force; `
iex "& { $(iwr -useb https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-client.ps1) }" `
  -Server "wss://vpn.example.com/tunnel" -Token "ваш-токен" -RegisterTask
```

Или скачайте и запустите:

```powershell
# Скачать
iwr -useb https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-client.ps1 -OutFile install-client.ps1

# Установить с автозапуском
.\install-client.ps1 -Server "wss://vpn.example.com:443/tunnel" -Token "ваш-токен" -RegisterTask

# Установить без автозапуска
.\install-client.ps1 -Server "wss://vpn.example.com:443/tunnel" -Token "ваш-токен"

# Удалить
.\install-client.ps1 -Uninstall
```

#### 2. Создайте конфиг

`C:\ProgramData\kvn-ws\client.yaml`:

```yaml
mode: proxy
proxy_listen: 127.0.0.1:2310
server: wss://vpn.example.com:443/tunnel
auth:
  token: ваш-токен
tls:
  verify_mode: verify
  server_name: vpn.example.com
log:
  level: info
```

#### 3. Запуск вручную

```powershell
# Командная строка (от администратора)
"C:\Program Files\kvn-ws\kvn-client.exe" --config "C:\ProgramData\kvn-ws\client.yaml"
```

#### 4. Запуск как Windows-сервис (через NSSM)

[NSSM](https://nssm.cc/) (Non-Sucking Service Manager) позволяет запускать любой бинарник как Windows-сервис.

```powershell
# Установите NSSM
winget install nssm

# Создайте сервис
nssm install kvn-client "C:\Program Files\kvn-ws\kvn-client.exe"
nssm set kvn-client AppParameters "--config C:\ProgramData\kvn-ws\client.yaml"
nssm set kvn-client AppStdout "C:\ProgramData\kvn-ws\stdout.log"
nssm set kvn-client AppStderr "C:\ProgramData\kvn-ws\stderr.log"
nssm set kvn-client Start SERVICE_AUTO_START
nssm set kvn-client ObjectName "NT AUTHORITY\LocalService"

# Запустите
nssm start kvn-client
```

#### 5. Настройка приложений

**Системный прокси (Windows 10/11):**

```
Параметры → Сеть и Интернет → Прокси → Использовать прокси-сервер
Адрес: 127.0.0.1  Порт: 2310
```

**Для отдельных приложений:**

```powershell
# curl
curl --proxy socks5://127.0.0.1:2310 https://ifconfig.me

# Firefox: Настройки → Сеть → Настройки прокси → Вручную → SOCKS v5 → 127.0.0.1:2310
# Telegram: Настройки → Дополнительно → Тип подключения → SOCKS5 → 127.0.0.1:2310
# Расширения браузера: FoxyProxy, SwitchyOmega (SOCKS5 127.0.0.1:2310)
```

#### 6. Автозапуск через планировщик задач

```powershell
$action = New-ScheduledTaskAction -Execute "C:\Program Files\kvn-ws\kvn-client.exe" `
  -Argument "--config C:\ProgramData\kvn-ws\client.yaml"
$trigger = New-ScheduledTaskTrigger -AtLogon
$principal = New-ScheduledTaskPrincipal -UserId "$env:USERNAME" -RunLevel Limited
Register-ScheduledTask -TaskName "kvn-client" -Action $action -Trigger $trigger -Principal $principal
```

---

## TLS-сертификаты

### Самоподписанные (для тестирования)

```bash
mkdir -p /etc/kvn-ws/certs
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout /etc/kvn-ws/certs/key.pem \
  -out /etc/kvn-ws/certs/cert.pem \
  -subj "/CN=$(hostname)"
```

### Let's Encrypt (production)

```bash
sudo apt install certbot
sudo certbot certonly --standalone -d vpn.example.com

# Симлинки в директорию kvn-ws
sudo ln -s /etc/letsencrypt/live/vpn.example.com/fullchain.pem /etc/kvn-ws/certs/cert.pem
sudo ln -s /etc/letsencrypt/live/vpn.example.com/privkey.pem /etc/kvn-ws/certs/key.pem
```

### mTLS (взаимная TLS-аутентификация) — опционально

Для проверки сертификата клиента:

```yaml
# server.yaml
tls:
  client_ca_file: /etc/kvn-ws/certs/client-ca.pem
  client_auth: verify    # или "require"
```

Создание клиентского сертификата:

```bash
openssl req -nodes -newkey rsa:2048 \
  -keyout client-key.pem -out client.csr \
  -subj "/CN=client1"
openssl x509 -req -days 365 \
  -in client.csr -CA client-ca.pem -CAkey client-ca-key.pem -CAcreateserial \
  -out client.pem
```

---

## Файрвол и NAT

Серверу требуется IP-форвардинг и NAT-правило для маршрутизации трафика клиентов.

```bash
# sysctl — включите форвардинг
echo 'net.ipv4.ip_forward=1' >> /etc/sysctl.conf
sysctl -p

# nftables — уже настраивается kvn-server автоматически при запуске
# Он создаёт правило MASQUERADE для подсети VPN-пула.
```

Если используете iptables вместо nftables:

```bash
iptables -t nat -A POSTROUTING -s 10.10.0.0/24 -o eth0 -j MASQUERADE
iptables -A FORWARD -i eth0 -o kvn -m state --state RELATED,ESTABLISHED -j ACCEPT
iptables -A FORWARD -i kvn -o eth0 -j ACCEPT
```

---

## Мониторинг

### Health endpoints

```bash
# Liveness
curl -k https://vpn.example.com/livez

# Readiness
curl -k https://vpn.example.com/readyz

# Полный health
curl -k https://vpn.example.com/health
```

### Prometheus-метрики

```bash
curl -k https://vpn.example.com/metrics -H "Authorization: Bearer ваш-admin-токен"
```

### Prometheus scrape config

```yaml
# prometheus.yml
scrape_configs:
  - job_name: kvn-ws
    scheme: https
    authorization:
      credentials: ваш-admin-токен
    static_configs:
      - targets:
          - vpn.example.com:443
    metrics_path: /metrics
```

### Grafana

Импортируйте метрики для дашбордов:
- Активные сессии (gauge)
- Пропускная способность (rx/tx bytes/s)
- Частота ошибок
- Смена сессий (session churn)
