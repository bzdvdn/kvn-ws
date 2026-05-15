#!/usr/bin/env bash
# Install kvn-ws server from GitHub release as a systemd service.
# Usage:
#   sudo ./install-server.sh
#   sudo ./install-server.sh --listen :8443 --subnet 10.20.0.0/16 --gateway 10.20.0.1
#   sudo ./install-server.sh --ipv6 --subnet6 fd01::/112 --gateway6 fd01::1
#   bash <(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-server.sh) --listen :443
set -euo pipefail

REPO="bzdvdn/kvn-ws"
VERSION="latest"
BINDIR="${BINDIR:-/usr/local/bin}"
CONFDIR="${CONFDIR:-/etc/kvn-ws}"
SYSTEMD_DIR="/etc/systemd/system"

LISTEN=":443"
SUBNET="10.10.0.0/24"
GATEWAY="10.10.0.1"
SUBNET6=""
GATEWAY6=""
RANGE_START=""
RANGE_END=""
ENABLE_IPV6=false

# --- parse CLI flags ---
while [[ $# -gt 0 ]]; do
  case "$1" in
    --listen)
      LISTEN="$2"; shift 2 ;;
    --subnet)
      SUBNET="$2"; shift 2 ;;
    --gateway)
      GATEWAY="$2"; shift 2 ;;
    --subnet6)
      SUBNET6="$2"; shift 2 ;;
    --gateway6)
      GATEWAY6="$2"; shift 2 ;;
    --range-start)
      RANGE_START="$2"; shift 2 ;;
    --range-end)
      RANGE_END="$2"; shift 2 ;;
    --ipv6)
      ENABLE_IPV6=true; shift ;;
    --bindir)
      BINDIR="$2"; shift 2 ;;
    --confdir)
      CONFDIR="$2"; shift 2 ;;
    --version)
      VERSION="$2"; shift 2 ;;
    --help|-h)
      echo "Usage: $0 [options]"
      echo ""
      echo "Network:"
      echo "  --listen <addr>         Server listen address (default: :443)"
      echo "  --subnet <cidr>         IPv4 pool subnet (default: 10.10.0.0/24)"
      echo "  --gateway <ip>          IPv4 gateway (default: 10.10.0.1)"
      echo "  --range-start <ip>      First allocatable IPv4 address"
      echo "  --range-end <ip>        Last allocatable IPv4 address"
      echo "  --ipv6                  Enable IPv6 pool"
      echo "  --subnet6 <cidr>        IPv6 pool subnet (default: fd00::/112)"
      echo "  --gateway6 <ip>         IPv6 gateway (default: fd00::1)"
      echo ""
      echo "Paths:"
      echo "  --bindir <path>         Binary install directory (default: /usr/local/bin)"
      echo "  --confdir <path>        Config directory (default: /etc/kvn-ws)"
      echo ""
      echo "Version:"
      echo "  --version <tag>         GitHub release tag (default: latest)"
      echo ""
      echo "Examples:"
      echo "  sudo $0 --listen :8443 --subnet 172.16.0.0/16 --gateway 172.16.0.1"
      echo "  sudo $0 --ipv6 --subnet6 fd01::/112 --gateway6 fd01::1"
      exit 0 ;;
    *)
      echo "Unknown option: $1" >&2
      echo "Use --help for usage." >&2
      exit 1 ;;
  esac
done

if [ "$EUID" -ne 0 ]; then
  echo "This script must be run as root (sudo)." >&2
  exit 1
fi

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

if [ "$OS" != "linux" ]; then
  echo "Server installation is linux-only. For client on $OS see docs." >&2
  exit 1
fi

# --- resolve version ---
if [ "$VERSION" = "latest" ]; then
  VERSION="$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)"
  echo "Resolved latest version: $VERSION"
fi

ARCHIVE="kvn-ws-server-${OS}-${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE"
CHECKSUM_URL="https://github.com/$REPO/releases/download/$VERSION/${ARCHIVE}.sha256"

# --- download ---
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading $URL ..."
curl -sL "$URL" -o "$TMPDIR/$ARCHIVE"

