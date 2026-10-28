# Constitution Summary — kvn-ws 2.0

**Purpose:** v2-only VPN: TLS 1.3 + HTTP/2 TCP, optional QUIC UDP, TUN/proxy, uTLS surface, fallback; Go, WebView desktop, Android.

**Non-negotiables:**
- v1 runtime, entrypoints and binaries MUST NOT return to the current tree/build; v1 stays in old releases only.
- Unified config MUST be `~/.config/kvn/config.yaml`; v1 file stays read-only as `config.yaml.v1.bak`.
- v2 MUST use TLS 1.3/HTTP2 with no KVN-specific paths/headers by default; unknown requests get fallback.
- `verify_mode=insecure` only disables certificate verification, MUST NOT be default, and does not guarantee DPI bypass.
- Every `[x]` task MUST have `Proof:`; public behavior changes MUST be in spec/tasks first.

**Stack/Architecture:**
- Go 1.26: HTTP/2, quic-go, uTLS, viper, zap, gVisor, tun, webview, gomobile.
- `src/cmd/{server,client,web,desktop}`, `src/internal/*`, `src/mobile`, `src/android`.
- Android Kotlin 1.9.20 + Compose/M3; VpnService owns TUN fd; Go core does not close it.
- Server assigns unique persisted TUN leases by `client_id`.
- Windows installer defaults to `kvn-desktop` (WebView) + `kvn-web`; `-Browser` is legacy.

**Workflow/DoD:**
- Branches: `feature/<slug>` from spec/propose.
- Go: `go test ./src/...`, `go vet ./src/...`, `gosec`, `golangci-lint`.
- Android: `testDebugUnitTest`, `assembleDebug`.
- DPI: lab report required; field run before claiming bypass.
- `REPOSITORY_MAP.md` updated in-place on structural changes.

**Languages:** docs=ru/en, agent=ru, comments=en
