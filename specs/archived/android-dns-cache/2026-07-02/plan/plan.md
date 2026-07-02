# DNS Cache + Exclude Domains + WS: EOF Reconnect — План реализации

## Phase Contract

Inputs: specs/active/android-dns-cache/spec.md, specs/active/android-dns-cache/inspect.md
Outputs: plan.md, data-model.md
Stop if: нет.

## Цель

Реализовать на Android-клиенте три связанных улучшения в инкрементальном порядке:

1. **WS: EOF fix** — исправление реконнекта (closeTun + сброс tunReaderStarted)
2. **DNS cache** — TTL-кэш domain→IP + IP→domain трекер, перехват UDP/53 в tunReader
3. **Exclude domains** — pre-resolve при старте, route exclusion, прямой DNS
4. **Config + UI** — dnsCacheEnabled toggle, проброс через QR/JSON

Работа изолирована в Android-модуле (`src/android/`) + минимальное расширение Go `ClientConfig` для kvn-web.

## MVP Slice

**Phase 1 (WS: EOF fix)** — минимальный independently shippable инкремент:
- AC-003 (WS: EOF reconnect with TUN recreation)
- AC-005 (no data loss during reconnect)
- Без конфигов, без нового UI

**Phase 2 добавляет:** AC-001 (cache hit), AC-002 (cache miss), AC-004 (TTL)
**Phase 3 добавляет:** AC-006 (exclude pre-resolve), AC-007 (direct DNS)
**Phase 4 добавляет:** AC-008 (toggle), AC-009 (config propagation)

## First Validation Path

1. Phase 1: Запустить VPN, kill WS на сервере (`kill <ws-pid>` или закрыть порт) → проверить авто-восстановление (`ping 8.8.8.8` не прерывается > 3s)
2. Phase 2: `dig example.com` ×2 — первый miss, второй hit (лог `CACHE HIT`)
3. Phase 3: `curl https://ozon.ru` во время VPN — tcpdump на сервере не показывает пакетов к ozon IP
4. Phase 4: toggle → выкл → поведение как до фичи

## Scope

- **Новый модуль** `src/android/.../dns/` — DnsCache.kt, DnsTracker.kt, DnsResolver.kt
- **Изменение** `src/android/.../vpn/KvnVpnService.kt` — WS:EOF fix, DNS intercept, exclude pre-resolve
- **Изменение** `src/android/.../config/AppConfig.kt` — `dnsCacheEnabled` поле
- **Изменение** `src/android/.../ui/ConnectScreen.kt` — DNS cache toggle
- **Изменение** `src/android/.../ui/QrScannerScreen.kt` — WebConfig JSON модели + mapping
- **Изменение** `src/internal/config/client.go` — `dns_cache_enabled` в web JSON (уже есть `DNSCacheCfg`, нужно убедиться что пробрасывается)
- **Изменение** `src/android/.../ui/MainViewModel.kt` — сохранение toggle

## Performance Budget

- DNS cache lookup: < 1ms p99 (in-memory map)
- DNS cache memory: < 256 KB (1024 entry cap, LRU eviction)
- Reconnect time: ≤ 10s p95 (SC-001)
- Cache hit ratio: ≥ 60% after 5 min (SC-002)

## Implementation Surfaces

### Surfaces, которые должны быть созданы

| Surface | Почему новая | Роль |
|---|---|---|
| `src/android/.../dns/DnsCache.kt` | Нет DNS-кэша на Android | TTL-кэш domain→IP с LRU-эвикцией |
| `src/android/.../dns/DnsTracker.kt` | Нет reverse-map IP→domain | Отслеживание resolved IP для exclude |
| `src/android/.../dns/DnsResolver.kt` | Нет прямого DNS через реальную сеть | `protect()` + `DatagramSocket` для excluded DNS |

### Surfaces, которые будут изменены

