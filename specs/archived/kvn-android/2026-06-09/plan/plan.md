# KVN Android Client — План

## Phase Contract

Inputs: spec.md, конституция.
Outputs: plan.md, data-model.md.
Stop if: нет — spec стабильна, AC чёткие.

## Цель

Реализовать нативный Android-клиент (Kotlin, Jetpack Compose) для существующего kvn-ws сервера. Работа: TUN via VpnService → WebSocket transport → handshake + auth → relay IP-трафика. Параллельно формализовать wire protocol в `protocol/` с codegen Go↔Kotlin.

## MVP Slice

- wire protocol YAML-описание + codegen (Go/Kotlin)
- VpnService + TUN read/write
- WebSocket клиент (OkHttp) с бинарным фреймингом
- Handshake + ClientHello/ServerHello + auth token
- AES-256-GCM encrypt/decrypt
- UI: экран Connect (server:port + token), статус Connected/Disconnected, Disconnect
- Сохранение конфига в DataStore
- AC покрытые MVP: AC-001, AC-002, AC-003, AC-006, AC-007

## First Validation Path

1. Собрать APK: `make android`
2. Установить на эмулятор API 26+ (или физическое устройство)
3. Открыть приложение, ввести адрес работающего kvn-ws сервера + token
4. Нажать Connect → подтвердить VPN-запрос системы
5. Убедиться: статус "Connected" <3 сек, открыть браузер — сайты идут через сервер
6. Нажать Disconnect — VpnService закрыт, трафик напрямую

## Scope

- `protocol/` — YAML-описание wire protocol (frames, handshake)
- `protocol/codegen/` — Go + Kotlin генератор структур из YAML
- `src/android/` — Kotlin проект (VpnService, WebSocket, UI, DataStore, QR scanner)
- Поддержка IPv4, WebSocket transport
- Генерация Go-структур в `src/internal/transport/framing/` из `protocol/`
- Вне scope: QUIC, split-tunnel, routing rules, iOS, системный VPN tile

## Implementation Surfaces

- **`protocol/frames.yaml`** — новая; YAML-спецификация FrameType, флаги, layout
- **`protocol/handshake.yaml`** — новая; YAML-спецификация ClientHello/ServerHello
- **`protocol/codegen/`** — новая; шаблоны + генератор (Go + Kotlin data classes)
- **`src/android/`** — новая; весь Android-проект (Gradle, Kotlin, Compose, QR scanner)
- **`src/internal/transport/framing/`** — существующая; будет переключена на codegen-вывод
- **`src/internal/protocol/handshake/`** — существующая; будет переключена на codegen-вывод

## Bootstrapping Surfaces

- `protocol/codegen/` — кодогенератор и шаблоны должны быть написаны первыми
- `src/android/build.gradle.kts` — Gradle-конфигурация (minSdk 26, Compose, OkHttp)
- `src/android/app/src/main/AndroidManifest.xml` — VpnService permission + declaration

## Влияние на архитектуру

- **Локальное:** новая кодовая база `src/android/` не пересекается с существующим Go-кодом
- **Интеграции:** Go-структуры фреймов переключаются с ручного определения на кодогенерацию; обратная совместимость гарантируется идентичным YAML-описанием
- **Rollout:** codegen-миграция Go-структур делается коммитом в той же feature-ветке, без intermediate breakage

## Acceptance Approach

- **AC-001 (Форма + подключение):** UI тест (Compose Test): ввод полей → нажатие Connect → проверка статуса Connected. Surface: UI, VpnService, WebSocket.
- **AC-002 (Статус + статистика):** UI тест: после Connected проверка отображения статуса и счётчиков. Surface: UI, статистика из TUN counters.
- **AC-003 (Сохранение конфига):** Unit/instrumented: запись в DataStore → restart activity → форма заполнена. Surface: DataStore, UI.
- **AC-004 (Генерация из protocol/):** `make generate` → diff Go + Kotlin файлы — нет изменений (YAML == codegen output). Surface: protocol/, codegen.
- **AC-005 (Авто-переподключение):** Интеграционный: WebSocket disconnect → verify retry → reconnect. Surface: WebSocket client, reconnect logic.
- **AC-006 (VpnService корректность):** Instrumented: Connect → VpnService запущен; Disconnect → VpnService остановлен. Surface: VpnService.
- **AC-007 (QR импорт):** UI тест: нажать "Scan QR" → передать mock-данные → поля заполнены. Surface: UI, QR scanner (zxing/camerax).

## Данные и контракты

