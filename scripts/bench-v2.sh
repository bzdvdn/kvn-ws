#!/usr/bin/env bash
# Benchmark v2 TCP/UDP path. Requires kvn-web/v2 client already connected.
# Default measures a TUN-routed download; -p uses the local SOCKS5 proxy.
set -euo pipefail

MODE=down
SIZE_MB=20
PROXY=""
TARGET="${V2_BENCH_TARGET:-https://speed.cloudflare.com}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -p) PROXY="${2:?proxy addr}"; shift 2 ;;
    -s) SIZE_MB="${2:?size MB}"; shift 2 ;;
    -u) MODE=up; shift ;;
    -t) TARGET="${2:?target URL}"; shift 2 ;;
    -h|--help) sed -n '1,12p' "$0"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

proxy_args=()
if [[ -n "$PROXY" ]]; then
  proxy_args=(--socks5-hostname "$PROXY")
fi

if [[ "$MODE" == "down" ]]; then
  echo "v2 download: ${SIZE_MB} MB via ${PROXY:-TUN} ..."
  bytes=$((SIZE_MB * 1024 * 1024))
  curl -sS "${proxy_args[@]}" -o /dev/null -w 'download: %{speed_download} B/s, total: %{time_total}s\n' \
    "${TARGET}/__down?bytes=${bytes}"
else
  echo "v2 upload: ${SIZE_MB} MB via ${PROXY:-TUN} ..."
  tmp=$(mktemp)
  trap 'rm -f "$tmp"' EXIT
  head -c $((SIZE_MB * 1024 * 1024)) /dev/zero > "$tmp"
  curl -sS "${proxy_args[@]}" -X POST -o /dev/null -w 'upload: %{speed_upload} B/s, total: %{time_total}s\n' \
    --data-binary @"$tmp" "${TARGET}/__up"
fi
