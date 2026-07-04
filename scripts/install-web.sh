#!/usr/bin/env bash
# @sk-task web-ci-install-autostart#T3.2: install script for kvn-web
# Usage:
#   sudo ./install-web.sh [--start [--port 2311]]
#   curl -sL https://github.com/bzdvdn/kvn-ws/releases/latest/download/install-web.sh | sudo bash
set -euo pipefail

REPO="bzdvdn/kvn-ws"
VERSION="latest"
BIN_DIR="/usr/local/bin"
SERVICE_DIR="/etc/systemd/system"
PLIST_DIR="/Library/LaunchDaemons"
PLIST_LABEL="com.kvn-web.daemon"

START=false
PORT="2311"
DESKTOP=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --start)   START=true; shift ;;
    --port)    PORT="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --desktop) DESKTOP=true; shift ;; # @sk-task kvn-desktop#T4.2: --desktop flag (AC-007)
    --help|-h)
      echo "Usage: sudo $0 [--start] [--port 2311] [--version <tag>] [--desktop]"
      exit 0 ;;
    *) echo "Unknown: $1"; exit 1 ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then
  echo "This script must be run as root (sudo)." >&2
  exit 1
fi

SCRIPT_DIR="$(dirname "$0" 2>/dev/null)" && SCRIPT_DIR="$(cd "$SCRIPT_DIR" && pwd)" || SCRIPT_DIR=""

# --- detect OS/arch ---
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64"  ;;
  arm64)   ARCH="arm64"  ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# --- use local binary if present, otherwise download ---
BINARY_SRC=""
SERVICES_DIR="$SCRIPT_DIR"
if [ -f "$SCRIPT_DIR/kvn-web" ]; then
  BINARY_SRC="$SCRIPT_DIR/kvn-web"
  echo "Using local binary: $BINARY_SRC"
elif [ -f "$SCRIPT_DIR/bin/kvn-web" ]; then
  BINARY_SRC="$SCRIPT_DIR/bin/kvn-web"
  echo "Using local binary: $BINARY_SRC"
fi

if [ -z "$BINARY_SRC" ]; then
  if [ "$VERSION" = "latest" ]; then
    VERSION="$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)"
    echo "Resolved latest version: $VERSION"
  fi

  ARCHIVE="kvn-${OS}-${ARCH}.tar.gz"
  URL="https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE"
  CHECKSUM_URL="https://github.com/$REPO/releases/download/$VERSION/${ARCHIVE}.sha256"

  TMPDIR="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR"' EXIT
  SERVICES_DIR="$TMPDIR"

  echo "Downloading $URL ..."
  curl -sL "$URL" -o "$TMPDIR/$ARCHIVE"

  echo "Verifying checksum ..."
  CHECKSUM_LINE="$(curl -sL "$CHECKSUM_URL" 2>/dev/null | grep -E '^[a-f0-9]{64}[[:space:]]' || true)"
  if [ -n "$CHECKSUM_LINE" ]; then
    CHECKSUM_VAL="${CHECKSUM_LINE%%[[:space:]]*}"
    printf "%s  %s\n" "$CHECKSUM_VAL" "$TMPDIR/$ARCHIVE" | sha256sum -c - || {
      echo "Checksum verification FAILED." >&2
      exit 1
    }
  else
    echo "Checksum file not found, skipping verification."
  fi

  echo "Extracting ..."
  tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR" --strip-components=1
  BINARY_SRC="$TMPDIR/kvn-web"
fi

DESKTOP_BINARY_SRC=""
if [ "$DESKTOP" = true ]; then
  if [ -f "$SCRIPT_DIR/kvn-desktop" ]; then
    DESKTOP_BINARY_SRC="$SCRIPT_DIR/kvn-desktop"
  elif [ -f "$SCRIPT_DIR/bin/kvn-desktop" ]; then
    DESKTOP_BINARY_SRC="$SCRIPT_DIR/bin/kvn-desktop"
  else
    # fallback: download archive does not contain kvn-desktop yet
    echo "WARN: kvn-desktop binary not found locally, skipping."
    echo "  Build it with: go build -o bin/kvn-desktop ./src/cmd/desktop"
  fi
fi