- Data model не меняется: фича не вводит новых persisted entities или state transitions на сервере.
- Локальные контракты: протокол фреймов остаётся идентичным — codegen только пересобирает уже существующие Go-структуры.
- См. `data-model.md` (no-change).

## Стратегия реализации

### DEC-001 Codegen-first Protocol Sync
- **Why:** Единый source of truth для wire protocol между Go и Kotlin; ручная синхронизация структур ненадёжна.
- **Tradeoff:** Дополнительная сложность codegen-инфраструктуры; первый коммит не покажет работающий туннель.
- **Affects:** `protocol/`, `protocol/codegen/`, `src/internal/transport/framing/`, `src/internal/protocol/handshake/`
- **Validation:** `make generate` из чистого репозитория — Go и Kotlin файлы обновлены без ошибок.

### DEC-002 Kotlin-native реалізация протокола (без gomobile)
- **Why:** Полный контроль над стеком, без накладных расходов gomobile-bridge.
- **Tradeoff:** Дублирование реализации handshake, framing, crypto на Kotlin.
- **Affects:** `src/android/app/src/main/kotlin/.../protocol/`
- **Validation:** Тест: Kotlin-клиент устанавливает соединение и handshake с kvn-ws сервером.

### DEC-003 OkHttp WebSocket (базовый транспорт), QUIC deferred
- **Why:** OkHttp — стандарт де-факто для Android, зрелый, стабильный.
- **Tradeoff:** Отсутствие QUIC до второй очереди; performance overhead WebSocket vs QUIC.
- **Affects:** `src/android/` — WebSocket client
- **Validation:** Соединение с сервером и передача бинарных фреймов.

### DEC-004 DataStore вместо SharedPreferences
- **Why:** DataStore — современная замена SharedPreferences с корутинами и type safety.
- **Tradeoff:** Дополнительная зависимость + coroutine scope.
- **Affects:** `src/android/app/src/main/kotlin/.../config/`
- **Validation:** Сохранение → закрытие → открытие → данные восстановлены.

### DEC-005 MVP без авто-переподключения
- **Why:** Сокращение первого инкремента; retry logic — изолируемый второй шаг.
- **Tradeoff:** AC-005 не проходит в MVP; пользователь теряет VPN при обрыве.
- **Affects:** Пропущено в MVP — добавляется в итеративном расширении.
- **Validation:** AC-005 верифицируется только после второй итерации.

## Incremental Delivery

### MVP (Первая ценность)

Задачи: protocol/ codegen, VpnService, WebSocket transport, handshake, AES-256-GCM, UI (Connect/Disconnect + статус), DataStore config.
AC: AC-001, AC-002, AC-003, AC-006, AC-007.
Validation: APK → Connect → сайты открываются → Disconnect.

### Итеративное расширение

- **Шаг 2 — Авто-переподключение:** exponential backoff, reconnect logic. AC-005.
- **Шаг 3 — Статистика RX/TX:** счётчики на главном экране. AC-002 (расширение).
- **Шаг 4 — AC-004 верификация:** codegen integration test в CI.

## Порядок реализации

1. `protocol/` + codegen (`DEC-001`) — база, от которой зависят Go и Kotlin
2. `src/android/` bootstrapping (Gradle, manifest, deps)
3. VpnService + TUN read/write (независимо от UI)
4. WebSocket client + handshake + crypto (параллельно с VpnService)
5. UI (Jetpack Compose) — экран Connect/Disconnect, статус
6. DataStore config persistence
7. Интеграция: VpnService → WebSocket → server → обратно
8. Авто-переподключение (шаг 2)
9. Статистика RX/TX (шаг 3)

Параллельно: 3 и 4 можно делать одновременно.

## Фаза 5: Settings Expansion — Полный набор настроек из kvn-web

После MVP и базового расширения (reconnect, stats) добавляем все настройки, которые есть в kvn-web.

### DEC-006 DataStore JSON Config
- **Why:** Хранить все настройки (38 полей) как JSON-строку в одном PreferencesKey — проще, чем 38 отдельных ключей.
- **Tradeoff:** Потеря type-safety на уровне Preferences; компенсируется Kotlinx Serialization.
- **Validation:** Сохранение → restart → все поля восстановлены.

### DEC-007 UI: Collapsible Sections
- **Why:** 38 полей не влезают на один экран. Collapsible секции как в kvn-web.
- **Tradeoff:** Дополнительная сложность UI; пользователь видит не все поля сразу.
- **Validation:** Каждая секция сворачивается/разворачивается.

