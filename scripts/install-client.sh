#!/usr/bin/env bash
# Install kvn-ws client from GitHub release.
# Usage:
#   sudo ./install-client.sh
#   sudo ./install-client.sh --server wss://vpn.example.com:443/tunnel --token YOUR_TOKEN
#   bash <(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-client.sh)
set -euo pipefail

REPO="bzdvdn/kvn-ws"
VERSION="latest"
BINDIR="${BINDIR:-/usr/local/bin}"
CONFDIR="${CONFDIR:-/etc/kvn-ws}"
SYSTEMD_DIR="/etc/systemd/system"

SERVER=""
TOKEN=""
MODE="tun"
PROXY_LISTEN=""
INSTALL_SERVICE=false

# --- parse CLI flags ---
while [[ $# -gt 0 ]]; do
  case "$1" in
    --server)
      SERVER="$2"; shift 2 ;;
    --token)
      TOKEN="$2"; shift 2 ;;
    --mode)
      MODE="$2"; shift 2 ;;
    --proxy-listen)
      PROXY_LISTEN="$2"; shift 2 ;;
    --service)
      INSTALL_SERVICE=true; shift ;;
    --bindir)
      BINDIR="$2"; shift 2 ;;
    --confdir)
      CONFDIR="$2"; shift 2 ;;
    --version)
      VERSION="$2"; shift 2 ;;
    --help|-h)
      echo "Usage: $0 [options]"
      echo ""
      echo "Connection:"
      echo "  --server <url>          WebSocket server URL (required)"
      echo "  --token <token>         Auth token (required)"
      echo "  --mode <mode>           Client mode: tun or proxy (default: tun)"
      echo "  --proxy-listen <addr>   SOCKS5 listen address for proxy mode (default: 127.0.0.1:2310)"
      echo ""
      echo "Service (linux only):"
      echo "  --service               Install as systemd service"
      echo ""
      echo "Paths:"
      echo "  --bindir <path>         Binary install directory (default: /usr/local/bin)"
      echo "  --confdir <path>        Config directory (default: /etc/kvn-ws)"
      echo ""
      echo "Version:"
      echo "  --version <tag>         GitHub release tag (default: latest)"
      echo ""
      echo "Examples:"
      echo "  sudo $0 --server wss://vpn.example.com:443/tunnel --token mytoken"
      echo "  sudo $0 --server wss://vpn.example.com:443/tunnel --token mytoken --service"
      echo "  sudo $0 --mode proxy --proxy-listen 0.0.0.0:2310 --server ... --token ..."
      exit 0 ;;
    *)
      echo "Unknown option: $1" >&2
      echo "Use --help for usage." >&2
      exit 1 ;;
  esac
done

# --- detect OS/arch ---
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64"  ;;
  armv7l)  ARCH="armv7"  ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# --- resolve version ---
if [ "$VERSION" = "latest" ]; then
  VERSION="$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)"
  echo "Resolved latest version: $VERSION"
fi

ARCHIVE="kvn-${OS}-${ARCH}.tar.gz"
if echo "$OS" | grep -q mingw; then
  ARCHIVE="kvn-${OS}-${ARCH}.zip"
fi
URL="https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE"
CHECKSUM_URL="https://github.com/$REPO/releases/download/$VERSION/${ARCHIVE}.sha256"

# --- download ---
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading $URL ..."
curl -sL "$URL" -o "$TMPDIR/$ARCHIVE"

# --- verify checksum ---
echo "Verifying checksum ..."
CHECKSUM_LINE="$(curl -sL "$CHECKSUM_URL" 2>/dev/null | grep -E '^[a-f0-9]{64}[[:space:]]' || true)"
if [ -n "$CHECKSUM_LINE" ]; then
  CHECKSUM_VAL="${CHECKSUM_LINE%%[[:space:]]*}"
  printf "%s  %s\n" "$CHECKSUM_VAL" "$TMPDIR/$ARCHIVE" | sha256sum -c - || {
    echo "Checksum verification FAILED." >&2
    exit 1
  }
else
  echo "Checksum file not found or invalid format, skipping verification."
fi

# --- extract ---
echo "Extracting ..."
if echo "$ARCHIVE" | grep -q '\.zip$'; then
  unzip -o "$TMPDIR/$ARCHIVE" -d "$TMPDIR" >/dev/null 2>&1
else
  tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR" --strip-components=1
fi

# --- install binary ---
mkdir -p "$BINDIR"
install -m 0755 "$TMPDIR/client" "$BINDIR/kvn-client"
echo "Installed client to $BINDIR/kvn-client"

# --- write client config ---
if [ -n "$SERVER" ] && [ -n "$TOKEN" ]; then
  mkdir -p "$CONFDIR"
  CONFIG_FILE="$CONFDIR/client.yaml"
  if [ ! -f "$CONFIG_FILE" ]; then
    cat > "$CONFIG_FILE" <<EOF
server: ${SERVER}
transport: quic
obfuscation: true
auth:
  token: ${TOKEN}
mode: ${MODE}
EOF
    if [ -n "$PROXY_LISTEN" ]; then
      echo "proxy_listen: ${PROXY_LISTEN}" >> "$CONFIG_FILE"
    fi
    echo "Generated config at $CONFIG_FILE"
  else
    echo "Config already exists at $CONFIG_FILE, skipping."
  fi
fi

# --- install systemd service (linux only) ---
if [ "$INSTALL_SERVICE" = true ] && [ "$OS" = "linux" ]; then
  cat > "$SYSTEMD_DIR/kvn-client.service" <<SYSTEMD
[Unit]
Description=kvn-ws VPN client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BINDIR}/kvn-client --config ${CONFDIR}/client.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536
CapabilityBoundingSet=CAP_NET_ADMIN CAP_SYS_ADMIN
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN
DeviceAllow=/dev/net/tun

[Install]
WantedBy=multi-user.target
SYSTEMD
  systemctl daemon-reload
  echo "systemd unit written: $SYSTEMD_DIR/kvn-client.service"
  echo "  -> Start:  systemctl enable --now kvn-client"
  echo "  -> Logs:   journalctl -u kvn-client -f"
fi

echo ""
echo "=== Installation complete ==="
echo ""
echo "Binary: $BINDIR/kvn-client"
if [ -n "$SERVER" ]; then
  echo "Config: $CONFDIR/client.yaml"
  echo ""
  echo "Run:"
  echo "  sudo $BINDIR/kvn-client --config $CONFDIR/client.yaml"
fi
