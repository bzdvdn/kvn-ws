#!/usr/bin/env bash
# Install kvn-ws relay (terminator) from GitHub release as a systemd service.
# Usage:
#   sudo ./install-relay.sh
#   sudo ./install-relay.sh --listen :8443 --server wss://vpn.example.com/tunnel
#   sudo ./install-relay.sh --subnet 172.16.0.0/24 --gateway 172.16.0.1
#   bash <(curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-relay.sh) --listen :8443
set -euo pipefail

REPO="bzdvdn/kvn-ws"
VERSION="latest"
BINDIR="${BINDIR:-/usr/local/bin}"
CONFDIR="${CONFDIR:-/etc/kvn-ws}"
SYSTEMD_DIR="/etc/systemd/system"

LISTEN=":8443"
SERVER=""
TRANSPORT="quic"
SUBNET="172.16.0.0/24"
GATEWAY="172.16.0.1"
SUBNET6=""
GATEWAY6=""
ENABLE_IPV6=false
UPSTREAM_TOKEN=""
CLIENT_TOKEN=""
DIRECT_RANGES="10.0.0.0/8,192.168.0.0/16"
DIRECT_DOMAINS=".local,.internal.example"
DNS_UPSTREAM="1.1.1.1:53"

# --- parse CLI flags ---
while [[ $# -gt 0 ]]; do
  case "$1" in
    --listen)
      LISTEN="$2"; shift 2 ;;
    --server)
      SERVER="$2"; shift 2 ;;
    --transport)
      TRANSPORT="$2"; shift 2 ;;
    --subnet)
      SUBNET="$2"; shift 2 ;;
    --gateway)
      GATEWAY="$2"; shift 2 ;;
    --subnet6)
      SUBNET6="$2"; shift 2 ;;
    --gateway6)
      GATEWAY6="$2"; shift 2 ;;
    --ipv6)
      ENABLE_IPV6=true; shift ;;
    --upstream-token)
      UPSTREAM_TOKEN="$2"; shift 2 ;;
    --client-token)
      CLIENT_TOKEN="$2"; shift 2 ;;
    --direct-ranges)
      DIRECT_RANGES="$2"; shift 2 ;;
    --direct-domains)
      DIRECT_DOMAINS="$2"; shift 2 ;;
    --dns)
      DNS_UPSTREAM="$2"; shift 2 ;;
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
      echo "  --listen <addr>         Relay listen address (default: :8443)"
      echo "  --server <url>          Upstream VPN server URL (required)"
      echo "  --transport <tcp|quic>  Upstream transport (default: quic)"
      echo "  --subnet <cidr>         Client pool subnet (default: 172.16.0.0/24)"
      echo "  --gateway <ip>          Client pool gateway (default: 172.16.0.1)"
      echo "  --ipv6                  Enable IPv6 pool"
      echo "  --subnet6 <cidr>        IPv6 pool subnet"
      echo "  --gateway6 <ip>         IPv6 pool gateway"
      echo ""
      echo "Routing:"
      echo "  --direct-ranges <list>  Comma-separated CIDRs for direct route (default: 10.0.0.0/8,192.168.0.0/16)"
      echo "  --direct-domains <list> Comma-separated domains for direct route (default: .local,.internal.example)"
      echo "  --dns <addr>            Upstream DNS resolver (default: 1.1.1.1:53)"
      echo ""
      echo "Auth:"
      echo "  --upstream-token <str>  Token for upstream VPN server (or KVN_RELAY_AUTH_TOKEN env)"
      echo "  --client-token <str>    Token for clients connecting to relay"
      echo ""
      echo "Paths:"
      echo "  --bindir <path>         Binary install directory (default: /usr/local/bin)"
      echo "  --confdir <path>        Config directory (default: /etc/kvn-ws)"
      echo ""
      echo "Version:"
      echo "  --version <tag>         GitHub release tag (default: latest)"
      echo ""
      echo "Examples:"
      echo "  sudo $0 --server wss://vpn.example.com/tunnel"
      echo "  sudo $0 --server quic://vpn.example.com --transport quic --subnet 10.11.0.0/24"
      echo "  sudo $0 --server wss://vpn.example.com/tunnel --direct-ranges '10.0.0.0/8' --direct-domains '.corp'"
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

if [ -z "$SERVER" ]; then
  echo "Error: --server is required (upstream VPN server URL)." >&2
  echo "  e.g. --server wss://vpn.example.com/tunnel" >&2
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
  echo "Relay installation is linux-only." >&2
  exit 1
fi

# --- resolve version ---
if [ "$VERSION" = "latest" ]; then
  VERSION="$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)"
  echo "Resolved latest version: $VERSION"
fi

ARCHIVE="kvn-${OS}-${ARCH}.tar.gz"
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
mkdir -p "$BINDIR" "$CONFDIR"
tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR" --strip-components=1

install -m 0755 "$TMPDIR/relay" "$BINDIR/kvn-relay"

# --- generate tokens ---
if [ -z "$UPSTREAM_TOKEN" ]; then
  UPSTREAM_TOKEN="${KVN_RELAY_AUTH_TOKEN:-}"
fi
if [ -z "$UPSTREAM_TOKEN" ]; then
  UPSTREAM_TOKEN="$(openssl rand -hex 24)"
fi
if [ -z "$CLIENT_TOKEN" ]; then
  CLIENT_TOKEN="$(openssl rand -hex 24)"
fi

