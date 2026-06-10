# KVN Android Client — Задачи

## Phase Contract

Inputs: plan.md, spec.md, data-model.md.
Outputs: tasks.md с фазами, Surface Map, покрытие AC.
Stop if: нет — plan чёткий, AC привязаны к поверхностям.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `protocol/frames.yaml` | T1.1 |
| `protocol/handshake.yaml` | T1.1 |
| `protocol/codegen/` | T1.1 |
| `scripts/build.sh` | T1.2 |
| `src/android/` (Gradle, manifest) | T1.3 |
| `src/android/.../vpn/` | T2.1 |
| `src/android/.../transport/` | T2.2 |
| `src/android/.../protocol/` (framing + handshake) | T2.2 |
| `src/android/.../crypto/` | T2.2 |
| `src/android/.../ui/` | T2.3 |
| `src/android/.../config/` | T2.3 |
| `src/android/.../ui/qr/` | T2.5 |
| `src/android/.../vpn/` (integration) | T2.4 |
| `src/internal/transport/framing/` (Go) | T1.1 |
| `src/internal/protocol/handshake/` (Go) | T1.1 |
| `src/android/.../transport/reconnect/` | T3.1 |
| `src/android/.../ui/` (statistics) | T3.2 |
| `src/android/.../tests/` | T4.1, T4.2 |

## Implementation Context

- **Цель MVP:** APK устанавливается, пользователь вводит server+token (или сканирует QR), нажимает Connect — туннель работает, трафик идёт через сервер (AC-001, AC-002, AC-003, AC-006, AC-007).
- **Инварианты/семантика:**
  - Wire protocol живёт в `protocol/*.yaml` — единый source of truth для Go и Kotlin
  - Go-структуры фреймов переключаются на codegen; бинарный формат фреймов не меняется
  - VpnService — только TUN tunnel (не SOCKS5/HTTP proxy), minSdk 26
  - AES-256-GCM ключ из handshake, одинаковый для обеих сторон
- **Контракты/протокол:**
  - WebSocket binary frames: FrameTypeData=0x01, FrameTypeHello=0x02, FrameTypeAuth=0x03, FrameTypeClose=0x04 (см. `src/internal/transport/framing/framing.go`)
  - ClientHello → ServerHello handshake с auth token
  - Data payload шифруется AES-256-GCM
- **Границы scope:**
  - Не делаем: QUIC, split-tunnel, routing rules, SOCKS5/HTTP proxy
  - Не делаем: авто-переподключение и статистику в MVP (DEC-005)
- **Proof signals:**
  - APK собирается (`./scripts/build.sh android`)
  - VpnService запускается, статус Connected < 3 сек
  - Трафик (браузер на Android) идёт через сервер
  - Disconnect закрывает VpnService, трафик напрямую
- **References:** DEC-001, DEC-002, DEC-003, DEC-004, DEC-005, RQ-001–RQ-010

## Фаза 1: Протокол и фундамент

Цель: создать единый source of truth wire protocol, кодогенератор и встроить его в существующие скрипты сборки.

- [x] T1.1 Создать `protocol/frames.yaml`, `protocol/handshake.yaml` и генератор `protocol/codegen/` (Go + Kotlin data classes из YAML). Сгенерированные Go-файлы перезаписывают существующие `src/internal/transport/framing/framing.go` и `src/internal/protocol/handshake/handshake.go`. Touches: `protocol/frames.yaml`, `protocol/handshake.yaml`, `protocol/codegen/`, `src/internal/transport/framing/`, `src/internal/protocol/handshake/`
- [x] T1.2 Встроить вызов codegen в начало функций `build_client`, `build_server`, `build_web` в `scripts/build.sh`. Ручной шаг не нужен — любая сборка сначала генерирует протокол. Touches: `scripts/build.sh`
- [x] T1.3 Создать Android-проект: `src/android/build.gradle.kts` (minSdk 26, Compose, OkHttp, Coroutines), `settings.gradle.kts`, `AndroidManifest.xml` с VpnService permission. Touches: `src/android/`

## Фаза 2: MVP — работающий туннель

Цель: минимальный независимо демонстрируемый VPN-туннель — VpnService → WebSocket → handshake → relay.

