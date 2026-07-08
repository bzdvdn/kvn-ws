# Deployment Guide

This guide covers production-ready deployment scenarios for kvn-ws.

## Contents

- [Server Installation](#server-installation)
  - [Option A: systemd (Linux VM, recommended)](#option-a-systemd-linux-vm-recommended)
  - [Option B: Docker](#option-b-docker)
- [Client Setup](#client-setup)
  - [Linux — TUN mode (VPN)](#linux--tun-mode-vpn)
  - [Linux — Proxy mode (SOCKS5/HTTP)](#linux--proxy-mode-socks5http)
  - [Windows — TUN mode (VPN)](#windows--tun-mode-vpn)
  - [Windows — Proxy mode](#windows--proxy-mode)
  - [macOS — TUN mode (VPN)](#macos--tun-mode-vpn)
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

Allow forwarding through the TUN interface in nftables:

```bash
sudo nft add table inet kvn-forward
sudo nft add chain inet kvn-forward forward '{ type filter hook forward priority 0; policy drop; }'
sudo nft add rule inet kvn-forward forward iifname "kvn" accept
sudo nft add rule inet kvn-forward forward oifname "kvn" accept
sudo nft list ruleset | sudo tee /etc/nftables/kvn.ruleset
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
    image: bzdvdn/kvn:latest
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

## Web UI Setup

KVN Web UI provides a browser-based management interface for the tunnel client (config editor, connect/disconnect, live logs).

### Linux — systemd service

#### Quick install (one-liner)

```bash
sudo bash -c "$(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-web.sh)" -- --start
```

Web UI: http://127.0.0.1:2311

#### What install-web.sh does

1. Copies `kvn-web` binary to `/usr/local/bin/kvn-web`
2. Installs systemd unit to `/etc/systemd/system/kvn-web.service`
3. Enables the service (auto-start on boot)
4. Starts the service (with `--start` flag)

#### Manage the service

```bash
sudo systemctl start kvn-web
sudo systemctl stop kvn-web
sudo systemctl status kvn-web
sudo journalctl -u kvn-web -f
```

#### Build from source

```bash
./scripts/build.sh web
sudo ./scripts/install-web.sh --start
```

### macOS — launchd daemon

#### Quick install (one-liner)

```bash
sudo bash -c "$(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-web.sh)" -- --start
```

Web UI: http://127.0.0.1:2311

#### Manage the daemon

```bash
sudo launchctl load -w /Library/LaunchDaemons/com.kvn-web.daemon.plist
sudo launchctl unload /Library/LaunchDaemons/com.kvn-web.daemon.plist
sudo launchctl list com.kvn-web.daemon
```

### Windows — Windows Service

#### Quick install (one-liner)

```powershell
iwr -useb https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-web.ps1 -OutFile install-web.ps1; .\install-web.ps1 -Start
```

Web UI: http://127.0.0.1:2311

#### Manage the service

```powershell
Start-Service KVNWeb
Stop-Service KVNWeb
Get-Service KVNWeb
```

### Configuration via Web UI

All client settings are available through the web interface. **kvn-web** supports **multi-server management** — you can add, switch, and delete multiple server configurations:
- **Global settings** (mode, proxy listen, log level, MTU, routing defaults) apply to all servers
- **Per-server settings** (server URL, token, TLS, routing overrides, obfuscation, encryption) are stored individually
- A **server selector dropdown** lets you switch between configurations; unsaved changes trigger a confirmation dialog

| Section | Fields |
|---------|--------|
| Servers | Add/Delete/Import server entries, Export/QR per-server config |
| Connection | Server, Token, Mode (proxy/TUN), Proxy Listen/Auth |
| TLS | Verify Mode, Server Name (SNI), CA File |
| Routing | Default Route, CIDR Include/Exclude, IP Include/Exclude |
| Advanced | MTU, Log Level, IPv6, Auto Reconnect, Multiplex, Max Message Size |
| Kill Switch & Reconnect | Kill Switch toggle, Min/Max Backoff |
| Encryption | App-Layer Encryption toggle, Key |

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
max_message_size: 10485760
tunnel_timeout: 30
routing:
  default_route: server
  exclude_ranges:
    - 10.0.0.0/8
    - 172.16.0.0/12
    - 192.168.0.0/16
  exclude_domains:
    - example.com

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

### Windows — TUN mode (VPN)

TUN mode on Windows uses [Wintun](https://www.wintun.net/) to create a virtual network adapter.
It routes all traffic through the VPN tunnel and supports exclude routes for split-tunnel setups.

**Prerequisites:**

- Run **as Administrator** (Wintun requires elevation)
- The `wintun.dll` binary must be in the same directory as `kvn-client.exe` or in `%PATH%`.
  Download from https://www.wintun.net/ (select your architecture) and place the DLL next to the binary.

#### Config example

`C:\ProgramData\kvn-ws\client.yaml`:

```yaml
mode: tun
server: wss://vpn.example.com:443/tunnel
auth:
  token: your-auth-token
mtu: 1400
log:
  level: info
```

#### Run manually

```powershell
# Command Prompt (as admin)
"C:\Program Files\kvn-ws\kvn-client.exe" --config "C:\ProgramData\kvn-ws\client.yaml"
```

#### Web UI

The `kvn-web` interface automatically detects Windows and shows the **TUN** mode option in the server settings dropdown.

#### DNS

TUN mode automatically configures DNS on the virtual adapter via `luid.SetDNS()`:
- DNS servers are set to `127.0.0.54` (local DNS proxy)
- Original DNS settings are not saved (restored automatically when the TUN adapter is closed)
- `CleanupStaleDNS` removes stale DNS entries on the adapter after an abnormal session termination

> **Note:** Windows TUN support requires the `wintun.dll` runtime; without it the adapter creation will fail. Always verify the DLL is present before starting the client.

---

### macOS — TUN mode (VPN)

TUN mode on macOS uses a utun interface to create a virtual network adapter.
It routes all traffic through the VPN tunnel and supports exclude routes for split-tunnel setups.

**Prerequisites:**

- Run **as root** using `sudo` (utun + route require root privileges)
- Alternatively, install `com.kvn.tun.plist` as a LaunchDaemon for automatic startup:
  ```bash
  sudo cp scripts/com.kvn.tun.plist /Library/LaunchDaemons/
  sudo launchctl load /Library/LaunchDaemons/com.kvn.tun.plist
  ```

#### Config example

`/etc/kvn/client.yaml`:

```yaml
mode: tun
server: wss://vpn.example.com:443/tunnel
auth:
  token: your-auth-token
mtu: 1400
log:
  level: info
```

#### Run manually

```bash
sudo ./kvn-client --config /etc/kvn/client.yaml
```

#### Web UI

The `kvn-web` interface automatically detects macOS and shows the **TUN** mode option in the server settings dropdown.

#### LaunchAgent for kvn-web (autostart at user logon)

```bash
cp scripts/com.kvn.web.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.kvn.web.plist
```

#### DNS

TUN mode automatically configures DNS via `networksetup -setdnsservers`:
- DNS servers are set to `127.0.0.54` (local DNS proxy)
- Network service is detected via `-listallhardwareports` (primary) with fallback to direct `utunX`
- Original DNS servers are saved and restored when the connection closes
- `CleanupStaleDNS` removes stale DNS entries on utun interfaces
- **macOS Ventura+**: fully supported; older versions may have limitations

> **Note:** macOS TUN mode requires root. Use `sudo` or the LaunchDaemon (`com.kvn.tun.plist`) for automatic startup at boot.

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
