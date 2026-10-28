# src/android

## Purpose
Android v2 client `kvn-v2-client` (`com.kvn.v2client`): Compose UI + Kotlin VpnService + gomobile Go core.

## How it works
- `MainActivity` opens Compose screens; `MainViewModel.connect` builds v2 JSON and starts `KvnV2VpnService`.
- `KvnV2VpnService` calls `mobile.Mobile.resolve`, builds TUN/routes/DNS, detaches fd and calls `mobile.Mobile.start`.
- Go core owns v2 HTTP/2/QUIC and TUN packet pump; Kotlin does not run a v1 WebSocket engine.
- `client_id` is generated once and stored in SharedPreferences.
- DNS exclude domains/direct routing is handled in Go (`internal/client/direct.go`); VpnService sets DNS `1.1.1.1`, intercepted at TUN.

## Key files
- `.../ui/MainViewModel.kt` — v2 config JSON + lifecycle.
- `.../vpn/KvnV2VpnService.kt` — TUN/VpnService bridge.
- `.../config/AppConfig.kt` — DataStore multi-server config.
- `src/mobile/mobile.go` — AAR API.
- `src/android/app/libs/kvn-v2.aar` — generated AAR (gitignored).

## Gotchas
- Build AAR first: `./scripts/build-android-aar.sh`; copy to `app/libs`.
- Old `KvnVpnService` and v1 transport classes are no longer used.
- `addDisallowedApplication(packageName)` keeps Go/transport sockets outside the VPN loop.
- DNS fake-IP is not used; exclude domains get real direct IPs.