### DEC-008 TLS: OkHttp Custom SSLSocketFactory
- **Why:** OkHttp позволяет подменить SSLSocketFactory и HostnameVerifier для кастомного TLS.
- **Tradeoff:** Нужно отключать проверку цепочек для режима insecure.
- **Validation:** Подключение к серверу с самоподписанным сертификатом в режиме insecure.

### DEC-009 Routing: VpnService.Builder CIDR + Kill Switch
- **Why:** VpnService.Builder.addRoute() позволяет указать, какие подсети маршрутизировать через TUN. Kill Switch реализуется через блокирующие правила.
- **Tradeoff:** Domain-роутинг требует DNS-перехвата в TUN.
- **Validation:** После Connect только указанные диапазоны идут через туннель.

### Incremental Delivery — Фаза 5

- **Шаг 5.1 — ConnectionConfig расширение:**
  - DataStore → JSON-конфиг со всеми полями (Gson/kotlinx.serialization)
  - QR-импорт полного JSON-конфига
  
- **Шаг 5.2 — TLS настройки:**
  - UI: verify_mode (verify/insecure/none), server_name (SNI override), sni (chip list)
  - OkHttp: кастомный SSLSocketFactory + HostnameVerifier
  
- **Шаг 5.3 — Routing:**
  - UI: default_route (server/direct), include_ranges, exclude_ranges, include_ips, exclude_ips, include_domains, exclude_domains
  - VpnService: CIDR-based routing через Builder.addRoute()
  - Domain routing: DNS-перехват в TUN (опционально)
  
- **Шаг 5.4 — Encryption + Kill Switch:**
  - UI: crypto.enabled, crypto.key
  - UI: kill_switch.enabled
  - VpnService: AES-256-GCM шифрование payload (уже есть AesGcmCipher)
  - VpnService: блокировка трафика при отключении
  
- **Шаг 5.5 — Остальные настройки:**
  - UI: transport (tcp/quic), auto_reconnect, reconnect min/max backoff, log_level, max_message_size, multiplex, obfuscation (enabled, uTLS, padding)
  - WebSocket: OkHttp-настройки под транспорт
  - ReconnectManager: configurable backoff (min/max)

## Риски

- **Риск 1:** Несоответствие протокола — Kotlin-реализация framing/handshake может разойтись с Go.
  - Mitigation: codegen из единого YAML; integration test против реального сервера.
- **Риск 2:** VpnService сложности на разных Android API / vendor-прошивках.
  - Mitigation: minSdk 26 (Android 8.0); тестирование на эмуляторе + физическом устройстве.
- **Риск 3:** Производительность TUN read/write в Kotlin.
  - Mitigation: OkHttp binary frames с direct ByteBuffer; профилирование при необходимости.
- **Риск 4:** Domain-роутинг сложен на Android — требует перехвата DNS-запросов в TUN.
  - Mitigation: domain routing отложен, сначала CIDR-based routing через VpnService.Builder.

## Rollout и compatibility

- Специальных rollout-действий не требуется: новая кодовая база не влияет на существующий сервер или Go-клиенты.
- Go-фреймы переключаются на codegen в том же PR — обратная совместимость гарантируется идентичным YAML.
- Codegen вшит в `scripts/build.sh` (вызов `scripts/generate-protocol.sh` в начале функций `build_client`, `build_server`, `build_web`) — ручной шаг не нужен, все три таргета автоматически получают актуальные структуры.
- CI: сборка APK, lint, unit tests, integration test handshake с dev-сервером.
- Конфиг обратно совместим: старый формат (server+port+token) читается и мигрируется в новый JSON.

## Проверка

- **Unit:** handshake encode/decode, frame encryption/decrypt, DataStore read/write
- **Compose UI:** статус после Connect/Disconnect, заполнение формы из DataStore
- **Integration:** WebSocket → handshake → бинарный фрейм → echo test (против сервера)
- **Manual:** APK установка → реальное VPN-соединение → сайты открываются
- AC-001–AC-011 покрываются комбинацией unit + integration + manual
- DEC-001–DEC-009: codegen diff-check, integration handshake test, DataStore roundtrip, connectivity test, TLS verify modes

## Соответствие конституции

- Нет конфликтов: Go-only не нарушается (Android — отдельная кодовая база на Kotlin)
- DDD/Clean Architecture: Android-код следует той же слоистой структуре (domain → infrastructure → presentation)
- Traceability: `@sk-task` и `@sk-test` обязательны
- Docker multi-stage: для Android сборка в CI (GitHub Actions) с отдельным workflow
