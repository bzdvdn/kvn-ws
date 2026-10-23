# Context — src/android

## Purpose

Android-клиент KVN (`com.kvn.client`): VPN-приложение на Jetpack Compose, которое поднимает `VpnService`, устанавливает WebSocket-туннель к серверу KVN, пересылает IP-пакеты между TUN и туннелем и умеет выборочно пускать трафик напрямую (split tunnel + fake DNS).

## Responsibilities

- Владеет: UI (экраны Compose), multi-server конфигом в DataStore, `VpnService` и TUN, WS-транспортом, кодированием кадров/handshake, AES-GCM для data-кадров, fake-DNS/fake-IP маршрутизацией, reconnect-логикой, логгером и QR-импортом/экспортом.
- Не владеет: серверной частью, relay/desktop/web UI, правилами geoip/geosite (поля конфига есть, реального резолвера на Android нет).

## How it works

- `MainActivity` — 4 таба (`ConnectScreen`, `SettingsScreen`, `LogViewerScreen`, `TrafficScreen`), тёмная тема, перехват crash-стека в `filesDir/crash.txt` с показом диалога при следующем запуске.
- `MainViewModel` держит `StateFlow` состояния/трафика/reconnect'ов, читает и пишет `AppConfig{activeServer, servers[]}` через `AppConfigStore` (Preferences DataStore `kvn_config`, JSON); legacy-конфиг мигрируется в `servers=[Default]`. Состояние соединения дублируется в SharedPreferences `kvn_state`.
- `KvnVpnService.start(...)` (static) кладёт `ConnectionConfig` и callbacks в companion и стартует foreground-service; `onStartCommand` → `doStart` → уведомление, pre-resolve IP сервера, WakeLock/WifiLock, SCREEN_ON-receiver, `ReconnectManager`, WebSocket-connect, network callback.
- На `CONNECTED` → `performHandshake`: ClientHello (`PROTO_VERSION=0x02`, token, mtu, ipv6, transport `"tcp"`), ответ `HELLO` → `sessionId`, crypto-salt → ключ через `AesGcmCipher.deriveKey`, `establishTun(assignedIp, assignedIpv6)`.
- `establishTun`: адрес `/32` (+ `/128` при IPv6), routes из `routingIncludeRanges` минус `routingExcludeRanges` (CIDR-вычитание), `dnsServers`, per-app фильтр ровно в одном режиме (`addAllowedApplication` XOR `addDisallowedApplication`).
- `tunReader` (корутина IO): читает пакеты, считает TX; при `routingDomainsEnabled` сначала `routePacket` (direct/fake-DNS), иначе шифрует и отправляет `DATA`; UDP при активном secondary уходит туда (с батчингом 4096 Б / ~5 мс), остальное — в primary.
- `handleFrame`: `HELLO` (сессия/крипта/TUN/secondary), `AUTH` (остановка), `DATA` (RX, decrypt, запись в TUN), `PROXY` (payload пишется в TUN как есть), `CLOSE` (остановка).
- Secondary-канал (`multi_channel`): второй WS + ClientHello с `channel="secondary"` и `sessionId`; сервер обязан вернуть тот же `sessionId`, батчинг включается только по `batchSupport`; при любой проблеме клиент остаётся на primary.
- Split tunnel: `FakeDnsResolver` перехватывает UDP/53, для `routingExcludeDomains` резолвит через физическую сеть и запоминает IP, для `routingIncludeDomains` выдаёт fake-IP из `FakeIpPool` (198.18.0.0 + до 32768); `DirectDeliverer` для исключённых IP реализует user-space TCP (SYN-ACK/seq/ack/checksums) и UDP через `protect()`-сокеты на `defaultNetwork`.
- `ReconnectManager`: до 10 попыток, экспоненциальный backoff от `minBackoffSec` до `maxBackoffSec` + jitter, по исчерпании — `safeStop(force=true)`.
- `WebSocketClient` (OkHttp): бинарные сообщения, опциональный padding (4-байтовый BE length + случайные байты до кратности `paddingSize`), `toFrames()` разбирает и батч-сообщения.

## Key files

