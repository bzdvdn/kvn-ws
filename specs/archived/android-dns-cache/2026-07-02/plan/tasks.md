# DNS Cache + Exclude Domains + WS: EOF Reconnect — Задачи

## Phase Contract

Inputs: plan.md, data-model.md, spec.md
Outputs: tasks.md с 4 фазами, покрытием всех 9 AC
Stop if: нет.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `KvnVpnService.kt` | T1.1, T1.2, T2.4, T3.1, T3.2 |
| `dns/DnsCache.kt` | T2.1 |
| `dns/DnsTracker.kt` | T2.2 |
| `dns/DnsResolver.kt` | T3.2 |
| `dns/DnsInterceptor.kt` | T2.4 |
| `dns/DnsParser.kt` | T2.3 |
| `config/AppConfig.kt` | T4.1 |
| `ui/ConnectScreen.kt` | T4.4 |
| `ui/QrScannerScreen.kt` | T4.2, T4.3 |
| `ui/MainViewModel.kt` | T4.4 |
| `config/client.go` (Go) | T4.5 |
| `dns/DnsCacheTest.kt` | T5.1 |
| `dns/DnsTrackerTest.kt` | T5.1 |
| `dns/DnsResolverTest.kt` | T5.1 |
| `config/AppConfigTest.kt` | T5.2 |
| `ui/QrScannerScreenTest.kt` | T5.3 |
| `KvnVpnServiceTest.kt` | T5.4 |

## Implementation Context

- **Цель MVP:** WS:EOF fix (Phase 1) — восстановление трафика после обрыва WS без ручного disconnect/connect.
- **Инварианты:**
  - `tunReaderStarted` сбрасывается в `false` при `DISCONNECTED`
  - TUN fd закрывается перед reconnect (нет утечек)
  - DNS wire-формат парсится только для A (type=1) и AAAA (type=28)
  - DnsCache LRU max 1024 entry; TTL min 1s, max 86400s
- **Ошибки/коды:**
  - Невалидный DNS wire-формат → не кэшируется, forward как есть
  - excluded домен не резолвится → log error, continue without exclude
- **Контракты/протокол:**
  - `FRAME_TYPE_DNS` wire format не меняется
  - QR JSON: snake_case (`dns_cache_enabled`); Android Config: camelCase (`dnsCacheEnabled`)
  - `FRAME_TYPE_DNS` для excluded доменов не отправляется (прямой DNS)
- **Границы scope:**
  - Не делаем standalone DNS proxy (как в Go `dnsproxy`)
  - Не делаем динамическое обновление exclude routes (только pre-resolve при старте)
- **Proof signals:**
  - Phase 1: kill WS → curl восстанавливается < 10s
  - Phase 2: `dig example.com` ×2 → второй CACHE HIT
  - Phase 3: `curl https://ozon.ru` → 0 FRAME_TYPE_DATA на сервер
  - Phase 4: toggle OFF → поведение как до фичи
- **References:** DEC-001 (TUN-level DNS intercept), DEC-002 (pre-resolve), DEC-003 (QR format), DM-001–DM-004

## Фаза 1: WS: EOF Fix

Цель: исправить reconnect — сброс `tunReaderStarted` + закрытие TUN при `DISCONNECTED`.

- [x] T1.1 Добавить `closeTun()` метод в `KvnVpnService`, вынести закрытие TUN fd из `onDestroy()`.
  - Outcome: TUN fd изолированно закрывается без остановки сервиса
  - Touches: `KvnVpnService.kt`

- [x] T1.2 Исправить `onConnectionStateChange` — при `DISCONNECTED` вызывать `closeTun()` и сбрасывать `tunReaderStarted = false`.
  - Outcome: reconnect пересоздаёт TUN через `establishTun()`
  - Touches: `KvnVpnService.kt`
  - AC: AC-003, AC-005
  - Reference: DEC-001

## Фаза 2: DNS Cache

Цель: TTL-кэш domain→IP, IP→domain трекер, DNS wire-парсер, перехват UDP/53 в tunReader.

- [x] T2.1 Реализовать `DnsCache` — TTL-кэш с LRU-эвикцией (1024 entry), thread-safe (Mutex).
  - Outcome: `DnsCache.get(domain) -> List<InetAddress>?`, `DnsCache.set(domain, ips, ttl)`
  - Touches: `dns/DnsCache.kt`

- [x] T2.2 Реализовать `DnsTracker` — reverse map IP→domain с TTL, thread-safe.
  - Outcome: `DnsTracker.lookup(ip) -> String?`, `DnsTracker.track(domain, ips, ttl)`
  - Touches: `dns/DnsTracker.kt`

- [x] T2.3 Реализовать `DnsParser` — парсинг DNS wire-формата (RFC 1035) для A/AAAA.
  - Outcome: `DnsParser.parseResponse(raw) -> List<InetAddress>`, парсит compression pointers
  - Touches: `dns/DnsParser.kt`

- [x] T2.4 Реализовать DNS intercept в `tunReader()` — перехват UDP/53 пакетов, проверка кэша, forward при miss.
  - Outcome: `tunReader()` проверяет dstPort==53 → парсит QNAME → cache hit → ответ в TUN output; miss → forward как FRAME_TYPE_DNS; `handleFrame()` для FRAME_TYPE_DNS парсит ответ → DnsCache.set + DnsTracker.track
  - Touches: `KvnVpnService.kt`, `dns/DnsParser.kt`, `dns/DnsCache.kt`, `dns/DnsTracker.kt`, `dns/DnsInterceptor.kt`
  - AC: AC-001, AC-002, AC-004

## Фаза 3: Exclude Domains