| Surface | Что меняется | AC |
|---|---|---|
| `KvnVpnService.kt` | WS:EOF fix + DNS intercept + exclude pre-resolve | AC-001–AC-009 |
| `AppConfig.kt` | `dnsCacheEnabled: Boolean = false` | AC-008 |
| `ConnectScreen.kt` | DNS cache toggle в UI, routing exclude domains уже есть | AC-008 |
| `QrScannerScreen.kt` | `dns_cache_enabled` в WebConfig JSON + mapping | AC-009 |
| `MainViewModel.kt` | Сохранение dnsCacheEnabled в конфиг | AC-008 |
| `client.go` (Go) | `dns_cache_enabled` уже есть в `DNSCacheCfg`; убедиться что пробрасывается в QR JSON | AC-009 |

## Bootstrapping Surfaces

- `src/android/.../dns/` — создать директорию + 3 файла: DnsCache.kt, DnsTracker.kt, DnsResolver.kt
- `src/android/.../dns/DnsParser.kt` — DNS wire-format парсер (если не вмещается в DnsCache/DnsTracker)

## Влияние на архитектуру

- **Новый слой**: `com.kvn.client.dns` — изолированный пакет без зависимостей от Android SDK (чистый Kotlin + java.nio.ByteBuffer)
- **KvnVpnService становится толще**: добавляется DNS-диспетчеризация в `tunReader()`. Для управляемости вынести логику DNS-перехвата в отдельный класс `DnsInterceptor` (внутри пакета `dns`)
- **ConnectionConfig расширяется**: одно новое поле, обратно совместимо (дефолт `false`)
- **QR/JSON**: два формата сериализации (нативный Android + web-совместимый) — нужно поддерживать оба

## Acceptance Approach

### Phase 1: WS: EOF Fix

- **AC-003**: `onConnectionStateChange` при `DISCONNECTED` → сброс `tunReaderStarted = false`, закрытие TUN fd → `ReconnectManager.start()` → новый handshake → `establishTun()` + `tunReader()`. Проверка: manual kill WS → `curl` восстанавливается < 10s.
- **AC-005**: TUN fd не закрывается при EOF, только при DISCONNECTED; `tunReader()` не бросает исключение. Проверка: unit test с mock transport.

Затрагиваемые surfaces: `KvnVpnService.kt`

### Phase 2: DNS Cache

- **AC-001**: DnsCache содержит запись → `tunReader()` возвращает ответ без `FRAME_TYPE_DNS`. Проверка: unit test DnsCache + integration test tunReader.
- **AC-002**: DnsCache пуст → forward как `FRAME_TYPE_DNS` → `handleFrame()` парсит ответ → сохраняет в кэш + трекер. Проверка: integration test.
- **AC-004**: TTL истёк → entry удалён, поведение как miss. Проверка: unit test DnsCache expiry.

Затрагиваемые surfaces: `dns/DnsCache.kt`, `dns/DnsTracker.kt`, `KvnVpnService.kt`

### Phase 3: Exclude Domains

- **AC-006**: `resolveExcludedDomains()` → `InetAddress.getAllByName()` → /32 exclude routes → `Builder.addRoute()` до establish(). Проверка: конфиг с ozon.ru → `curl` не создаёт FRAME_TYPE_DATA (tcpdump на сервере).
- **AC-007**: DNS для excluded → `tunReader()` определяет QNAME в excludeDomains → `DnsResolver.resolve()` через `protect()` → ответ в TUN output. Проверка: лог `"DNS exclude DIRECT ozon.ru"`.

Затрагиваемые surfaces: `dns/DnsResolver.kt`, `KvnVpnService.kt`

### Phase 4: Config + Toggle

- **AC-008**: `dnsCacheEnabled=false` → DnsCache/DnsTracker no-op, tunReader не intercept-ит UDP/53. Проверка: toggle off → поведение идентично текущей версии.
- **AC-009**: QR/JSON содержит `dns_cache_enabled` → `ConnectionConfig.deserialize()` возвращает `dnsCacheEnabled=true`. Проверка: unit test десериализации.

Затрагиваемые surfaces: `AppConfig.kt`, `ConnectScreen.kt`, `QrScannerScreen.kt`, `MainViewModel.kt`, `client.go`

