# src/android

## Purpose

Android v2 client `kvn-v2-client` (`com.kvn.v2client`): Compose UI + Kotlin VpnService + gomobile Go core.

## How it works

- `MainActivity` открывает Compose-экраны; `MainViewModel.connect` собирает v2 JSON и запускает `KvnV2VpnService`.
- `KvnV2VpnService` вызывает `mobile.Mobile.resolve`, настраивает TUN/routes/DNS, передаёт fd в `mobile.Mobile.start`.
- Go core выполняет v2 HTTP/2 + optional QUIC и packet pump; Kotlin не держит v1 WebSocket runtime.
- `client_id` генерируется один раз и хранится в SharedPreferences.
- DNS exclude/direct routing выполняет `internal/client/direct.go`; VpnService ставит DNS и перехватывает трафик в TUN.
- `ConnectionConfig.quicEnabled` (Settings → Tunnel → QUIC datagrams) управляет QUIC для UDP; по умолчанию off, fallback — HTTP/2 UDP.

## Key files

- `.../ui/MainViewModel.kt` — v2 JSON, lifecycle, metrics.
- `.../ui/SettingsScreen.kt` — TLS, QUIC, DNS, apps, diagnostics.
- `.../ui/AppPickerScreen.kt` — include/exclude apps.
- `.../ui/QrScannerScreen.kt` — QR/JSON import, web parity.
- `.../config/AppConfig.kt` — DataStore multi-server config + `quicEnabled`.
- `.../vpn/KvnV2VpnService.kt` — TUN/VpnService bridge.
- `src/mobile/mobile.go` — AAR API.

## Gotchas

- Сначала соберите AAR: `./scripts/build-android-aar.sh`, затем `./gradlew assembleDebug`.
- `addDisallowedApplication(packageName)` держит Go/transport сокеты вне VPN-петли.
- Legacy Kotlin transport/protocol classes остаются в дереве, но v2 runtime их не использует.
- Для self-signed сервера включайте `Allow insecure TLS`; для QUIC сервер должен слушать UDP 443.
- `QUIC` default off: при недоступном UDP клиент делает fallback на HTTP/2 UDP после короткой попытки.
