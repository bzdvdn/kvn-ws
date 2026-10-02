#!/usr/bin/env bash
# kvn-speedtest — замер пропускной способности к фиксированному endpoint.
# Запускайте до VPN и после VPN к одному и тому же target и сравнивайте.
#
# Usage:
#   ./scripts/speedtest.sh            # download 50 MB (1 поток)
#   ./scripts/speedtest.sh -s 100     # download 100 MB (Cloudflare может 403 на 100+)
#   ./scripts/speedtest.sh -p 4       # 4 параллельных потока (агрегированная скорость)
#   ./scripts/speedtest.sh -u         # upload 50 MB
#   ./scripts/speedtest.sh -u -b file.bin   # upload an existing file
#
# Один поток (curl) ограничен TCP window/RTT (bandwidth-delay product), поэтому
# -p N показывает агрегированную пропускную способность канала/туннеля.
# Cloudflare speed (__down) блокирует параллельные download (403), поэтому -p N
# работает для upload; для download используйте -p 1 или свой SPEED_URL_BASE.
# @sk-task speedtest-script#T1.1: curl throughput to a fixed target (AC-001)

set -euo pipefail

SIZE_MB=50
PARALLEL=1
MODE=down
UPLOAD_FILE=""
SPEED_URL_BASE="${SPEED_URL_BASE:-https://speed.cloudflare.com}"
UA="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36"
MAXTIME=300

while [[ $# -gt 0 ]]; do
  case "$1" in
    -s) SIZE_MB="${2:?size in MB}"; shift 2 ;;
    -p) PARALLEL="${2:?parallel streams}"; shift 2 ;;
    -u) MODE=up; shift ;;
    -b) UPLOAD_FILE="${2:?file path}"; MODE=up; shift 2 ;;
    -h|--help) sed -n '1,16p' "$0"; exit 0 ;;
    *) echo "unknown option: $1"; exit 2 ;;
  esac
done

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

run_stream() {  # $1 = payload or per-stream bytes, $2 = stream index
  if [[ "$MODE" == "down" ]]; then
    curl -s -A "$UA" --max-time "$MAXTIME" -o /dev/null -w "%{speed_download}" \
      "${SPEED_URL_BASE}/__down?bytes=$1" > "$TMPDIR/s$2"
  else
    curl -X POST -s -A "$UA" --max-time "$MAXTIME" -o /dev/null -w "%{speed_upload}" \
      --data-binary @"$1" "${SPEED_URL_BASE}/__up" > "$TMPDIR/s$2"
  fi
}

if [[ "$MODE" == "down" ]]; then
  total_bytes=$((SIZE_MB * 1024 * 1024))
  per_bytes=$((total_bytes / PARALLEL))
  echo "download: ${SIZE_MB} MB (${PARALLEL} потоков) from ${SPEED_URL_BASE}/__down ..."
  for i in $(seq 1 "$PARALLEL"); do
    run_stream "$per_bytes" "$i" &
  done
else
  if [[ -z "$UPLOAD_FILE" ]]; then
    per=$((SIZE_MB * 1024 * 1024 / PARALLEL))
    for i in $(seq 1 "$PARALLEL"); do
      head -c "$per" /dev/zero > "$TMPDIR/u$i"
    done
  else
    for i in $(seq 1 "$PARALLEL"); do
      cp "$UPLOAD_FILE" "$TMPDIR/u$i"
    done
  fi
  echo "upload: ${SIZE_MB} MB (${PARALLEL} потоков) to ${SPEED_URL_BASE}/__up ..."
  for i in $(seq 1 "$PARALLEL"); do
    run_stream "$TMPDIR/u$i" "$i" &
  done
fi

wait

total=0
for i in $(seq 1 "$PARALLEL"); do
  v=$(cat "$TMPDIR/s$i" 2>/dev/null || echo 0)
  total=$(awk "BEGIN{print $total+$v}")
done

if [[ "$total" == "0" ]]; then
  if [[ "$MODE" == "down" && "$PARALLEL" -gt 1 ]]; then
    echo "error: Cloudflare блокирует параллельные downloads (403). Используйте -p 1 для download или SPEED_URL_BASE на свой сервер; -p N работает для upload."
  else
    echo "error: endpoint вернул 0 байт (403/блок). Попробуйте меньший -s или проверьте connectivity."
  fi
  exit 1
fi
mbps=$(awk "BEGIN{printf \"%.2f\", (${total}*8)/1e6}")
echo "result: ${mbps} Mbps (aggregate ${PARALLEL} streams)"