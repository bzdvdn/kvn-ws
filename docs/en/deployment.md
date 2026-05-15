# Deployment Guide

This guide covers production-ready deployment scenarios for kvn-ws.

## Contents

- [Server Installation](#server-installation)
  - [Option A: systemd (Linux VM, recommended)](#option-a-systemd-linux-vm-recommended)
  - [Option B: Docker](#option-b-docker)
- [Client Setup](#client-setup)
  - [Linux — TUN mode (VPN)](#linux--tun-mode-vpn)
  - [Linux — Proxy mode (SOCKS5/HTTP)](#linux--proxy-mode-socks5http)
  - [Windows — Proxy mode](#windows--proxy-mode)
- [TLS certificates](#tls-certificates)
- [Firewall & NAT](#firewall--nat)
- [Monitoring](#monitoring)

---

## Server Installation

### Option A: systemd (Linux VM, recommended)

#### Quick install (one-liner)

```bash
sudo bash -c "$(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-server.sh)"
```

Or download and run manually:

```bash
curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-server.sh -o install-server.sh
chmod +x install-server.sh
sudo ./install-server.sh
```

The script will:

1. Detect your OS/arch and download the matching release binary
2. Verify the SHA256 checksum
3. Install the binary to `/usr/local/bin/kvn-server`
4. Create a default config at `/etc/kvn-ws/server.yaml` with a random auth token
5. Install a systemd unit at `/etc/systemd/system/kvn-server.service`

#### Manual setup

```bash
# 1. Download the release
curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/kvn-ws-server-linux-amd64.tar.gz \
  -o /tmp/kvn-server.tar.gz
tar -xzf /tmp/kvn-server.tar.gz -C /tmp
sudo install -m 0755 /tmp/server /usr/local/bin/kvn-server

# 2. Create config directory and TLS certs
sudo mkdir -p /etc/kvn-ws/certs
sudo openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout /etc/kvn-ws/certs/key.pem \
  -out /etc/kvn-ws/certs/cert.pem \
  -subj "/CN=$(hostname)"
```

Create `/etc/kvn-ws/server.yaml`:

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

Create `/etc/systemd/system/kvn-server.service`:

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

Start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now kvn-server
sudo journalctl -u kvn-server -f
```

Expected log output:

```
"msg":"starting server"  "listen":":443"
"msg":"listening"        "addr":":443"
```

#### VM requirements

| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| CPU | 1 core | 2 cores |
| RAM | 256 MB | 512 MB+ |
| Disk | 1 GB | 10 GB (for logs) |
| OS | Linux (kernel 4.18+) | Ubuntu 22.04+, Debian 12, Fedora 38+ |
| TUN | `/dev/net/tun` must exist | — |
| nftables | `nft` binary | `nftables` package |
| Capabilities | `NET_ADMIN`, `SYS_ADMIN` | — |

Enable IP forwarding:

```bash
echo 'net.ipv4.ip_forward=1' | sudo tee /etc/sysctl.d/99-kvn.conf
echo 'net.ipv6.conf.all.forwarding=1' | sudo tee -a /etc/sysctl.d/99-kvn.conf
sudo sysctl -p /etc/sysctl.d/99-kvn.conf
```

---

### Option B: Docker

#### With docker-compose (local development)

```bash
git clone https://github.com/bzdvdn/kvn-ws.git
cd kvn-ws

# Copy example files
cp -r examples/* .

# Generate self-signed TLS cert and start
bash examples/run.sh
```

#### Production Docker with systemd service

Create `/opt/kvn-ws/docker-compose.yml`:

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

Create `/etc/systemd/system/kvn-docker.service`:

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

#### Build custom Docker image

```bash
docker build -t kvn-ws:latest .
```

---

## Client Setup

### Linux — TUN mode (VPN)

Full VPN tunnel: all (or selected) traffic is routed through the remote server.

#### 1. Download the client

```bash
curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/kvn-ws-client-linux-amd64.tar.gz \
  -o /tmp/kvn-client.tar.gz
tar -xzf /tmp/kvn-client.tar.gz -C /tmp
sudo install -m 0755 /tmp/client /usr/local/bin/kvn-client
```

#### 2. Create config

`/etc/kvn-ws/client.yaml`:

```yaml
server: wss://vpn.example.com:443/tunnel
auth:
  token: your-auth-token
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

#### 3. Run as a systemd service

Create `/etc/systemd/system/kvn-client.service`:

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

Expected output:

```
"msg":"handshake complete"  "session":"..."  "ip":"10.10.0.2"
```

#### 4. Split-tunnel with kill-switch

```yaml
# /etc/kvn-ws/client.yaml
routing:
  default_route: server
  include_ranges:
    - 0.0.0.0/0        # route all traffic (full tunnel)
  exclude_ranges:
    - 192.168.0.0/16   # except local network
kill_switch:
  enabled: true         # block all traffic on disconnect
```

> **Note:** Kill-switch requires `nftables`. Install it: `sudo apt install nftables`.

---

### Linux — Proxy mode (SOCKS5/HTTP)

Runs as a local proxy — no TUN device needed, no root required.

#### 1. Download (same binary)

```bash
sudo install -m 0755 /tmp/client /usr/local/bin/kvn-client
```

#### 2. Create config

`~/.config/kvn-ws/client.yaml`:

```yaml
mode: proxy
proxy_listen: 127.0.0.1:2310
server: wss://vpn.example.com:443/tunnel
auth:
  token: your-auth-token
tls:
  verify_mode: verify
  server_name: vpn.example.com
log:
  level: info

# Optional: exclude certain traffic from the proxy
routing:
  default_route: server
  exclude_ranges:
    - 10.0.0.0/8
    - 192.168.0.0/16
  exclude_domains:
    - example.com
    - internal.corp
```

#### 3. Run

```bash
kvn-client --config ~/.config/kvn-ws/client.yaml
```

#### 4. Use the proxy

```bash
# HTTP — set environment variables
export HTTP_PROXY=socks5://127.0.0.1:2310
export HTTPS_PROXY=socks5://127.0.0.1:2310

# curl
curl --proxy socks5://127.0.0.1:2310 https://ifconfig.me

# Firefox: Settings → Network Settings → Manual proxy → SOCKS v5 → 127.0.0.1:2310
# Telegram: Settings → Advanced → Connection Type → Use Proxy → SOCKS5
```

#### 5. systemd user service (no root)

Create `~/.config/systemd/user/kvn-client.service`:

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

### Windows — Proxy mode

#### Quick install (one-liner)

Run in PowerShell (as admin):

```powershell
Set-ExecutionPolicy RemoteSigned -Scope Process -Force; `
iex "& { $(iwr -useb https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-client.ps1) }" ` 
  -Server "wss://vpn.example.com/tunnel" -Token "your-auth-token" -RegisterTask
```

Or download and run:

```powershell
# Download
iwr -useb https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-client.ps1 -OutFile install-client.ps1

# Install with autostart scheduled task
.\install-client.ps1 -Server "wss://vpn.example.com:443/tunnel" -Token "your-auth-token" -RegisterTask

# Install without autostart
.\install-client.ps1 -Server "wss://vpn.example.com:443/tunnel" -Token "your-auth-token"

# Uninstall
.\install-client.ps1 -Uninstall
```

#### 2. Create config

`C:\ProgramData\kvn-ws\client.yaml`:

```yaml
mode: proxy
proxy_listen: 127.0.0.1:2310
server: wss://vpn.example.com:443/tunnel
auth:
  token: your-auth-token
tls:
  verify_mode: verify
  server_name: vpn.example.com
log:
  level: info
```

#### 3. Run manually

```powershell
# Command Prompt (as admin)
"C:\Program Files\kvn-ws\kvn-client.exe" --config "C:\ProgramData\kvn-ws\client.yaml"
```

#### 4. Run as a Windows service (using NSSM)

[NSSM](https://nssm.cc/) (Non-Sucking Service Manager) wraps any binary as a Windows service.

```powershell
# Install NSSM (one-time)
winget install nssm

# Create the service
nssm install kvn-client "C:\Program Files\kvn-ws\kvn-client.exe"
nssm set kvn-client AppParameters "--config C:\ProgramData\kvn-ws\client.yaml"
nssm set kvn-client AppStdout "C:\ProgramData\kvn-ws\stdout.log"
nssm set kvn-client AppStderr "C:\ProgramData\kvn-ws\stderr.log"
nssm set kvn-client Start SERVICE_AUTO_START
nssm set kvn-client ObjectName "NT AUTHORITY\LocalService"

# Start
nssm start kvn-client
```

#### 5. Configure applications

**System-wide proxy (Windows 10/11):**

```
Settings → Network & Internet → Proxy → Use a proxy server
Address: 127.0.0.1  Port: 2310
```

**Per-application:**

```powershell
# curl
curl --proxy socks5://127.0.0.1:2310 https://ifconfig.me

# Firefox: Settings → Network → Manual proxy → SOCKS v5 → 127.0.0.1:2310
# Telegram: Settings → Advanced → Connection Type → SOCKS5 → 127.0.0.1:2310
# Browser extensions: FoxyProxy, SwitchyOmega (set SOCKS5 127.0.0.1:2310)
```

#### 6. Auto-start via Task Scheduler

```powershell
$action = New-ScheduledTaskAction -Execute "C:\Program Files\kvn-ws\kvn-client.exe" `
  -Argument "--config C:\ProgramData\kvn-ws\client.yaml"
$trigger = New-ScheduledTaskTrigger -AtLogon
$principal = New-ScheduledTaskPrincipal -UserId "$env:USERNAME" -RunLevel Limited
Register-ScheduledTask -TaskName "kvn-client" -Action $action -Trigger $trigger -Principal $principal
```

---

## TLS certificates

### Self-signed (testing)

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

# Symlink to kvn-ws dir
sudo ln -s /etc/letsencrypt/live/vpn.example.com/fullchain.pem /etc/kvn-ws/certs/cert.pem
sudo ln -s /etc/letsencrypt/live/vpn.example.com/privkey.pem /etc/kvn-ws/certs/key.pem
```

### mTLS (mutual TLS) — optional

If you want client certificate verification:

```yaml
# server.yaml
tls:
  client_ca_file: /etc/kvn-ws/certs/client-ca.pem
  client_auth: verify    # or "require"
```

Generate a client certificate:

```bash
openssl req -nodes -newkey rsa:2048 \
  -keyout client-key.pem -out client.csr \
  -subj "/CN=client1"
openssl x509 -req -days 365 \
  -in client.csr -CA client-ca.pem -CAkey client-ca-key.pem -CAcreateserial \
  -out client.pem
```

---

## Firewall & NAT

The server needs IP forwarding and a NAT rule to route client traffic.

```bash
# sysctl — enable forwarding
echo 'net.ipv4.ip_forward=1' >> /etc/sysctl.conf
sysctl -p

# nftables — already set up by kvn-server automatically on start
# It creates a MASQUERADE rule for the VPN pool subnet.
```

If using iptables instead:

```bash
iptables -t nat -A POSTROUTING -s 10.10.0.0/24 -o eth0 -j MASQUERADE
iptables -A FORWARD -i eth0 -o kvn -m state --state RELATED,ESTABLISHED -j ACCEPT
iptables -A FORWARD -i kvn -o eth0 -j ACCEPT
```

---

## Monitoring

### Health endpoints

```bash
# Liveness
curl -k https://vpn.example.com/livez

# Readiness
curl -k https://vpn.example.com/readyz

# Full health
curl -k https://vpn.example.com/health
```

### Prometheus metrics

```bash
curl -k https://vpn.example.com/metrics -H "Authorization: Bearer your-admin-token"
```

### Prometheus scrape config

```yaml
# prometheus.yml
scrape_configs:
  - job_name: kvn-ws
    scheme: https
    authorization:
      credentials: your-admin-token
    static_configs:
      - targets:
          - vpn.example.com:443
    metrics_path: /metrics
```

### Grafana

Import the metrics to build dashboards for:
- Active sessions (gauge)
- Throughput (rx/tx bytes/s)
- Error rates
- Session churn
