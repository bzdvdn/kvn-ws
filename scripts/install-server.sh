#!/usr/bin/env bash
# Install kvn-server 2.0 on Linux (systemd, TLS, TUN pool).
set -euo pipefail

REPO="${KVN_REPO:-bzdvdn/kvn-ws}"
VERSION="${KVN_VERSION:-latest}"
BIN_DIR="/usr/local/bin"
CONF_DIR="/etc/kvn-ws"
CERT_DIR="$CONF_DIR/certs"
STATE_DIR="/var/lib/kvn"
PORT=443
TOKEN=""
TUN_SUBNET="10.66.0.0/24"
RANGE_START="10.66.0.2"
RANGE_END="10.66.0.254"
GATEWAY="10.66.0.1"
LEASE_TTL=300
QUIC_LISTEN=""
NO_QUIC=false
FORCE=false

usage() {
  echo "Usage: sudo $0 [--port 443] [--token TOKEN] [--force] [--no-quic]"
  echo "  --tun-subnet 10.66.0.0/24 --range-start 10.66.0.2 --range-end 10.66.0.254 --gateway 10.66.0.1"
}
while [[ $# -gt 0 ]]; do
  case "$1" in
    --port) PORT="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    --tun-subnet) TUN_SUBNET="$2"; shift 2 ;;
    --range-start) RANGE_START="$2"; shift 2 ;;
    --range-end) RANGE_END="$2"; shift 2 ;;
    --gateway) GATEWAY="$2"; shift 2 ;;
    --lease-ttl) LEASE_TTL="$2"; shift 2 ;;
    --no-quic) NO_QUIC=true; shift ;;
    --force) FORCE=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown: $1" >&2; usage >&2; exit 1 ;;
  esac
done
if [[ "$NO_QUIC" != true ]]; then QUIC_LISTEN="0.0.0.0:$PORT"; fi
if [[ "$(id -u)" -ne 0 ]]; then echo "Run as root (sudo)." >&2; exit 1; fi
if ! command -v openssl >/dev/null; then echo "openssl is required." >&2; exit 1; fi
if ! command -v curl >/dev/null; then echo "curl is required." >&2; exit 1; fi

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in x86_64) ARCH=amd64 ;; aarch64|arm64) ARCH=arm64 ;; *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;; esac
if [[ "$OS" != linux ]]; then echo "This installer supports Linux; use the release archive on $OS." >&2; exit 1; fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_SRC=""
for candidate in "$SCRIPT_DIR/kvn-server" "$SCRIPT_DIR/bin/kvn-server" "./bin/kvn-server"; do
  if [[ -x "$candidate" ]]; then BIN_SRC="$candidate"; break; fi
done
if [[ -z "$BIN_SRC" ]]; then
  if [[ "$VERSION" == latest ]]; then
    VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)"
  fi
  TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
  ARCHIVE="kvn-$OS-$ARCH.tar.gz"
  curl -fsSL "https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE" -o "$TMP/$ARCHIVE"
  tar -xzf "$TMP/$ARCHIVE" -C "$TMP" --strip-components=1
  BIN_SRC="$TMP/kvn-server"
fi
install -m 0755 "$BIN_SRC" "$BIN_DIR/kvn-server"
mkdir -p "$CONF_DIR" "$CERT_DIR" "$STATE_DIR"
chmod 700 "$CONF_DIR" "$CERT_DIR"

if [[ ! -f "$CERT_DIR/server.pem" || ! -f "$CERT_DIR/server-key.pem" ]]; then
  HOST="$(hostname -f 2>/dev/null || hostname)"
  IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
  SAN="DNS:$HOST"
  [[ -n "$IP" ]] && SAN="$SAN,IP:$IP"
  openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
    -keyout "$CERT_DIR/server-key.pem" -out "$CERT_DIR/server.pem" \
    -subj "/CN=$HOST" -addext "subjectAltName=$SAN" >/dev/null 2>&1
  chmod 600 "$CERT_DIR/server-key.pem"
fi

if [[ -z "$TOKEN" ]]; then TOKEN="$(openssl rand -hex 24)"; fi
CONF="$CONF_DIR/server.yaml"
if [[ -f "$CONF" && "$FORCE" != true ]]; then
  echo "Config exists: $CONF (use --force to overwrite)" >&2
else
  cat > "$CONF" <<YAML
listen: 0.0.0.0:$PORT
quic_listen: $QUIC_LISTEN
tls:
  cert: $CERT_DIR/server.pem
  key: $CERT_DIR/server-key.pem
auth:
  tokens:
    - name: default
      secret: $TOKEN
fallback:
  title: Welcome
max_streams: 512
tun:
  subnet: $TUN_SUBNET
  range_start: $RANGE_START
  range_end: $RANGE_END
  gateway: $GATEWAY
  lease_ttl_sec: $LEASE_TTL
  lease_file: $STATE_DIR/tun-leases.json
YAML
  chmod 600 "$CONF"
fi

cat > /etc/systemd/system/kvn-server.service <<UNIT
[Unit]
Description=KVN v2 server
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
ExecStart=$BIN_DIR/kvn-server --config $CONF
Restart=on-failure
RestartSec=2
LimitNOFILE=65536
Environment=HOME=/root
[Install]
WantedBy=multi-user.target
UNIT

# Stop the old test service if present.
systemctl disable --now kvn-v2-server.service 2>/dev/null || true
systemctl daemon-reload
systemctl enable --now kvn-server.service
sleep 1
systemctl --no-pager --full status kvn-server.service | head -12 || true
echo
echo "Installed."
echo "  config: $CONF"
echo "  token:  $TOKEN"
echo "  tun:    $TUN_SUBNET ($RANGE_START-$RANGE_END, gw $GATEWAY)"
echo "  service: systemctl status kvn-server"