## Данные и контракты

См. `data-model.md`.

Расширения:
- `ConnectionConfig.dnsCacheEnabled: Boolean = false` (Android, @Serializable)
- `WebRoutingCfg.dns_cache_enabled: Boolean? = null` (QR web JSON)
- `RoutingCfg.DNSCache.Enabled` уже существует (Go)

Контракты не меняются: `FRAME_TYPE_DNS` wire format остаётся тем же, server-side не меняется.

## Стратегия реализации

### DEC-001 TUN-level DNS intercept vs standalone DNS proxy

- **Why**: DNS over UDP/53 уже проходит через TUN — intercept на уровне `tunReader()` не требует отдельного UDP-сокета и лишнего копирования. Standalone DNS proxy (как в Go `dnsproxy`) требует listen на 127.0.0.x и может конфликтовать с другими DNS-серверами на устройстве.
- **Tradeoff**: Не перехватывает DoH/DoT (это не в scope). Для чистоты кода DNS-логика выносится в отдельный класс `DnsInterceptor`, а не in-line в `tunReader()`.
- **Affects**: `dns/DnsInterceptor.kt` (новый), `KvnVpnService.kt`
- **Validation**: UDP/53 пакеты не доходят до сервера при cache hit.

### DEC-002 Pre-resolve excluded domains vs lazy resolve

- **Why**: Android VpnService routes нельзя менять после `establish()`. Единственный способ исключить IP на уровне ядра — добавить `/32` routes ДО вызова `establish()`. Lazy resolve + data-plane routing требует userspace-перехвата TCP, что сложно и ненадёжно.
- **Tradeoff**: При смене IP excluded домена новый IP пойдёт через туннель до reconnect. Acceptable для MVP.
- **Affects**: `KvnVpnService.kt`
- **Validation**: После `establish()` трафик к pre-resolved IP не создаёт FRAME_TYPE_DATA.

### DEC-003 Extend existing QR JSON vs new serialization

- **Why**: Android уже имеет два формата: (1) нативный `ConnectionConfig` JSON через `kotlinx.serialization`, (2) web-совместимый JSON в `QrScannerScreen.kt`. `routing_exclude_domains` уже есть в `WebRoutingCfg`. Добавить `dns_cache_enabled` в `WebRoutingCfg` — минимальное изменение.
- **Tradeoff**: Поддержка двух форматов — небольшой maintenance overhead.
- **Affects**: `QrScannerScreen.kt` (`WebRoutingCfg`), `AppConfig.kt` (`ConnectionConfig`)
- **Validation**: QR export → scan → config содержит dnsCacheEnabled.

## Incremental Delivery

### MVP (Phase 1 — WS: EOF Fix)

- Что: `onConnectionStateChange` reset `tunReaderStarted` + close TUN on DISCONNECTED
- AC: AC-003, AC-005
- Проверка: kill WS → `curl` восстанавливается

### Phase 2 — DNS Cache

- Что: DnsCache + DnsTracker + DnsInterceptor + перехват UDP/53
- AC: AC-001, AC-002, AC-004
- Зависит от: Phase 1 (нужен стабильный reconnect для тестов)
- Проверка: `dig` ×2 → второй ответ из кэша

### Phase 3 — Exclude Domains

- Что: pre-resolve + route exclusion + прямой DNS для excluded
- AC: AC-006, AC-007
- Зависит от: Phase 2 (DnsTracker нужен для tracking excluded IPs)
- Проверка: `curl https://ozon.ru` не создаёт трафика на сервер

### Phase 4 — Config + Toggle

- Что: dnsCacheEnabled поле, UI toggle, QR/JSON propagation
- AC: AC-008, AC-009
- Зависит от: Phase 2, Phase 3 (без них toggle нечего отключать)
- Проверка: toggle off → поведение как до фичи

## Порядок реализации