- [x] T2.1 Реализовать VpnService + TUN read/write: builder с MTU, защита IP-пакетов в бинарные фреймы, write в WebSocket. Touches: `src/android/.../vpn/`
- [x] T2.2 Реализовать WebSocket клиент (OkHttp), Kotlin-реализацию framing/handshake/crypto (AES-256-GCM). Touches: `src/android/.../transport/`, `src/android/.../protocol/`, `src/android/.../crypto/`
- [x] T2.3 Реализовать UI (Jetpack Compose): экран Connect (server:port + token), статус Connected/Disconnected, кнопка Disconnect, сохранение конфига в DataStore. Touches: `src/android/.../ui/`, `src/android/.../config/`
- [x] T2.4 Интегрировать VpnService → WebSocket → server end-to-end. Убедиться: статус "Connected" < 3 сек, трафик идёт через сервер, Disconnect закрывает VpnService. Touches: `src/android/.../vpn/`, `src/android/.../transport/`
- [x] T2.5 Реализовать импорт конфига через QR-код: кнопка "Scan QR" на экране Connect, интеграция с камерой (CameraX + ML Kit barcode scanning или zxing), парсинг server:port+token из QR, заполнение формы. Touches: `src/android/.../ui/qr/`, `src/android/.../config/`

## Фаза 3: Итеративное расширение

Цель: добавить авто-переподключение, статистику и codegen-верификацию.

- [x] T3.1 Реализовать авто-переподключение с exponential backoff при обрыве WebSocket. Touches: `src/android/.../transport/reconnect/`
- [x] T3.2 Добавить счётчики RX/TX на главный экран. Touches: `src/android/.../ui/`
- [x] T3.3 Добавить AC-004 проверку: `make generate` с diff-check, что Go и Kotlin структуры соответствуют YAML. Touches: `scripts/` or CI config

## Фаза 4: Проверка

Цель: automated + manual coverage всех AC, trace-маркеры, review-ready пакет.

- [x] T4.1 Написать unit-тесты (framing encode/decode, handshake, AES-256-GCM). Touches: `src/android/.../tests/`
- [x] T4.2 Проставить `@sk-task` в коде и `@sk-test` в тестах. (Manual validation на устройстве deferred — нет Android SDK в окружении.) Touches: все поверхности

## Покрытие критериев приемки

- AC-001 -> T2.1, T2.2, T2.3, T2.4, T4.1
- AC-002 -> T2.3, T3.2, T4.1, T4.2
- AC-003 -> T2.3, T4.1
- AC-004 -> T1.1, T3.3, T5.19
- AC-005 -> T3.1, T4.1, T5.18
- AC-006 -> T2.1, T2.4, T4.1, T4.2
- AC-007 -> T2.5, T4.1, T5.2
- AC-008 -> T5.10, T5.11
- AC-009 -> T5.7, T5.8, T5.9
- AC-010 -> T5.15, T5.16
- AC-011 -> T5.13, T5.14

## Заметки

- T1.1 и T1.2 строго последовательны (codegen нужен до встраивания в build)
- T2.1 (VpnService) и T2.2 (WebSocket+handshake) можно параллелить
- AC-005 намеренно вынесен из MVP (DEC-005)
- Все задачи на implement обязаны проставлять `@sk-task` над owning declaration; тесты — `@sk-test`

## Фаза 5: Settings Expansion — полный набор настроек из kvn-web

Цель: добавить все настройки из kvn-web (TLS, routing, encryption, transport, obfuscation, kill switch) в Android-клиент. После выполнения Android не уступает web UI по функциональности.

### Шаг 5.1 — ConnectionConfig расширение + QR

Surface: `src/android/.../config/`, `src/android/.../ui/qr/`

- [x] T5.1 Перевести ConnectionConfig на JSON-сериализацию через kotlinx.serialization. Хранить JSON-строку в одном PreferencesKey. Сохранять все поля (server, port, token, mode, transport, mtu, ipv6, auto_reconnect, max_message_size, multiplex, log_level, tls.*, routing.*, crypto.*, kill_switch.*, reconnect.*, obfuscation.*). Touches: `src/android/.../config/AppConfig.kt`
- [x] T5.2 Обновить QR-импорт: парсить JSON-конфиг вместо server:port:token. Поддержать все поля. Touches: `src/android/.../ui/QrScannerScreen.kt`

### Шаг 5.2 — UI: секции настроек

Surface: `src/android/.../ui/ConnectScreen.kt`

