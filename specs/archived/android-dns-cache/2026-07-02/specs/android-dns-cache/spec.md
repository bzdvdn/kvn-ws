# DNS Cache + Exclude Domains + WS: EOF Reconnect для Android

## Scope Snapshot

- In scope: DNS-кэширование на Android-клиенте (перехват DNS на уровне TUN, TTL-кэш, IP→domain трекер), поддержка exclude_domains (трафик к указанным доменам идёт напрямую вне туннеля), исправление реконнекта при WS: EOF.
- Out of scope: server-side DNS proxy изменения, split-tunnel по CIDR (уже есть через `routingExcludeRanges`), динамическое обновление TUN routes через пересоздание (только pre-resolve при старте).

## Цель

Пользователь Android-клиента получает: (1) трафик к указанным доменам (ozon.ru, и т.д.) идёт напрямую, минуя VPN-туннель, (2) снижение latency DNS-запросов через локальный TTL-кэш, (3) автоматическое восстановление трафика после WS: EOF без ручного disconnect/connect. Фича считается успешной, когда `curl https://ozon.ru` через активный VPN не создаёт `FRAME_TYPE_DATA` на сервер, а DNS-запросы возвращаются из кэша в пределах TTL, и WS: EOF не требует ручного reconnect.

## Основной сценарий

1. Пользователь запускает KVN VPN на Android с конфигом, где `excludeDomains: ["ozon.ru", "yandex.ru"]`.
2. До старта VPN: `resolveServerIpsBeforeVpn()` + `resolveExcludedDomains()` — все excluded домены резолвятся, их IP добавляются как `/32` exclude routes в `Builder.addRoute()`.
3. TUN-устройство создано с excluded IP — трафик к ozon.ru/yandex.ru идёт напрямую с самого начала.
4. Приложение на устройстве делает DNS-запрос (UDP/53) — `tunReader()` читает пакет.
5. Если QNAME в списке excluded → DNS резолвится напрямую (через реальную сеть с `protect()`), IP→domain сохраняется в `DnsTracker`.
6. Если QNAME не excluded и есть в кэше → ответ из кэша.
7. Если QNAME не excluded и не в кэше → forward через туннель как `FRAME_TYPE_DNS`, ответ кэшируется.
8. Data-пакеты к IP excluded доменов не попадают в TUN — route exclusion на уровне `Builder.addRoute()` работает для всех протоколов (TCP/UDP/ICMP). DnsTracker используется как fallback для IP, не известных на момент `establish()`, и для диагностики.
9. При WS: EOF → `ReconnectManager` делает reconnect, TUN пересоздаётся, excluded domains pre-resolve повторяется.

## User Stories

none — infra-улучшения.

## MVP Slice

- AC-003 (WS: EOF reconnect) + AC-006 (exclude_domains pre-resolve + route exclusion) — критический reliability + domain exclude.
- AC-001/AC-002 (DNS cache) — следующий приоритет для performance.

## First Deployable Outcome

APK: после `./gradlew assembleDebug`:

1. `excludeDomains: ["ozon.ru"]` в конфиге — `curl https://ozon.ru` во время VPN работает напрямую (проверка: tcpdump не показывает пакетов к серверу).
2. При WS: EOF трафик восстанавливается автоматически.
3. DNS-запросы кэшируются.

## Scope

- Новый модуль `com.kvn.client.dns`:
  - `DnsCache.kt` — TTL-кэш domain→IP (аналог Go `dns/cache.go`)
  - `DnsTracker.kt` — reverse-map IP→domain с TTL (аналог Go `dns/tracker.go`)
  - `DnsResolver.kt` — прямой DNS-резолвинг через реальную сеть (с `protect()`) для excluded доменов
