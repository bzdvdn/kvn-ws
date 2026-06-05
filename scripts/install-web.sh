#!/usr/bin/env sh
# @sk-task web-ci-install-autostart#T3.2: install script for kvn-web
# Usage: sudo ./install.sh [--start]
set -e

BINARY="kvn-web"
SERVICE="kvn-web.service"
PLIST="kvn-web.plist"
BIN_DIR="/usr/local/bin"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ "$(id -u)" -ne 0 ]; then
  echo "This script must be run as root (sudo)." >&2
  exit 1
fi

case "$(uname -s)" in
  Linux)
    echo "Installing kvn-web for Linux (systemd)..."
    cp -f "$SCRIPT_DIR/$BINARY" "$BIN_DIR/$BINARY"
    chmod 755 "$BIN_DIR/$BINARY"
    cp -f "$SCRIPT_DIR/$SERVICE" /etc/systemd/system/$SERVICE
    systemctl daemon-reload
    systemctl enable $SERVICE
    if [ "${1:-}" = "--start" ]; then
      systemctl restart $SERVICE
      echo "Service started."
    fi
    echo "Installed. Manage with: systemctl [start|stop|status] $SERVICE"
    ;;
  Darwin)
    echo "Installing kvn-web for macOS (launchd)..."
    cp -f "$SCRIPT_DIR/$BINARY" "$BIN_DIR/$BINARY"
    chmod 755 "$BIN_DIR/$BINARY"
    mkdir -p /usr/local/var/log
    cp -f "$SCRIPT_DIR/$PLIST" /Library/LaunchDaemons/com.kvn-web.daemon.plist
    chown root:wheel /Library/LaunchDaemons/com.kvn-web.daemon.plist
    if [ "${1:-}" = "--start" ]; then
      launchctl load -w /Library/LaunchDaemons/com.kvn-web.daemon.plist
      echo "Service started."
    fi
    echo "Installed. Manage with: launchctl [load|unload] /Library/LaunchDaemons/com.kvn-web.daemon.plist"
    ;;
  *)
    echo "Unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
esac