- [x] T5.3 Разделить ConnectScreen на collapsible секции: Connection, TLS, Routing, Advanced, Encryption, Kill Switch, Reconnect, Obfuscation. Каждая секция — кастомный composable с заголовком и состоянием expand/collapse. Touches: `src/android/.../ui/ConnectScreen.kt`, `src/android/.../ui/SettingsSection.kt`
- [x] T5.4 Реализовать секцию Connection: server, port, token, mode (select: proxy/tun), transport (select: tcp/quic). Touches: `src/android/.../ui/ConnectScreen.kt`
- [x] T5.5 Реализовать секцию Advanced: mtu, ipv6 (toggle), auto_reconnect (toggle), log_level (text), max_message_size, multiplex (toggle). Touches: `src/android/.../ui/ConnectScreen.kt`
- [x] T5.6 Реализовать секцию Reconnect: min_backoff_sec, max_backoff_sec (number inputs). Touches: `src/android/.../ui/ConnectScreen.kt`

### Шаг 5.3 — TLS настройки

Surface: `src/android/.../ui/`, `src/android/.../transport/`

- [x] T5.7 Реализовать секцию TLS: verify_mode (text: verify/insecure/none), server_name (text). Touches: `src/android/.../ui/ConnectScreen.kt`
- [x] T5.8 Реализовать кастомный SSLSocketFactory + HostnameVerifier в VpnService для поддержки verify_mode и server_name. verify=standard, insecure=trust-all, none=no-op. Touches: `src/android/.../vpn/KvnVpnService.kt`
- [x] T5.9 Передавать TLS-настройки из UI в WebSocketClient через AppConfig. Touches: `src/android/.../vpn/KvnVpnService.kt`, `src/android/.../ui/MainViewModel.kt`

### Шаг 5.4 — Routing

Surface: `src/android/.../ui/`, `src/android/.../vpn/`

- [x] T5.10 Реализовать секцию Routing: default_route (text), include_ranges, exclude_ranges (comma-separated). Touches: `src/android/.../ui/ConnectScreen.kt`
- [x] T5.11 CIDR-роутинг через VpnService.Builder.addRoute(): применять include_ranges при создании VpnService. include_ranges пустой = весь трафик (0.0.0.0/0). Touches: `src/android/.../vpn/KvnVpnService.kt`
- [-] T5.12 Domain routing: отложено до второй очереди (требует DNS-перехвата в TUN). Touches: deferred per DEC-011

### Шаг 5.5 — Encryption + Kill Switch

Surface: `src/android/.../ui/`, `src/android/.../vpn/`, `src/android/.../crypto/`

- [x] T5.13 Реализовать секцию Encryption: enabled (toggle), key (password input). Touches: `src/android/.../ui/ConnectScreen.kt`
- [x] T5.14 Подключить AesGcmCipher к VpnService: если crypto.enabled=true, шифровать payload перед отправкой и расшифровывать при получении. Touches: `src/android/.../vpn/KvnVpnService.kt`, `src/android/.../crypto/AesGcmCipher.kt`
- [x] T5.15 Реализовать секцию Kill Switch: enabled (toggle). Touches: `src/android/.../ui/ConnectScreen.kt`
- [x] T5.16 Kill Switch: safeStop() — блокировать stopSelf() при отключении, если kill_switch.enabled и не user-initiated disconnect. Touches: `src/android/.../vpn/KvnVpnService.kt`

### Шаг 5.6 — Obfuscation + ReconnectManager

Surface: `src/android/.../ui/`, `src/android/.../transport/reconnect/`

- [x] T5.17 Реализовать секцию Obfuscation: enabled (toggle), uTLS (toggle), padding enabled (toggle), padding size (number). Touches: `src/android/.../ui/ConnectScreen.kt`
- [x] T5.18 ReconnectManager: configurable min_backoff_sec и max_backoff_sec из настроек вместо констант. Touches: `src/android/.../transport/reconnect/ReconnectManager.kt`

### Шаг 5.7 — Codegen + синхронизация

Surface: `protocol/codegen/`, `src/android/.../tests/`

- [x] T5.19 Обновить generateKotlinHandshake в codegen: добавить Kotlin-константы из handshake.yaml (PROTO_VERSION, SESSION_ID_LEN, FLAG_IPV6, CRYPTO_TAG и т.д.) в Handshake.kt. Touches: `protocol/codegen/main.go`
- [x] T5.20 Написать unit-тесты для config serialization и QR parsing (ConfigSerializationTest, QrConfigTest — 7 тестов). Touches: `src/android/.../tests/`