# --- verify checksum ---
echo "Verifying checksum ..."
if CHECKSUM="$(curl -sL "$CHECKSUM_URL" 2>/dev/null | awk '{print $1}')" && [ -n "$CHECKSUM" ]; then
  echo "$CHECKSUM  $TMPDIR/$ARCHIVE" | sha256sum -c - || {
    echo "Checksum verification FAILED." >&2
    exit 1
  }
else
  echo "Checksum file not found, skipping verification."
fi

# --- extract ---
tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR"
mkdir -p "$BINDIR" "$CONFDIR"

install -m 0755 "$TMPDIR/server" "$BINDIR/kvn-server"

# --- build network config yaml ---
NETWORK_YAML="network:
  pool_ipv4:
    subnet: ${SUBNET}
    gateway: ${GATEWAY}"

if [ -n "$RANGE_START" ]; then
  NETWORK_YAML="${NETWORK_YAML}
    range_start: ${RANGE_START}"
fi
if [ -n "$RANGE_END" ]; then
  NETWORK_YAML="${NETWORK_YAML}
    range_end: ${RANGE_END}"
fi

if [ "$ENABLE_IPV6" = true ]; then
  if [ -z "$SUBNET6" ]; then
    SUBNET6="fd00::/112"
  fi
  if [ -z "$GATEWAY6" ]; then
    GATEWAY6="fd00::1"
  fi
  NETWORK_YAML="${NETWORK_YAML}
  pool_ipv6:
    subnet: ${SUBNET6}
    gateway: ${GATEWAY6}"
fi

# --- create default config if not exists ---
if [ ! -f "$CONFDIR/server.yaml" ]; then
  TOKEN="$(openssl rand -hex 24)"
  cat > "$CONFDIR/server.yaml" <<EOF
listen: ${LISTEN}
tls:
  cert: ${CONFDIR}/certs/cert.pem
  key: ${CONFDIR}/certs/key.pem
${NETWORK_YAML}
session:
  max_clients: 100
  idle_timeout_sec: 120
auth:
  mode: token
  tokens:
    - name: default
      secret: ${TOKEN}
logging:
  level: info
EOF
  echo "Generated default config at $CONFDIR/server.yaml"
  echo "  -> Auth token: ${TOKEN}"
  echo "  -> Listen: ${LISTEN}"
  echo "  -> Subnet: ${SUBNET}"
  echo "  -> Gateway: ${GATEWAY}"
  if [ "$ENABLE_IPV6" = true ]; then
    echo "  -> Subnet6: ${SUBNET6}"
    echo "  -> Gateway6: ${GATEWAY6}"
  fi
fi

# --- write systemd unit ---
cat > "$SYSTEMD_DIR/kvn-server.service" <<SYSTEMD
[Unit]
Description=kvn-ws VPN server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BINDIR}/kvn-server --config ${CONFDIR}/server.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536
AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN
DeviceAllow=/dev/net/tun
StateDirectory=kvn-ws
WorkingDirectory=${CONFDIR}

[Install]
WantedBy=multi-user.target
SYSTEMD

echo "systemd unit written: $SYSTEMD_DIR/kvn-server.service"

systemctl daemon-reload

echo ""
echo "=== Installation complete ==="
echo ""
echo "Next steps:"
echo "  1. Place TLS certificate at ${CONFDIR}/certs/cert.pem and ${CONFDIR}/certs/key.pem"
echo "     or generate self-signed:"
echo "       mkdir -p ${CONFDIR}/certs"
echo "       openssl req -x509 -nodes -days 365 -newkey rsa:2048 \\"
echo "         -keyout ${CONFDIR}/certs/key.pem \\"
echo "         -out ${CONFDIR}/certs/cert.pem \\"
echo "         -subj '/CN=$(hostname)'"
echo "  2. Review config: ${CONFDIR}/server.yaml"
echo "  3. Enable and start:"
echo "       systemctl enable --now kvn-server"
echo "  4. Check logs:"
echo "       journalctl -u kvn-server -f"
echo "  5. Client config (auth token: ${TOKEN}):"
echo "       server: wss://YOUR_SERVER_IP:443/tunnel"
echo "       auth.token: ${TOKEN}"
echo ""
