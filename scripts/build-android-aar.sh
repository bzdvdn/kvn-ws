#!/usr/bin/env sh
# Build the gomobile AAR used by the v2 Android client.
set -e
cd "$(dirname "$0")/.."
: "${ANDROID_HOME:?ANDROID_HOME is required}"
if [ -z "${ANDROID_NDK_HOME:-}" ]; then
  ANDROID_NDK_HOME="$(find "$ANDROID_HOME/ndk" -maxdepth 1 -type d -name '28.*' | head -1)"
fi
: "${ANDROID_NDK_HOME:?NDK 28.x is required}"
export ANDROID_HOME ANDROID_NDK_HOME
export PATH="$PATH:$(go env GOPATH)/bin"
mkdir -p bin
gomobile bind -target=android -androidapi 26 -o bin/kvn-v2.aar ./src/mobile
ls -lh bin/kvn-v2.aar
