#!/usr/bin/env sh
# @sk-task kvn-android#T4.2: build APK locally (AC-001)
set -e
cd "$(dirname "$0")/.."

echo "=== Building KVN Android APK ==="

# 1. Generate protocol types first
echo "[1/4] Generating protocol types..."
go run ./protocol/codegen/

# 2. Set ANDROID_HOME if not set
if [ -z "$ANDROID_HOME" ]; then
    # Common default paths
    for dir in "$HOME/Android/Sdk" "/usr/lib/android-sdk" "/opt/android-sdk"; do
        if [ -d "$dir" ]; then
            export ANDROID_HOME="$dir"
            echo "  ANDROID_HOME=$ANDROID_HOME"
            break
        fi
    done
fi

if [ -z "$ANDROID_HOME" ]; then
    echo "ERROR: ANDROID_HOME not set. Install Android SDK first:"
    echo "  https://developer.android.com/studio#command-line-tools-only"
    exit 1
fi

# 3. Init Gradle wrapper if needed
echo "[2/4] Checking Gradle wrapper..."
if [ ! -f src/android/gradle/wrapper/gradle-wrapper.jar ]; then
    if command -v gradle >/dev/null 2>&1; then
        echo "  Generating wrapper..."
        (cd src/android && gradle wrapper --gradle-version 8.5)
    else
        echo "  WARNING: gradle-wrapper.jar missing. Install Gradle or copy from an existing project."
        echo "  To generate: cd src/android && gradle wrapper --gradle-version 8.5"
    fi
fi

# 4. Build APK
echo "[3/4] Building APK..."
(cd src/android && ./gradlew assembleDebug)

echo "[4/4] Done!"
echo "APK: src/android/app/build/outputs/apk/debug/app-debug.apk"