Цель: pre-resolve excluded доменов, route exclusion, прямой DNS для excluded.

- [x] T3.1 Реализовать `resolveExcludedDomains()` — pre-resolve всех доменов из `routingExcludeDomains` через реальную сеть, добавление `/32` exclude routes до `establish()`.
  - Outcome: `computeVpnRoutes()` включает /32 для IP excluded доменов; `Builder.addRoute()` вызывается до establish()
  - Touches: `KvnVpnService.kt`
  - AC: AC-006
  - Reference: DEC-002

- [x] T3.2 Реализовать `DnsResolver` и прямой DNS для excluded доменов — при DNS-запросе к excluded domain резолвим через `protect()` (не через TUN).
  - Outcome: `tunReader()` определяет QNAME в excludeDomains → `DnsResolver.resolve(qname)` через `DatagramSocket` с `protect()` → ответ в TUN output + DnsTracker.track
  - Touches: `dns/DnsResolver.kt`, `KvnVpnService.kt`, `dns/DnsTracker.kt`
  - AC: AC-007
  - Reference: DEC-001

## Фаза 4: Config + UI Toggle

Цель: `dnsCacheEnabled` поле в ConnectionConfig, UI toggle, QR/JSON propagation, Go ClientConfig.

- [x] T4.1 Добавить `dnsCacheEnabled: Boolean = false` в `ConnectionConfig` (Android, @Serializable).
  - Outcome: новое поле с default=false, обратно совместимо
  - Touches: `config/AppConfig.kt`
  - AC: AC-008
  - Reference: DM-001

- [x] T4.2 Добавить `WebDnsCacheCfg` + `val dns_cache: WebDnsCacheCfg? = null` в `WebRoutingCfg` (QR web JSON модель).
  - Outcome: QR/JSON импорт поддерживает новое поле
  - Touches: `ui/QrScannerScreen.kt`
  - AC: AC-009
  - Reference: DM-002, DEC-003

- [x] T4.3 Обновить `webToAndroidConfig()` и `configToWebJson()` — маппинг `dns_cache.enabled` ↔ `dnsCacheEnabled`.
  - Outcome: двусторонняя конвертация поля между web JSON и Android Config
  - Touches: `ui/QrScannerScreen.kt`
  - AC: AC-009
  - Reference: DM-002, RQ-013

- [x] T4.4 Добавить DNS cache toggle в UI (экран настроек ConnectScreen).
  - Outcome: Switch "DNS Cache" в settings; значение сохраняется в ConnectionConfig через buildConfig()
  - Touches: `ui/ConnectScreen.kt`
  - AC: AC-008
  - Reference: RQ-014

- [x] T4.5 Проверить Go `ClientConfig` (`client.go`) — убедиться что `dns_cache.enabled` и `exclude_domains` пробрасываются в QR JSON через kvn-web.
  - Outcome: kvn-web JSON содержит `routing.dns_cache.enabled` и `routing.exclude_domains` (уже есть)
  - Touches: `config/client.go` (Go) — не требует изменений

## Фаза 5: Тесты и проверка

Цель: automated coverage для всех новых компонентов.

- [x] T5.1 Написать unit-тесты для DnsCache (set/get/expiry/LRU/race), DnsTracker (track/lookup/expiry/race), DnsParser (valid/truncated/compression/EDNS0).
  - Outcome: 8 tests DnsCacheTest, 8 tests DnsTrackerTest, 10 tests DnsParserTest
  - Touches: `dns/DnsCacheTest.kt`, `dns/DnsTrackerTest.kt`, `dns/DnsParserTest.kt`
  - AC: AC-001, AC-002, AC-004
  - DnsResolver: не тестируется (требует сеть/Android protect)

- [x] T5.2 Написать unit-тест для ConnectionConfig serialization round-trip с dnsCacheEnabled.
  - Touches: `config/DnsCacheConfigTest.kt`
  - AC: AC-008

- [x] T5.3 Написать unit-тест для QR JSON mapping — webToAndroidConfig + configToWebJson с dns_cache.
  - Touches: `config/DnsCacheConfigTest.kt`
  - AC: AC-009

- [x] T5.4 Написать integration-тест для KvnVpnService — WS:EOF reconnect + DNS intercept cache hit.
  - Outcome: 6 тестов (closeTun, autoReconnect guard, cache hit, cache miss, disabled toggle, handleTunnelResponse); Robolectric + reflection для доступа к private методам
  - Touches: `KvnVpnServiceTest.kt`, `build.gradle.kts`
  - AC: AC-003, AC-005, AC-001, AC-002, AC-008

## Покрытие критериев приемки

- AC-001 (cache hit) → T2.4, T5.1, T5.4
- AC-002 (cache miss) → T2.4, T5.1
- AC-003 (WS:EOF reconnect) → T1.1, T1.2, T5.4
- AC-004 (TTL invalidation) → T2.1, T5.1
- AC-005 (no data loss) → T1.2, T5.4
- AC-006 (exclude pre-resolve) → T3.1
- AC-007 (direct DNS exclude) → T3.2, T5.1
- AC-008 (toggle off) → T4.1, T4.4, T5.2
- AC-009 (config propagation) → T4.2, T4.3, T4.5, T5.3

## Заметки

- Phase 1 обязательна первой (без неё тестирование DNS cache невозможно — частые WS обрывы)
- Phase 2 и 3 зависят от Phase 1
- Phase 4 не зависит от Phase 2/3 функционально, но toggle без функционала бесполезен
- Phase 5 (тесты) пишутся после/параллельно с соответствующими фазами
- Trace-маркеры: `@sk-task android-dns-cache#T<phase>.<idx>` над объявлениями классов/методов