case "$OS" in
  linux)
    echo "Installing kvn-web for Linux (systemd)..."
    install -m 0755 "$BINARY_SRC" "$BIN_DIR/kvn-web"
    if [ -f "$SERVICES_DIR/kvn-web.service" ]; then
      install -m 644 "$SERVICES_DIR/kvn-web.service" "$SERVICE_DIR/kvn-web.service"
    else
      cat > "$SERVICE_DIR/kvn-web.service" <<UNIT
[Unit]
Description=KVN Web UI
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
ExecStart=$BIN_DIR/kvn-web --no-browser --port $PORT
Restart=on-failure
RestartSec=5
Environment=HOME=/root

[Install]
WantedBy=multi-user.target
UNIT
    fi
    systemctl daemon-reload
    systemctl enable kvn-web.service
    if [ "$START" = true ]; then
      systemctl restart kvn-web.service
      echo "Service started."
    fi
    echo "Installed. Manage with: systemctl [start|stop|status] kvn-web.service"
    if [ "$DESKTOP" = true ] && [ -n "$DESKTOP_BINARY_SRC" ]; then
      install -m 0755 "$DESKTOP_BINARY_SRC" "$BIN_DIR/kvn-desktop"
      mkdir -p /usr/local/share/applications
      cat > /usr/local/share/applications/kvn-desktop.desktop <<DESKTOP_FILE
[Desktop Entry]
Name=KVN Desktop
Comment=KVN Web UI desktop wrapper
Exec=$BIN_DIR/kvn-desktop
Icon=preferences-system-network
Terminal=false
Type=Application
Categories=Network;Utility;
DESKTOP_FILE
      echo "  Desktop: $BIN_DIR/kvn-desktop + /usr/local/share/applications/kvn-desktop.desktop"
    fi
    ;;
  darwin)
    echo "Installing kvn-web for macOS (launchd)..."
    install -m 0755 "$BINARY_SRC" "$BIN_DIR/kvn-web"
    mkdir -p /usr/local/var/log
    install -m 644 "$TMPDIR/kvn-web.plist" "$PLIST_DIR/$PLIST_LABEL.plist" 2>/dev/null || {
      cat > "$PLIST_DIR/$PLIST_LABEL.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>$PLIST_LABEL</string>
    <key>ProgramArguments</key>
    <array>
        <string>$BIN_DIR/kvn-web</string>
        <string>--no-browser</string>
        <string>--port</string>
        <string>$PORT</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>ThrottleInterval</key>
    <integer>5</integer>
    <key>StandardOutPath</key>
    <string>/usr/local/var/log/kvn-web.log</string>
    <key>StandardErrorPath</key>
    <string>/usr/local/var/log/kvn-web.log</string>
</dict>
</plist>
PLIST
    }
    chown root:wheel "$PLIST_DIR/$PLIST_LABEL.plist"
    if [ "$START" = true ]; then
      launchctl load -w "$PLIST_DIR/$PLIST_LABEL.plist"
      echo "Service started."
    fi
    echo "Installed. Manage with: launchctl [load|unload] $PLIST_DIR/$PLIST_LABEL.plist"
    if [ "$DESKTOP" = true ] && [ -n "$DESKTOP_BINARY_SRC" ]; then
      install -m 0755 "$DESKTOP_BINARY_SRC" "$BIN_DIR/kvn-desktop"
      mkdir -p "/Applications/KVN Desktop.app/Contents/MacOS"
      cat > "/Applications/KVN Desktop.app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>KVN Desktop</string>
    <key>CFBundleDisplayName</key>
    <string>KVN Desktop</string>
    <key>CFBundleExecutable</key>
    <string>kvn-desktop</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
</dict>
</plist>
PLIST
      ln -sf "$BIN_DIR/kvn-desktop" "/Applications/KVN Desktop.app/Contents/MacOS/kvn-desktop"
      echo "  Desktop: $BIN_DIR/kvn-desktop + /Applications/KVN Desktop.app"
    fi
    ;;
  *)
    echo "Unsupported OS: $OS" >&2
    exit 1
    ;;
esac

echo ""
echo "=== kvn-web installation complete ==="
echo "  Binary: $BIN_DIR/kvn-web"
echo "  Web UI: http://127.0.0.1:$PORT"