- `src/android/app/src/main/kotlin/com/kvn/client/vpn/KvnVpnService.kt` — сердце: service lifecycle, TUN, handshake, frame-loop, secondary, fake-DNS/direct delivery, reconnect, notifications.
- `.../ui/MainViewModel.kt` + `.../ui/MainActivity.kt` — состояние приложения, навигация, crash-отчёт.
- `.../config/AppConfig.kt` — `ConnectionConfig`, `ServerEntry`, `AppConfig`, `AppConfigStore` (DataStore + миграция + JSON).
- `.../transport/WebSocketClient.kt`, `.../transport/reconnect/ReconnectManager.kt`, `.../transport/TransportClient.kt`.
- `.../protocol/{Frames,FrameCodec,Handshake,HandshakeClient}.kt` — сгенерированные константы и кодеки.
- `.../dns/{FakeDnsResolver,FakeIpPool,DnsCache,DnsParser}.kt` — split-tunnel DNS.
- `.../crypto/AesGcmCipher.kt`, `.../logger/{AppLogger,LogEntry}.kt`.
- `src/android/app/build.gradle.kts`, `src/android/app/src/main/AndroidManifest.xml` — сборка/разрешения.

## Public contracts

- `ConnectionConfig` (поля `@Serializable`, дефолты: port 443, path `/kvn`, mode `tun`, transport `tcp`, MTU 1500, `tlsVerifyMode=verify`) и `AppConfig{activeServer, servers[]}` — JSON-формат хранилища и QR.
- `KvnVpnService.start(context, cfg, appIncludeList, appExcludeList, dnsServers, callbacks...)` / `KvnVpnService.stop(context)`; callbacks: state, traffic, error, dualChannel.
- `TransportClient`: `connect()`, `send(Frame)`, `disconnect()`, `isConnected()`; `ConnectionState{CONNECTING, CONNECTED, DISCONNECTING, DISCONNECTED, RECONNECTING}`.
- Кадры — те же, что у Go-клиента: 4-байтовый заголовок `type|flags|uint16 BE length`, типы/флаги генерируются из `protocol/frames.yaml`, handshake — из `protocol/handshake.yaml`.
- QR: Android JSON `ConnectionConfig`, web-JSON (конвертируется `webToAndroidConfig`), legacy `server:port:token`; `parseQrConfig(raw)`.

## Dependencies

- AndroidX Compose/Material3, Activity, Lifecycle; OkHttp 4.12; DataStore Preferences; kotlinx-serialization-json; coroutines; CameraX + ML Kit barcode; ZXing core.
- Внешний контракт — Go-сервер KVN (`protocol/*.yaml`, WS-padding, handshake-теги).

## Consumed by

- N/A — это leaf-приложение (APK); его API наружу не импортируется.

## Gotchas

- Реализован только WebSocket-транспорт; QUIC на Android отсутствует, а `ConnectionConfig.transport` в hello всегда отправляется как `"tcp"`.
- `tlsVerifyMode`: `"insecure"` — trust-all + `HostnameVerifier=true`; `"none"` — **plaintext** `ws://` (TLS выключен); `"verify"` — обычная проверка OkHttp, `tlsServerName` подменяет ожидаемый hostname.
- `FrameCodec.toFrames()` молча обрывается на неполном хвосте батча — повреждённый батч не считается ошибкой, оставшиеся кадры теряются.
- Шифруются только `DATA`-кадры; `PROXY`-payload пишется в TUN без расшифровки (совместимо с сервером, где proxy-кадры тоже не шифруются).
- `safeStop` при `killSwitchEnabled && !killed && !force` не останавливает сервис: он лишь отключает транспорт и показывает DISCONNECTED (kill switch удерживает приложение активным).
- `tunReader` ловит любое исключение и **завершает цикл** — до следующего reconnect TUN не читается.
- `DirectDeliverer` — упрощённый TCP-стек: без retransmit/окон/out-of-order, только IPv4; при объёме payload ≤512 Б пишет hex/ASCII в лог (приватность).
- `FakeDnsResolver` работает только при `routingDomainsEnabled`; AAAA-запросы всегда получают пустой ответ; при отсутствии `defaultNetwork`/DNS-серверов include/exclude-домены не резолвятся и трафик уходит в туннель.
- UDP-сокеты и direct-TCP используют `protect()`/`bindSocket(defaultNetwork)`, иначе пакеты уйдут в собственный VPN и зациклятся.
- `establishTun` применяет per-app фильтр только в одном режиме: при непустом include-списке exclude игнорируется.
- Release-сборка требует `keystore.properties` (иначе signingConfig пустой); `versionCode` берётся из `git rev-list --count HEAD` или `-PversionCode`.
- Константы кадров/handshake генерируются из `protocol/*.yaml` — правки в Kotlin будут перезаписаны кодогенерацией.
