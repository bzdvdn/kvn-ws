# Repository Map — kvn-ws 2.0

v2-only Go backend/CLI under `src/`; Android/Kotlin v2 client under `src/android/`; gomobile core under `src/mobile/`.

## Entry Points
- `src/cmd/server/main.go` — v2 server (HTTP/2 TCP + optional QUIC UDP, pool session).
- `src/cmd/client/main.go` — v2 standalone client (TUN/proxy).
- `src/cmd/web/main.go` — `kvn-web`, v2-only UI, unified `~/.config/kvn/config.yaml`.
- `src/cmd/desktop/main.go` — native WebView wrapper for `kvn-web`.
- `src/android/app/src/main/kotlin/com/kvn/client/ui/MainActivity.kt` — Android Compose UI.
- `src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnV2VpnService.kt` — Kotlin VpnService + TUN fd.
- `src/mobile/mobile.go` — gomobile AAR boundary (`Resolve`, `Start`, `Stop`).

## Core Packages
- `src/internal/client/` — v2 data plane, TUN/proxy, session, DNS direct routing.
- `src/internal/server/` — v2 fallback site.
- `src/internal/transport/` — HTTP/2 stream client/server, session endpoint, padding, QUIC.
- `src/internal/tlsprofile/` — uTLS Chrome/Firefox/Safari profiles + fallback.
- `src/internal/tcpstack/` — gVisor TCP split/forwarder.
- `src/internal/tunpool/` — unique IPv4 leases, TTL, persisted lease store.
- `src/internal/config/` — unified client/server/webui config and v2 runtime schema.
- `src/internal/routing/` — split-tunnel rules, domain matcher, DNS tracker.
- `src/internal/dns/` — DNS cache/tracker.
- `src/internal/dnsproxy/` — local DNS proxy for desktop.
- `src/internal/proxy/` — SOCKS5/HTTP CONNECT listener.
- `src/internal/tun/` — platform TUN device.
- `src/internal/webui/` — v2-only web backend + embedded React SPA.
- `src/internal/transport/tls/` — standard TLS config helpers.
- `src/internal/logger/` — zap logging.

## Android
- `src/android/app/src/main/kotlin/com/kvn/client/config/` — DataStore config, QR model.
- `src/android/app/src/main/kotlin/com/kvn/client/ui/` — Compose screens/ViewModel.
- `src/android/app/src/main/kotlin/com/kvn/client/vpn/` — `KvnV2VpnService`, tunnel lifecycle.
- `src/android/app/src/main/kotlin/com/kvn/client/dns/` — legacy DNS helpers (v2 direct routing lives in Go core).
- `src/android/app/src/main/kotlin/com/kvn/client/logger/` — app logs.
- `src/android/app/src/main/res/` — theme, `ic_launcher_v2` adaptive icon.

## Key Paths
- `configs/client.example.yaml`, `configs/server.example.yaml` — v2 examples.
- `configs/server.example.yaml` — includes required `tun` pool and `lease_file`.
- `bin/kvn-client`, `bin/kvn-server`, `bin/kvn-web` — build outputs.
- `bin/kvn-v2.aar` — gomobile Android library.
- `scripts/build.sh` — v2 client/server/web/desktop build.
- `scripts/build-android-aar.sh` — gomobile AAR build.
- `scripts/build-android.sh` — Android APK build.
- `scripts/dpi-check.sh` — TLS/H2/active-probe lab gate.
- `scripts/check-v2-only.sh` — guard against v1 runtime reintroduction.
- `Dockerfile` — v2 client/server/web image.
- `.github/workflows/ci.yml` — Go/Android/desktop CI/release.
- `docs/ru|en/v2.md` — v2 usage/config.
- `docs/ru|en/v1-legacy.md` — old v1 releases only.
- `docs/ru|en/v2-release.md` — release checklist.
- `docs/ru|en/v2-testers.md` — tester smoke checklist.
- `ROADMAP.md` — 2.x DPI/cover/multi-server roadmap.
- `docs/ru|en/v2-dpi-report.md` — DPI evidence.
- `CHANGELOG.md`, `README.md` — 2.0 release.

## Where To Edit
- TCP/HTTP2/UDP transport — `src/internal/transport/`.
- uTLS profiles — `src/internal/tlsprofile/`.
- Client data plane/TUN/DNS direct — `src/internal/client/`.
- Server pool/session — `src/internal/tunpool/`, `src/internal/transport/session.go`.
- Web UI/config — `src/internal/webui/`, `src/cmd/web/`, `src/internal/config/`.
- Android UI/VpnService — `src/android/app/src/main/kotlin/com/kvn/client/`.
- gomobile API — `src/mobile/`.
- Build/release — `scripts/`, `.github/workflows/`, `Dockerfile`.

## Modules & Context
- `src/mobile/CONTEXT.md` — AAR boundary and fd contract.
- `src/internal/client/CONTEXT.md` — v2 desktop/mobile data plane.
- `src/internal/server/CONTEXT.md` — fallback server.
- `src/internal/transport/CONTEXT.md` — HTTP/2/QUIC transport.
- `src/internal/tunpool/CONTEXT.md` — lease pool.
- `src/internal/config/CONTEXT.md` — unified config.
- `src/internal/webui/CONTEXT.md` — v2-only UI.
- `src/android/CONTEXT.md` — Android v2 app.
- `src/cmd/CONTEXT.md` — entrypoints.

## Excluded
- `.speckeep/**`, `specs/archived/**`, `specs/active/**`
- `bin/`, `certs/`, `vendor/`
- `src/internal/webui/frontend/node_modules/`, `src/internal/webui/frontend/dist/`
