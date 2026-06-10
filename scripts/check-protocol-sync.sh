#!/usr/bin/env sh
# @sk-task kvn-android#T3.3: verify generated Go+Kotlin match YAML specs (AC-004)
set -e
cd "$(dirname "$0")/.."

echo "=== AC-004: Protocol sync check ==="

# Backup generated files
cp src/internal/transport/framing/types_gen.go /tmp/types_gen_go_framing.bak
cp src/internal/protocol/handshake/types_gen.go /tmp/types_gen_go_handshake.bak
cp src/android/app/src/main/kotlin/com/kvn/client/protocol/Frames.kt /tmp/Frames_kt.bak 2>/dev/null || true
cp src/android/app/src/main/kotlin/com/kvn/client/protocol/Handshake.kt /tmp/Handshake_kt.bak 2>/dev/null || true

# Regenerate
go run ./protocol/codegen/

# Check Go diffs
GO_FRAMING_DIFF=0
GO_HANDSHAKE_DIFF=0

if ! diff -q src/internal/transport/framing/types_gen.go /tmp/types_gen_go_framing.bak >/dev/null 2>&1; then
    GO_FRAMING_DIFF=1
fi
if ! diff -q src/internal/protocol/handshake/types_gen.go /tmp/types_gen_go_handshake.bak >/dev/null 2>&1; then
    GO_HANDSHAKE_DIFF=1
fi

# Check Kotlin diffs (if backup exists)
KT_FRAMING_DIFF=0
KT_HANDSHAKE_DIFF=0
if [ -f /tmp/Frames_kt.bak ]; then
    if ! diff -q src/android/app/src/main/kotlin/com/kvn/client/protocol/Frames.kt /tmp/Frames_kt.bak >/dev/null 2>&1; then
        KT_FRAMING_DIFF=1
    fi
fi
if [ -f /tmp/Handshake_kt.bak ]; then
    if ! diff -q src/android/app/src/main/kotlin/com/kvn/client/protocol/Handshake.kt /tmp/Handshake_kt.bak >/dev/null 2>&1; then
        KT_HANDSHAKE_DIFF=1
    fi
fi

# Restore backups
cp /tmp/types_gen_go_framing.bak src/internal/transport/framing/types_gen.go
cp /tmp/types_gen_go_handshake.bak src/internal/protocol/handshake/types_gen.go
[ -f /tmp/Frames_kt.bak ] && cp /tmp/Frames_kt.bak src/android/app/src/main/kotlin/com/kvn/client/protocol/Frames.kt || true
[ -f /tmp/Handshake_kt.bak ] && cp /tmp/Handshake_kt.bak src/android/app/src/main/kotlin/com/kvn/client/protocol/Handshake.kt || true

TOTAL_DIFF=$((GO_FRAMING_DIFF + GO_HANDSHAKE_DIFF + KT_FRAMING_DIFF + KT_HANDSHAKE_DIFF))

echo "Go framing:   $([ $GO_FRAMING_DIFF -eq 0 ] && echo 'OK' || echo 'CHANGED')"
echo "Go handshake: $([ $GO_HANDSHAKE_DIFF -eq 0 ] && echo 'OK' || echo 'CHANGED')"
echo "Kotlin frames:  $([ $KT_FRAMING_DIFF -eq 0 ] && echo 'OK' || echo 'CHANGED')"
echo "Kotlin handshake: $([ $KT_HANDSHAKE_DIFF -eq 0 ] && echo 'OK' || echo 'CHANGED')"

if [ "$TOTAL_DIFF" -eq 0 ]; then
    echo "AC-004: PASS — generated files match YAML specs"
else
    echo "AC-004: FAIL — some generated files differ from YAML specs"
    exit 1
fi
