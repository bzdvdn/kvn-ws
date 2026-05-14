<!-- @sk-task docs-and-release#T2.1: quickstart guide (AC-001) -->

# Quickstart

Get kvn-ws server and client running in 5 minutes.

## Prerequisites

- [Docker Engine](https://docs.docker.com/engine/install/) 24+ with docker compose plugin
- [OpenSSL](https://www.openssl.org/) (usually pre-installed on Linux)
- [Git](https://git-scm.com/)

## Step-by-step

### 1. Clone the repository

```bash
git clone https://github.com/bzdvdn/kvn-ws.git
cd kvn-ws
```

### 2. Copy example files

```bash
cp -r examples/* .
```

This copies a standalone docker-compose.yml, configs, and a helper script into the current directory.

### 3. Generate TLS certificate and start

```bash
bash examples/run.sh
```

The script:
1. Generates a self-signed TLS certificate (cert.pem, key.pem) via OpenSSL
2. Starts the server and client containers with `docker compose up -d`

### 4. Verify the connection

```bash
docker compose logs client | grep "handshake complete"
```

Expected output:
```
client-1  | ... "msg":"handshake complete" ... "ip":"10.10.0.2" ...
```

If you see this, kvn-ws is working. The client has a virtual IP from the server's pool.

### 5. (Optional) Follow client logs in real-time

```bash
docker compose logs -f client
```

## Architecture overview

```
┌──────────┐     WebSocket/TLS 1.3     ┌──────────┐
│  client  │ ────────────────────────▶ │  server  │
│ (TUN)    │                           │ (IP pool)│
└──────────┘                           └──────────┘
```

## Troubleshooting

| Problem | Check |
|---------|-------|
| `docker compose` not found | Install Docker Engine 24+ with compose plugin |
| `openssl` not found | Install openssl: `apt install openssl` (Debian) / `yum install openssl` (RHEL) |
| Client shows `connection refused` | Ensure port 443 is not in use: `ss -tlnp \| grep 443` |
| Client shows `auth failed` | Update the token in `server.yaml` and `client.yaml` to match |
| Client not connecting | Check server logs: `docker compose logs server` |

## Next steps

- [Configuration reference](config.md) — all config keys documented
- [Architecture](architecture.md) — system design and data flow
- [Examples](../examples/) — docker-compose, configs, and scripts
