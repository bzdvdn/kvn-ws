# KVN Android Client — v2 (Settings Expansion)

## Scope Snapshot

- In scope: нативный Android-клиент на Kotlin под существующий kvn-ws сервер. Режим TUN (VPN) через VpnService, управление через UI (Jetpack Compose), подключение по WebSocket (с перспективой QUIC). Единый source of truth для wire protocol — `protocol/` с генерацией Go + Kotlin структур. Полный набор настроек, идентичный kvn-web: TLS, routing, encryption, transport selection, obfuscation, kill switch.
- Out of scope: iOS, Desktop GUI, серверная админка, режим SOCKS5/HTTP proxy в Android.

## Цель

Пользователь Android устанавливает APK, вводит server address + token, нажимает "Connect" — и получает рабочий VPN-туннель через kvn-ws сервер с тем же уровнем контроля, что и в kvn-web. Весь трафик идёт через WebSocket-соединение с бинарным фреймингом, маскируясь под обычный HTTPS. Успех: пользователь видит статус "Connected", счётчики трафика, может отключиться и настроить TLS, routing, encryption, transport.

## Основной сценарий

1. Пользователь устанавливает APK и открывает приложение.
2. На первом экране — форма с секциями: Connection, TLS, Routing, Advanced, Encryption, Kill Switch.
3. Пользователь настраивает параметры и нажимает "Connect" — система запрашивает разрешение VPN (VpnService).
4. После подтверждения — устанавливается WebSocket-соединение с сервером (OkHttp с TLS-настройками), ClientHello → ServerHello, получен IP из пула.
5. VpnService читает TUN, упаковывает IP-пакеты в бинарные фреймы, отправляет на сервер, и наоборот.
6. На экране — статус "Connected", счётчики RX/TX, кнопка "Disconnect".
7. При повторном открытии все поля заполнены из сохранённого конфига.

## User Stories

- P1 Story: как пользователь Android, я хочу ввести server и token и нажать "Connect", чтобы получить рабочий VPN.
- P2 Story: как пользователь, я хочу видеть статус подключения и статистику трафика, чтобы понимать, что туннель работает.
- P3 Story: как пользователь, я хочу чтобы приложение запоминало конфиг между запусками.
- P4 Story: как разработчик, я хочу единый source of truth для wire protocol, чтобы Go и Kotlin структуры были синхронизированы.
- P5 Story: как продвинутый пользователь, я хочу настраивать TLS (verify/insecure/none, SNI), чтобы обходить DPI и подключаться к серверам с самоподписанными сертификатами.
- P6 Story: как продвинутый пользователь, я хочу настраивать routing (CIDR include/exclude, домены), чтобы контролировать, какой трафик идёт через VPN.
- P7 Story: как пользователь, я хочу выбирать transport (tcp/quic) для подключения.
- P8 Story: как пользователь, я хочу включать AES-256-GCM шифрование данных с настраиваемым ключом.
- P9 Story: как пользователь, я хочу Kill Switch, чтобы трафик блокировался при отключении VPN.

## Scope

- `protocol/` — формальное описание wire protocol (frames, handshake)
- `protocol/codegen/` — генератор Go + Kotlin структур из YAML
- `src/android/` — Kotlin проект:
  - VpnService с TUN read/write
  - WebSocket client (OkHttp) с бинарным фреймингом
  - Handshake + Auth протокол
  - AES-256-GCM расшифровка/зашифровка
  - UI (Jetpack Compose): экран подключения с секциями
  - Сохранение конфига в DataStore
  - Настройки: TLS (verify_mode, server_name, sni), routing (default_route, include/exclude CIDR, IPs, domains), transport (tcp/quic), encryption (enabled/key), obfuscation (enabled/padding), kill switch, reconnect backoff, log level
- Импорт конфига через QR-код (полный конфиг, не только server:port + token)
- Поддержка IPv4 (IPv6 опционально)

## Контекст

- Сервер уже работает, протокол стабилен (v0.3.0)
- kvn-web содержит полный набор настроек — Android должен быть на том же уровне
- Каждой настройке в kvn-web соответствует поле в конфиге сервера
- Большинство настроек (TLS, routing, encryption) — клиентские, не требуют изменений протокола

## Требования

- RQ-001 Минимальная Android API: 26 (Android 8.0) — WebSocket API и VpnService доступны.
- RQ-002 Wire protocol — единое описание в `protocol/`, Go и Kotlin код генерируется из YAML.
- RQ-003 Форма ввода с секциями: Connection (server, port, token, mode, transport), TLS (verify_mode, server_name, sni), Routing (default_route, CIDR, IPs, domains), Advanced (mtu, ipv6, auto_reconnect, log_level, multiplex, max_message_size), Encryption (enabled, key), Kill Switch (enabled), Reconnect (min/max backoff), Obfuscation (enabled, uTLS, padding).
- RQ-004 Connect/Disconnect, индикатор статуса, счётчики RX/TX.
- RQ-005 Конфиг (все поля) сохраняется в DataStore, при повторном открытии форма заполняется.
- RQ-006 VpnService — с поддержкой routing (include/exclude CIDR, IPs), kill switch.
- RQ-007 WebSocket transport с бинарными фреймами (FrameTypeData = 0x01) + TLS-настройки OkHttp.
- RQ-008 Поддержка ClientHello → ServerHello handshake, включая auth token + transport selection.
- RQ-009 AES-256-GCM encrypt/decrypt для data payload (ключ из настроек, не только из handshake).
- RQ-010 Авто-переподключение при обрыве соединения (configurable exponential backoff).
- RQ-011 Импорт конфигурации через QR-код — полный JSON-конфиг, а не только server:port + token.
- RQ-012 Kill Switch — блокировка всего трафика при отключении VpnService (firewall rule).
- RQ-013 Routing — CIDR-based include/exclude, IP-based include/exclude, domain-based include/exclude.