- Модификация `ConnectionConfig` (Android) — добавление `dnsCacheEnabled: Boolean`, `excludeDomains: List<String>`
- Модификация Go `ClientConfig` (`src/internal/config/client.go`) — добавление `dns_cache_enabled`, `exclude_domains` для kvn-web
- Модификация QR-сериализации (`src/android/.../config/`) — поля `dns_cache_enabled`, `exclude_domains` в JSON для QR-кода
- Модификация Android UI (`MainViewModel` + экран Settings) — toggle `dnsCacheEnabled` в настройках приложения
- Модификация `KvnVpnService.kt`:
  - `resolveExcludedDomains()` — pre-resolve excluded доменов до `establish()`
  - `establishTun()` — добавление `/32` exclude routes для IP excluded доменов
  - `tunReader()` — перехват UDP/53 только если `dnsCacheEnabled=true`; проверка кэша + трекера; для excluded domain → прямой резолв
  - `handleFrame()` для `FRAME_TYPE_DNS` — парсинг и кэширование ответов, обновление `DnsTracker`
  - `onConnectionStateChange` — сброс `tunReaderStarted` и `closeTun()` при `DISCONNECTED`
- Исправление WS: EOF в `onConnectionStateChange` + `ReconnectManager`

## Контекст

- Android VpnService НЕ позволяет динамически менять routes после `establish()`. Поэтому exclude_domains реализуется через pre-resolve при старте — все IP excluded доменов добавляются как `/32` exclude routes.
- Если excluded домен меняет IP во время сессии, новый трафик к старому IP всё равно идёт напрямую (IP уже исключён). Трафик к новому IP пойдёт через туннель до следующего reconnect. Это acceptable trade-off для MVP.
- Go-сервер уже имеет полный стек: `dns/cache.go`, `dns/resolver.go`, `dns/tracker.go` — Android-реализация структурный аналог на Kotlin.
- Текущий reconnect при WS: EOF не сбрасывает `tunReaderStarted` → `establishTun()` не вызывается → root cause проблемы «трафик не восстанавливается».
- OkHttp 4.12.0: `WebSocketListener.onFailure` — единственный колбэк для транспортных ошибок.
- UDP-пакеты для excluded доменов можно отправлять напрямую через `DatagramSocket` с `protect()`. TCP-пакеты для excluded доменов не требуют userspace-обработки — IP уже исключены на уровне routes.
- Конфиг передаётся на Android через QR-код (JSON) или нативный JSON. `dns_cache_enabled` и `exclude_domains` должны быть частью этой сериализации.

## Зависимости

- OkHttp 4.12.0 (уже)
- Kotlin stdlib (уже)
- `ConnectionConfig` (уже, нужно расширить)
- `src/internal/config/client.go` (Go ClientConfig — нужно расширить)
- `src/android/.../config/ConnectionConfig.kt` + QR-сериализация (нужно расширить)
- `DnsTracker` требует парсинга DNS wire-формата — only stdlib (`java.nio.ByteBuffer`)

## Требования

- RQ-001 Android-клиент ДОЛЖЕН иметь конфигурационный флаг `dnsCacheEnabled: Boolean` (true — DNS-кэш и exclude_domains активны, false — всё работает как сейчас, без перехвата DNS).
- RQ-002 Если `dnsCacheEnabled=false`: DNS-запросы НЕ перехватываются, `DnsTracker` НЕ обновляется, exclude_domains НЕ резолвятся, `FRAME_TYPE_DNS` forwarding без изменений.
- RQ-003 Android-клиент ДОЛЖЕН принимать конфигурацию `excludeDomains: List<String>` — список доменов, трафик к которым идёт напрямую вне туннеля.
- RQ-004 До `establish()` TUN ДОЛЖНЫ быть pre-resolved все excluded домены через реальную сеть; их IP добавляются как `/32` exclude routes.
- RQ-005 DNS-запросы к excluded доменам ДОЛЖНЫ резолвиться напрямую (через `protect()`), а не через туннель.
- RQ-006 IP→domain mapping (DnsTracker) ДОЛЖЕН обновляться при каждом DNS-ответе для классификации data-пакетов.
- RQ-007 При WS: EOF система ДОЛЖНА автоматически reconnect, пересоздать TUN (включая заново pre-resolve excluded domains) и возобновить forwarding.
- RQ-008 DNS-кэш (domain→IP) ДОЛЖЕН использовать TTL из upstream DNS-ответа, min 1s, max 86400s.
- RQ-009 DNS-кэш + DnsTracker ДОЛЖНЫ быть потокобезопасными (coroutine-safe).
- RQ-010 Кэшируются только A (type=1) и AAAA (type=28); NS/SOA/прочие forward-ятся без кэширования.
- RQ-011 Go `ClientConfig` (для kvn-web) ДОЛЖЕН включать поля `dns_cache_enabled` и `exclude_domains` в секции клиентского конфига.
- RQ-012 QR-код/JSON-сериализация Android `ConnectionConfig` ДОЛЖНА включать `dns_cache_enabled` и `exclude_domains`, чтобы конфиг из kvn-web автомативно подтягивался на Android-клиент.
- RQ-013 Маппинг имён полей: в JSON/QR — `snake_case` (`dns_cache_enabled`, `exclude_domains`); в Kotlin `ConnectionConfig` — `camelCase` (`dnsCacheEnabled`, `excludeDomains`). Десериализация через `@SerialName` или ручной маппинг.
- RQ-014 Android UI ДОЛЖЕН предоставлять switch/toggle для `dnsCacheEnabled` на экране настроек/подключения. Значение toggle сохраняется в `ConnectionConfig` и применяется при следующем старте VPN.

