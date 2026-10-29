#!/usr/bin/env bash
# Install kvn-client 2.0 on Linux (systemd, v2 config).
set -euo pipefail
REPO="${KVN_REPO:-bzdvdn/kvn-ws}"
VERSION="${KVN_VERSION:-latest}"
BIN_DIR="/usr/local/bin"
CONF_DIR="/etc/kvn-ws"
CONF="$CONF_DIR/client.yaml"
ENDPOINT=""
TOKEN=""
EXCLUDE_DOMAINS=".ru"
PROXY=false

usage() {
  echo "Usage: sudo $0 --server https://host:443 --token TOKEN [--proxy] [--exclude-domains .ru,.su]"
}
while [[ $# -gt 0 ]]; do
  case "$1" in
    --server) ENDPOINT="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    --exclude-domains) EXCLUDE_DOMAINS="$2"; shift 2 ;;
    --proxy) PROXY=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown: $1" >&2; usage >&2; exit 1 ;;
  esac
done
if [[ -z "$ENDPOINT" || -z "$TOKEN" ]]; then usage >&2; exit 1; fi
if [[ "$(id -u)" -ne 0 ]]; then echo "Run as root (sudo)." >&2; exit 1; fi
if ! command -v curl >/dev/null; then echo "curl is required." >&2; exit 1; fi
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in x86_64) ARCH=amd64 ;; aarch64|arm64) ARCH=arm64 ;; *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;; esac
if [[ "$OS" != linux ]]; then echo "This installer supports Linux." >&2; exit 1; fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_SRC=""
for candidate in "$SCRIPT_DIR/kvn-client" "$SCRIPT_DIR/bin/kvn-client" "./bin/kvn-client"; do
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
  BIN_SRC="$TMP/kvn-client"
fi
install -m 0755 "$BIN_SRC" "$BIN_DIR/kvn-client"
mkdir -p "$CONF_DIR"

MODE="tun"; [[ "$PROXY" == true ]] && MODE="proxy"
# Escape YAML quotes minimally.
cat > "$CONF" <<YAML
active_server: default
mode: $MODE
mtu: 1200
quic_enabled: true
proxy_listen: 127.0.0.1:2310
tls:
  verify_mode: insecure
servers:
  - name: default
    endpoint: $ENDPOINT
    token: $TOKEN
routing:
  default_route: server
  exclude_domains: [$(printf '%s' "$EXCLUDE_DOMAINS" | awk -F, '{for(i=1;i<=NF;i++){printf "\"%s\"",$i; if(i<NF)printf ","}}')]
dns_proxy:
  listen: 127.0.0.153:53
  upstreams:
    - 1.1.1.1:53
    - 8.8.8.8:53
YAML
chmod 600 "$CONF"

cat > /etc/systemd/system/kvn-client.service <<UNIT
[Unit]
Description=KVN v2 client
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
ExecStart=$BIN_DIR/kvn-client --config $CONF
Restart=on-failure
RestartSec=2
LimitNOFILE=65536
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
Environment=HOME=/root
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now kvn-client.service
systemctl --no-pager --full status kvn-client.service | head -10 || true
echo "Installed: $CONF"
echo "For TUN, server cert is self-signed; verify_mode is insecure by default for test deployments."
