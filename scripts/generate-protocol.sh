#!/usr/bin/env sh
# @sk-task kvn-android#T1.2: generate Go and Kotlin types from protocol YAML (AC-004)
# shellcheck disable=all
set -e
cd "$(dirname "$0")/.."
echo "Generating protocol types..."
# @sk-task kvn-android#T1.2: actual codegen invocation (AC-004)
go run ./protocol/codegen/
echo "Done."