# --- build direct_ranges yaml list ---
DIRECT_RANGES_YAML=""
IFS=',' read -ra RANGES <<< "$DIRECT_RANGES"
for r in "${RANGES[@]}"; do
  r="$(echo "$r" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
  if [ -n "$r" ]; then
    DIRECT_RANGES_YAML="${DIRECT_RANGES_YAML}
      - ${r}"
  fi
done

# --- build direct_domains yaml list ---
DIRECT_DOMAINS_YAML=""
IFS=',' read -ra DOMAINS <<< "$DIRECT_DOMAINS"
for d in "${DOMAINS[@]}"; do
  d="$(echo "$d" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
  if [ -n "$d" ]; then
    DIRECT_DOMAINS_YAML="${DIRECT_DOMAINS_YAML}
      - ${d}"
  fi
done

# --- create default config if not exists ---
if [ ! -f "$CONFDIR/relay.yaml" ]; then
  cat > "$CONFDIR/relay.yaml" <<EOF
mode: relay
server: ${SERVER}
transport: ${TRANSPORT}
upstream_token: ${UPSTREAM_TOKEN}

relay:
  mode: terminator
  listen: ${LISTEN}
  ws_paths:
    - /tunnel
  max_connections: 200
  quic:
    keep_alive: 7
    idle_timeout: 60
EOF

  # routing section
  cat >> "$CONFDIR/relay.yaml" <<EOF
  routing:
    direct_ranges:
EOF
  echo "${DIRECT_RANGES_YAML}" >> "$CONFDIR/relay.yaml"

  if [ -n "$DIRECT_DOMAINS" ]; then
    cat >> "$CONFDIR/relay.yaml" <<EOF
    direct_domains:
EOF
    echo "${DIRECT_DOMAINS_YAML}" >> "$CONFDIR/relay.yaml"
  fi

  cat >> "$CONFDIR/relay.yaml" <<EOF
    dns:
      upstream: "${DNS_UPSTREAM}"
      cache_ttl: 60
      transparent: false
EOF

  # network section
  cat >> "$CONFDIR/relay.yaml" <<EOF
  network:
    pool_ipv4:
      subnet: ${SUBNET}
      gateway: ${GATEWAY}
EOF

  if [ "$ENABLE_IPV6" = true ]; then
    if [ -z "$SUBNET6" ]; then
      SUBNET6="fd00::/112"
    fi
    if [ -z "$GATEWAY6" ]; then
      GATEWAY6="fd00::1"
    fi
    cat >> "$CONFDIR/relay.yaml" <<EOF
    pool_ipv6:
      subnet: ${SUBNET6}
      gateway: ${GATEWAY6}
EOF
  fi

  # auth + tls + log
  cat >> "$CONFDIR/relay.yaml" <<EOF

obfuscation:
  enabled: true
  padding:
    enabled: true
    size: 512
crypto:
  key: relay-master-key

auth:
  tokens:
    - name: default
      secret: ${CLIENT_TOKEN}
tls:
  verify_mode: insecure
log:
  level: info
EOF

  echo "Generated default config at $CONFDIR/relay.yaml"
  echo "  -> Upstream server: ${SERVER}"
  echo "  -> Listen: ${LISTEN}"
  echo "  -> Transport: ${TRANSPORT}"
  echo "  -> Client pool: ${SUBNET} (gateway: ${GATEWAY})"
  if [ "$ENABLE_IPV6" = true ]; then
    echo "  -> IPv6 pool: ${SUBNET6} (gateway: ${GATEWAY6})"
  fi
  echo "  -> Direct CIDRs: ${DIRECT_RANGES}"
  echo "  -> Direct domains: ${DIRECT_DOMAINS}"
fi

# --- write systemd unit ---
cat > "$SYSTEMD_DIR/kvn-relay.service" <<SYSTEMD
[Unit]
Description=kvn-ws VPN relay (terminator)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BINDIR}/kvn-relay --config ${CONFDIR}/relay.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536
AmbientCapabilities=CAP_NET_ADMIN
DeviceAllow=/dev/net/tun
StateDirectory=kvn-ws
WorkingDirectory=${CONFDIR}

[Install]
WantedBy=multi-user.target
SYSTEMD

echo "systemd unit written: $SYSTEMD_DIR/kvn-relay.service"

systemctl daemon-reload

# --- enable IP forwarding ---
sysctl_conf="/etc/sysctl.d/99-kvn-relay.conf"
echo "Configuring IP forwarding ..."
cat > "$sysctl_conf" <<SYSCTL
# kvn-ws relay — enable IP forwarding for TUN traffic
net.ipv4.ip_forward=1
SYSCTL
if [ "$ENABLE_IPV6" = true ]; then
  echo "net.ipv6.conf.all.forwarding=1" >> "$sysctl_conf"
fi
sysctl -p "$sysctl_conf" >/dev/null 2>&1

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
echo "  2. If using QUIC upstream with obfuscation, ensure upstream server"
echo "     has obfuscation.enabled: true in its config."
echo "  3. Review config: ${CONFDIR}/relay.yaml"
echo "  4. Enable and start:"
echo "       systemctl enable --now kvn-relay"
echo "  5. Check logs:"
echo "       journalctl -u kvn-relay -f"
echo "  6. Client config (connect to relay):"
echo "       mode: tun"
echo "       server: wss://YOUR_RELAY_IP:8443/tunnel"
echo "       auth.token: ${CLIENT_TOKEN}"
echo "       tls.verify_mode: insecure"
echo ""
echo "  Upstream token (if generated): ${UPSTREAM_TOKEN}"
echo ""