## Вне scope

- Динамическое обновление exclude routes во время сессии (при смене IP excluded домена) — только при reconnect
- Server-side DNS proxy изменения
- Изменение протокола `FRAME_TYPE_DNS`
- Hot-reload exclude_domains или dnsCacheEnabled через UI (только через перезапуск VPN)
- Userspace TCP proxy для excluded доменов (TCP идёт через route exclusion)

## Критерии приемки

### AC-001 DNS cache hit возвращает закэшированный ответ

- Почему это важно: снижение latency и трафика для повторных DNS-запросов к не-excluded доменам
- **Given** DNS-кэш содержит запись `example.com → [93.184.216.34]` с TTL 300s, TTL не истёк, `example.com` не в excludeDomains
- **When** приложение отправляет DNS A-запрос для `example.com` через TUN
- **Then** `tunReader()` перехватывает пакет, находит запись в кэше, формирует DNS-ответ и пишет в TUN output без отправки `FRAME_TYPE_DNS` на сервер
- Evidence: лог `"DNS cache HIT example.com -> 93.184.216.34"`

### AC-002 DNS cache miss forward-ит запрос и кэширует ответ

- Почему это важно: cache-first не ломает резолвинг для незакэшированных не-excluded доменов
- **Given** DNS-кэш пуст, домен не в excludeDomains
- **When** приложение отправляет DNS A-запрос для `example.org` через TUN
- **Then** `tunReader()` не находит запись, forward-ит пакет как `FRAME_TYPE_DNS` на сервер; при получении `FRAME_TYPE_DNS`-ответа парсится A/AAAA и сохраняется в кэш + DnsTracker
- Evidence: после ответа кэш содержит `example.org → <IP>`; DnsTracker содержит `<IP> → example.org`

### AC-003 WS: EOF автоматический reconnect с пересозданием TUN

- **Given** VPN-туннель активен, `tunReaderStarted = true`, есть excluded domain
- **When** происходит WS: EOF
- **Then** система: `DISCONNECTED` → старый TUN fd закрывается, ресурсы освобождены → ReconnectManager → новый WebSocket → handshake → `establishTun()` с повторным `resolveExcludedDomains()` → `tunReader()` перезапущен → трафик восстановлен
- Evidence: после обрыва `ping/curl` через/вне туннеля работают; log: "excluded domains re-resolved"

### AC-004 TTL-инвалидация DNS-кэша

- **Given** в кэше `example.com` с TTL=60s
- **When** проходит 61s, затем A-запрос для `example.com`
- **Then** запись удалена, запрос — miss (AC-002)
- Evidence: поведение идентично AC-002

### AC-005 WS: EOF не теряет данные TUN во время реконнекта

- **Given** поток UDP-пакетов через TUN
- **When** WS: EOF
- **Then** TUN fd НЕ закрывается, `tunReader()` продолжает читать из fd (буферы ядра); после CONNECTED `transportClient?.send(frame)` снова возвращает true; error log не содержит "tun reader crashed"
- Evidence: `transportClient?.send(frame)` возвращает `false` во время `transportClient = null`, затем `true` после CONNECTED; `tunReader()` не бросает исключение

### AC-006 Exclude domains pre-resolve + route exclusion