## Вне scope

- QUIC transport (вторая очередь)
- SOCKS5/HTTP proxy mode
- iOS клиент
- Server-side dashboard
- Multi-hop / цепочки серверов
- Widgets / Quick Settings tile

## Критерии приемки

### AC-001 Форма и подключение

- Почему это важно: пользователь получает работающий туннель за <3 секунды.
- **Given** пользователь открыл приложение и заполнил все секции настроек
- **When** он нажимает "Connect" и подтверждает VPN-запрос системы
- **Then** статус меняется на "Connected" в течение 3 секунд, трафик идёт через сервер с учётом routing-правил

### AC-002 Статус и статистика

- Почему это важно: пользователь видит, что туннель работает.
- **Given** приложение подключено к серверу
- **When** пользователь смотрит на главный экран
- **Then** отображается статус (Connected), счётчики RX/TX, кнопка Disconnect

### AC-003 Сохранение конфига

- Почему это важно: пользователь не вводит данные каждый раз.
- **Given** пользователь сохранил конфиг и закрыл приложение
- **When** он открывает приложение снова
- **Then** все поля формы заполнены последними значениями

### AC-004 Генерация из protocol/

- Почему это важно: Go и Kotlin структуры синхронизированы всегда.
- **Given** изменения в `protocol/frames.yaml` или `protocol/handshake.yaml`
- **When** запущен `make generate`
- **Then** Go фреймы в `src/internal/transport/framing/` и Kotlin data-классы в `src/android/` обновлены

### AC-005 Авто-переподключение

- Почему это важно: пользователь не теряет VPN при временных сбоях сети.
- **Given** приложение было подключено и соединение оборвалось
- **When** сеть восстанавливается
- **Then** приложение автоматически переподключается с configurable exponential backoff, статус меняется на Connected

### AC-006 VpnService корректность

- Почему это важно: стандартное поведение VPN на Android.
- **Given** приложение не подключено
- **When** пользователь нажимает Connect
- **Then** система показывает стандартный VPN-запрос
- **Given** приложение подключено
- **When** пользователь нажимает Disconnect (или выключает VPN в системных настройках)
- **Then** VpnService закрыт, трафик идёт напрямую

### AC-007 Импорт через QR

- Почему это важно: пользователь быстро переносит конфиг с другого устройства или из kvn-web.
- **Given** на экране подключения нет заполненных полей
- **When** пользователь нажимает "Scan QR" и сканирует QR-код с полным JSON-конфигом
- **Then** все поля формы заполняются автоматически

### AC-008 Routing

- Почему это важно: пользователь контролирует, какой трафик идёт через VPN.
- **Given** пользователь настроил include_ranges/exclude_ranges
- **When** VpnService запущен
- **Then** только указанные диапазоны маршрутизируются через туннель

### AC-009 TLS

- Почему это важно: пользователь подключается к серверам с разными TLS-настройками.
- **Given** пользователь выбрал verify_mode=insecure
- **When** WebSocket подключается к серверу с самоподписанным сертификатом
- **Then** соединение устанавливается успешно

### AC-010 Kill Switch

- Почему это важно: пользователь защищён от утечек при обрыве.
- **Given** kill_switch.enabled=true
- **When** VpnService отключается
- **Then** весь сетевой трафик блокируется до переподключения

### AC-011 Encryption

- Почему это важно: пользователь шифрует данные поверх TLS.
- **Given** crypto.enabled=true и crypto.key задан
- **When** данные отправляются через WebSocket
- **Then** payload зашифрован AES-256-GCM

## Допущения

- Сервер уже работает и поддерживает текущий wire protocol (breaking-изменения не вносятся).
- Android API 26+ (Android 8.0) — доступен VpnService и OkHttp WebSocket.
- У команды/разработчика есть устройство для тестирования (эмулятор или физический Android).
- OkHttp — основная HTTP/WebSocket библиотека (стандарт де-факто для Android).
- Google Play Services не обязательны.
- Настройки TLS, routing, encryption — клиентские, сервер не требует изменений.
- QR-код содержит полный JSON-конфиг (не только server:port:token).

## Открытые вопросы

- QUIC transport на Android — отложено до появления стабильной Kotlin-библиотеки.
- Domain-based routing на Android — требует DNS-резолвера внутри VpnService.
- UI-язык: пока английский.

## Принятые решения

- **Протокол:** чистый Kotlin (без gomobile) — полная реализация framing, handshake, crypto на Kotlin.
- **Распространение:** только APK (sideload / ADB). Google Play не целевой.
- **CI:** GitHub Actions (GitHub-hosted runners) — сборка + lint + apk artifact.
- **TLS:** OkHttp HostnameVerifier + SSLSocketFactory для кастомизации.
- **Routing:** Android VpnService.Builder.addRoute() для CIDR, системный firewall для kill switch.
- **Конфиг:** DataStore с JSON-сериализацией всего конфига в одно поле (вместо десятков PreferencesKey).