1. **Phase 1** — WS:EOF fix (первым, без него тестировать DNS cache невозможно — частые обрывы)
2. **Phase 2** — DNS cache (базовый функционал, можно тестировать сразу после Phase 1)
3. **Phase 3** — Exclude domains (строится поверх DnsTracker)
4. **Phase 4** — Config + UI (замыкает фичу, добавляет управление)

Phase 1 и Phase 4 можно безопасно делать параллельно,
но Phase 2 и Phase 3 зависят от Phase 1.

## Риски

- **R1 [DNS parse complexity]**: DNS wire-format парсинг (RFC 1035) — Java stdlib не имеет готового парсера для A/AAAA. Реализация compression pointer + label parsing может быть хрупкой.
  - Mitigation: скопировать подход из Go `dns/tracker.go:ParseDNSResponse()`; покрыть unit-тестами (valid, truncated, compression, EDNS0). Использовать `java.nio.ByteBuffer`.

- **R2 [Protect race]**: `protect()` на `DatagramSocket` для прямого DNS может не сработать если VPN ещё не fully established.
  - Mitigation: pre-resolve excluded domains ДО `establish()`, для рантайм DNS использовать `protect()` на уже готовом сокете.

- **R3 [Multiple concurrent DNS]**: Несколько приложений одновременно делают DNS → race condition при записи ответов в TUN output (разные потоки).
  - Mitigation: `tunReader()` уже однопоточный (single coroutine). `handleFrame()` вызывается из onFrame callback (OkHttp thread) — синхронизация через `Mutex` на DnsCache запись.

- **R4 [Backward compat QR]**: Старые QR коды без `dns_cache_enabled` должны корректно загружаться.
  - Mitigation: `ignoreUnknownKeys = true` в `Json` конфиге (уже установлен). `dnsCacheEnabled` defaults to `false`.

## Rollout и compatibility

- `dnsCacheEnabled` по умолчанию `false` — обратно совместимо
- Старые QR-коды без этого поля работают (ignoreUnknownKeys)
- Server-side изменения НЕ требуются
- Phase 1 (WS:EOF fix) можно выкатывать отдельно, без конфигов
- Логи на debug level для диагностики cache hit/miss

## Проверка

### Automated tests

| Уровень | Что тестировать | Файл |
|---|---|---|
| Unit | DnsCache: set/get/expiry/LRU | `dns/DnsCacheTest.kt` |
| Unit | DnsTracker: track/lookup/expiry | `dns/DnsTrackerTest.kt` |
| Unit | DnsResolver: resolve excluded | `dns/DnsResolverTest.kt` |
| Unit | ConnectionConfig: serialization round-trip | `config/AppConfigTest.kt` |
| Unit | WebConfig: QR JSON mapping | `ui/QrScannerScreenTest.kt` |
| Integration | tunReader: DNS intercept cache hit | `KvnVpnServiceTest.kt` |
| Integration | WS:EOF → reconnect → traffic resume | `KvnVpnServiceTest.kt` |

### Manual checks

- Phase 1: kill WS → `ping` не прерывается > 3s
- Phase 2: `dig example.com` ×2 → лог CACHE HIT
- Phase 3: `curl https://ozon.ru` → tcpdump сервера не показывает пакетов
- Phase 4: Toggle OFF → поведение как до фичи

### AC ↔ DEC coverage

| AC | DEC | Проверка |
|---|---|---|
| AC-001, AC-002 | DEC-001 | Unit: DnsCache hit/miss |
| AC-003, AC-005 | — | Manual: kill WS |
| AC-004 | — | Unit: DnsCache expiry |
| AC-006 | DEC-002 | Manual: curl excluded |
| AC-007 | DEC-001 | Integration: tunReader |
| AC-008 | — | E2E: toggle off |
| AC-009 | DEC-003 | Unit: QR round-trip |

## Соответствие конституции

- Нет конфликтов.
- Kotlin 1.9+, `@Serializable`, корутины, OkHttp — уже в зависимостях.
- Trace-маркеры `@sk-task` и `@sk-test` будут добавлены в коде над объявлениями классов/методов.