- Почему это важно: прямой доступ к ozon.ru без прохождения через VPN-сервер
- **Given** конфиг содержит `excludeDomains: ["ozon.ru"]`, IP ozon.ru = `[213.180.193.250]`
- **When** `doStart()` → `resolveExcludedDomains()` → `establishTun()`
- **Then** `computeVpnRoutes()` включает `213.180.193.250/32` как exclude; трафик к ozon.ru идёт напрямую
- Evidence: `curl --resolve ozon.ru:443:213.180.193.250 https://ozon.ru` во время VPN не создаёт `FRAME_TYPE_DATA` на сервер (проверка: server metrics или tcpdump)

### AC-007 DNS для excluded доменов резолвится напрямую

- **Given** `excludeDomains: ["ozon.ru"]`
- **When** приложение делает DNS A-запрос для `ozon.ru` через TUN
- **Then** `tunReader()` определяет QNAME в excludeDomains → резолвит через реальную сеть (`protect()`) → ответ пишется в TUN output → IP сохраняется в DnsTracker
- Evidence: лог `"DNS exclude DIRECT ozon.ru -> 213.180.193.250"`; `FRAME_TYPE_DNS` для ozon.ru не отправляется

### AC-008 DNS cache/exclude можно отключить через конфиг

- Почему это важно: обратная совместимость — пользователь может отключить фичу если что-то пошло не так
- **Given** `dnsCacheEnabled=false` в конфиге Android-клиента
- **When** `doStart()` → `buildDnsCache()` инициализирует `DnsCache`/`DnsTracker` как no-op заглушки; `tunReader()` не перехватывает UDP/53
- **Then** весь DNS-трафик forward-ится через туннель как `FRAME_TYPE_DNS` без изменений; exclude_domains игнорируются
- Evidence: поведение идентично текущей версии без фичи — DNS-запросы не логируются как cache hit/miss, все DNS пакеты уходят на сервер

### AC-009 Конфиг из kvn-web включает dns_cache_enabled + exclude_domains

- Почему это важно: пользователь настраивает один раз в kvn-web, Android подхватывает автоматически
- **Given** kvn-web сконфигурирован с `dns_cache_enabled: true, exclude_domains: ["ozon.ru"]`
- **When** пользователь сканирует QR-код из kvn-web Android-клиентом
- **Then** Android `ConnectionConfig` после десериализации JSON содержит `dnsCacheEnabled=true`, `excludeDomains=["ozon.ru"]`
- Evidence: `ConnectionConfig.deserialize()` возвращает объект с полями `dnsCacheEnabled=true`, `excludeDomains=listOf("ozon.ru")`

## Допущения

- TTL из DNS-ответа авторитетен (no adaptive TTL)
- Android VpnService routes статичны после establish()
- Смена IP excluded домена во время сессии — редкий случай; корректно обрабатывается при reconnect
- TCP для excluded доменов не требует userspace-обработки — route exclusion на уровне ядра работает корректно
- OkHttp onFailure покрывает все транспортные ошибки
- Конфиг передаётся через QR (JSON) или нативный JSON — поля `dns_cache_enabled`, `exclude_domains` сериализуются как snake_case

## Критерии успеха

- SC-001 Время восстановления после WS: EOF ≤ 10s (95p)
- SC-002 DNS cache hit ratio ≥ 60% после 5 мин работы
- SC-003 exclude_domains трафик не создаёт FRAME_TYPE_DATA (0 пакетов на сервер для excluded IP)
- SC-004 Нет утечек TUN fd при reconnect

## Краевые случаи

- excludeDomains пуст — поведение без изменений (нет pre-resolve, кэш для не-excluded доменов работает)
- dnsCacheEnabled=false — весь DNS-стека отключён, никаких изменений против текущего поведения
- Домен не резолвится при старте — log error, continue without exclude; при reconnect — повторная попытка
- TTL=0 для excluded domain — IP всё равно кэшируется в DnsTracker для routing, но domain→кэш не сохраняется (always re-resolve)
- Множественные WS: EOF → MAX_RETRIES (10) → safeStop()
- WS: EOF до первого ServerHello → reconnect, `tunReaderStarted = false`, handshake + resolve excluded повторяется
- Невалидный DNS wire-формат — не кэшируется
- EDNS0 — forward без изменений
- excluded domain совпадает с serverAddress — приоритет у serverAddress (уже исключён через `resolveServerIpsBeforeVpn`)

## Открытые вопросы

- Размер кэша по умолчанию? (1024 записи, LRU-эвикция при переполнении)
