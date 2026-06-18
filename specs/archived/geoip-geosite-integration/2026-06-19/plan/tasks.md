# GeoIP / GeoSite / External Sources для роутинга — Задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/routing/geoip/geoip.pb.go` (new, gen) | T1.1 |
| `src/internal/routing/geoip/geosite.pb.go` (new, gen) | T1.1 |
| `src/internal/config/client.go` | T1.2 |
| `src/internal/routing/resolver.go` (new) | T2.1, T3.1, T4.1, T4.2 |
| `src/internal/routing/geoip/parser.go` (new) | T2.1, T4.1 |
| `src/internal/routing/geoip/geoip.proto` (new) | T1.1 |
| `src/internal/routing/geoip/geosite.proto` (new) | T1.1 |
| `src/internal/bootstrap/client/client.go` | T3.1 |
| `src/internal/bootstrap/client/proxy.go` | T3.1 |
| `src/internal/bootstrap/client/tun.go` | T3.1 |
| `src/internal/bootstrap/relay/bootstrap.go` | T3.1 |
| `src/internal/routing/router.go` | T4.2 |
| `src/internal/webui/handler_config.go` | T5.1 |
| `src/internal/webui/frontend/` | T5.1 |
| `src/internal/config/client_test.go` | T3.2 |
| `src/internal/routing/resolver_test.go` (new) | T3.2 |
| `src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt` | T5.2 |
| `src/android/app/src/main/kotlin/com/kvn/client/ui/ConnectScreen.kt` | T5.2 |
| `src/android/app/src/main/kotlin/com/kvn/client/ui/MainViewModel.kt` | T5.2 |

## Implementation Context

- **Цель MVP:** GeoIP + CIDR + URL источники, резолв на старте, мерж с плоскими списками, graceful degradation.
- **Инварианты/семантика:**
  - `SourceRule` — ровно одно поле из `{geoip, geosite, cidr, url}`.
  - Resolver принимает `*RoutingCfg`, возвращает смерженный `*RoutingCfg` с заполненными `IncludeRanges`/`ExcludeRanges`/`IncludeDomains`/`ExcludeDomains`.
  - `geoip_path`/`geosite_path` — статика, без автообновления. `geoip_url`/`geosite_url` — скачивание с TTL 24h.
  - Если ни path, ни url не указаны — debug-лог, пропуск источника (не error).
  - `geoip: "private"` — built-in alias, раскрывается в RFC 1918 + CGNAT + ULA без внешней базы.
- **Ошибки/коды:** нет новых кодов ошибок; resolver логирует warning при битом URL/файле и продолжает без источника.
- **Контракты/протокол:**
  - geoip.dat — v2fly protobuf: `GeoIPList → GeoIP[] { country_code, cidr[] { ip, prefix } }`
  - geosite.dat — v2fly protobuf: `GeoSiteList → GeoSite[] { category_code, domain[] { value, type } }`
  - URL-список: строка = CIDR (если есть `/`) или домен. `#` — комментарий.
  - `google.golang.org/protobuf` — единственная новая зависимость.
- **Границы scope:**
  - Не меняем `RuleSet`, `Router`, `CIDRMatcher`, `DomainMatcher`, `tunnel/session.go`, data-path.
  - Не делаем runtime geoip-матчинг на каждый пакет.
- **Proof signals:**
  - Unit-тест: SourceRule YAML десериализация.
  - Unit-тест: resolver читает тестовый .dat → раскрывает CIDR.
  - Integration: config with sources → resolved → RuleSet → Route().
  - Manual: `geoip: "ru"` → в логах "resolved geoip:ru → N CIDRs".
- **References:** DEC-001 (resolver пакет), DEC-002 (proto-gen), DEC-003 (bootstrap merge), DEC-004 (atomic refresh)

## Фаза 1: База (proto + data model)

Цель: подготовить типы данных и proto-парсеры. Три независимые задачи.

- [x] T1.1 Сгенерировать proto-код для geoip.dat и geosite.dat. Touches: src/internal/routing/geoip/geoip.proto, src/internal/routing/geoip/geosite.proto, src/internal/routing/geoip/geoip.pb.go, src/internal/routing/geoip/geosite.pb.go, go.mod
  - Создать `src/internal/routing/geoip/geoip.proto` и `src/internal/routing/geoip/geosite.proto` со схемами v2fly
  - Сгенерировать `geoip.pb.go` и `geosite.pb.go` через `protoc` + `protoc-gen-go`
  - Проверить, что `google.golang.org/protobuf` есть в go.mod (добавить если нет)
  - Trace: `@sk-task geoip-geosite-integration#T1.1` над proto-файлами

- [x] T1.2 Добавить `SourceRule` тип и расширить `RoutingCfg`. Touches: src/internal/config/client.go
  - Новый тип `SourceRule` с полями `GeoIP *string`, `GeoSite *string`, `CIDR *string`, `URL *string` + методы `Type()`, `Value()`, `Valid()`
  - `RoutingCfg`: новые поля `GeoIPPath`, `GeoSitePath`, `GeoIPURL`, `GeoSiteURL`, `SourceTTL`, `IncludeSources`, `ExcludeSources`
  - Валидатор SourceRule: ровно одно поле задано, иначе warning + пропуск
  - Trace: `@sk-task geoip-geosite-integration#T1.2` над `SourceRule` и его методами, над `RoutingCfg`

## Фаза 2: Resolver

Цель: резолв источников — GeoIP, CIDR, URL, merge + dedup, graceful degradation.

- [x] T2.1 Реализовать Resolver: GeoIP + CIDR + URL + merge + graceful degradation. Touches: src/internal/routing/resolver.go, src/internal/routing/geoip/parser.go
  - `src/internal/routing/resolver.go`:
    - `NewResolver(cfg, cacheDir, logger)`
    - `Resolve() (*RoutingCfg, error)` — проходит IncludeSources/ExcludeSources:
      - `geoip:` → читает geoip.dat (из geoip_path или geoip_url), находит код страны, раскрывает CIDR
      - `cidr:` → добавляет напрямую
      - `url:` → скачивает/читает файл, парсит CIDR и домены
      - `geosite:` → заглушка (MVP) / реальная реализация (если включена в скоуп)
    - `geoip: "private"` → built-in список частных диапазонов
    - Merge раскрытых CIDR с `IncludeRanges`/`ExcludeRanges` (dedup)
    - Merge раскрытых доменов с `IncludeDomains`/`ExcludeDomains` (dedup)
    - Graceful degradation: битый URL/файл → warning, continue
    - Если ни path, ни url → debug, пропуск
  - `src/internal/routing/geoip/parser.go`:
    - `ReadGeoIP(path string) (map[string][]netip.Prefix, error)` — читает .dat через proto, возвращает страна→CIDR
    - `ReadGeoSite(path string) (map[string][]string, error)` — читает .dat через proto, возвращает категория→домены (заглушка для MVP)
  - `Refresh()` — заглушка (возвращает Resolve)
  - Trace: `@sk-task geoip-geosite-integration#T2.1` над `Resolver`, `ReadGeoIP`, `ReadGeoSite`

## Фаза 3: Bootstrap + MVP тесты

Цель: интеграция resolver в точки входа клиента + верификация.

- [x] T3.1 Интегрировать resolver в bootstrap клиента и релея. Touches: src/internal/bootstrap/client/client.go, src/internal/bootstrap/client/tun.go, src/internal/bootstrap/client/proxy.go, src/internal/bootstrap/relay/bootstrap.go
  - `src/internal/bootstrap/client/client.go`: перед `NewRuleSet(cfg.Routing)` — создать resolver, вызвать `Resolve()`, подменить `cfg.Routing` на смерженный
  - `src/internal/bootstrap/client/tun.go`: аналогично для TUN-режима
  - `src/internal/bootstrap/client/proxy.go`: аналогично для proxy-режима
  - `src/internal/bootstrap/relay/bootstrap.go`: вызвать `resolveDirectSources()` перед `newDirectRuleSet()`
  - Trace: `@sk-task geoip-geosite-integration#T3.1` над модифицированными функциями

- [x] T3.3 Реализовать поддержку DirectSources в relay terminator. Touches: src/internal/config/client.go, src/internal/bootstrap/relay/router.go, src/internal/routing/resolver.go
  - `src/internal/config/client.go`: `RelayRoutingCfg` — добавить `DirectSources []SourceRule`, `GeoIPPath`, `GeoSitePath`, `GeoIPURL`, `GeoSiteURL`, `SourceTTL`
  - `src/internal/bootstrap/relay/router.go`: `resolveDirectSources()` — создаёт временный `RoutingCfg` с полями путей/URL, создаёт `Resolver`, вызывает `ResolveSources()`, мержит в `DirectRanges`/`DirectDomains`
  - `src/internal/routing/resolver.go`: `ResolveSources(sources []SourceRule) (*SourcesResult, error)` — публичный метод для резолва произвольного списка источников
  - `src/internal/bootstrap/relay/bridge.go`: добавить `configPath` в `Relay` struct, передавать в `NewFromConfig`
  - Trace: `@sk-task geoip-geosite-integration#T3.3` над `RelayRoutingCfg`, `resolveDirectSources`, `ResolveSources`

- [x] T3.2 Написать unit-тесты MVP. Touches: src/internal/config/client_test.go, src/internal/routing/resolver_test.go
  - Trace: `@sk-test geoip-geosite-integration#T3.2` над тестовыми функциями

## Фаза 4: GeoSite + Refresh

Цель: GeoSite поддержка и механизм обновления без перезапуска.

- [x] T4.1 Добавить поддержку `SourceRule.geosite`. Touches: src/internal/routing/geoip/parser.go, src/internal/routing/resolver.go, src/internal/routing/resolver_test.go
  - Парсер `ReadGeoSite()` — читает geosite.dat через proto, возвращает категория→домены
  - Resolver: `geosite:` → раскрытие в домены → мерж с `IncludeDomains`/`ExcludeDomains`
  - Trace: `@sk-task geoip-geosite-integration#T4.1` над `ReadGeoSite` и веткой `geosite` в resolver

- [x] T4.2 Реализовать Refresh button + атомарную подмену RuleSet. Touches: src/internal/routing/resolver.go, src/internal/routing/router.go, src/internal/bootstrap/client/client.go, src/internal/webui/handler_config.go
  - `Resolver.Refresh()` — перескачивает базы, перерезолв, возвращает новый `*RoutingCfg`
  - `TunRouter` / proxy: хранят `atomic.Pointer[RuleSet]`
  - После Refresh: создаём новый `RuleSet(mergedCfg)`, подменяем через `Store()`
  - Старые соединения продолжают использовать старый RuleSet (у каждого своя копия ссылки)
  - Web UI endpoint: POST `/api/config/refresh-sources`
  - Trace: `@sk-task geoip-geosite-integration#T4.2` над `Refresh()`, над `TunRouter.AtomicStore()`, над refresh handler

## Фаза 5: Web UI + Android

Цель: отображение и управление источниками в UI.

- [x] T5.1 Web UI: карточки sources в секции Routing + кнопка Refresh. Touches: src/internal/webui/frontend/src/App.tsx, src/internal/webui/handler_config.go
  - `src/internal/webui/frontend/`:
  - `src/internal/webui/handler_config.go`: GET/PUT для `include_sources`/`exclude_sources`
  - Trace: `@sk-task geoip-geosite-integration#T5.1` над React-компонентами и handler

- [x] T5.2 Android: кнопка Refresh. Touches: src/android/app/src/main/kotlin/com/kvn/client/config/AppConfig.kt, src/android/app/src/main/kotlin/com/kvn/client/ui/ConnectScreen.kt, src/android/app/src/main/kotlin/com/kvn/client/ui/MainViewModel.kt
  - Добавить источник-поля в `ConnectionConfig`: includeSources, excludeSources, geoipPath, geoipUrl, geositePath, geositeUrl, sourceTtlHours
  - Добавить текстовые поля для источников в Routing секцию `ConnectScreen.kt`
  - Добавить кнопку "Refresh Sources" — сохраняет конфиг и отключается для переподключения
  - Trace: `@sk-task geoip-geosite-integration#T5.2`

## Покрытие критериев приемки

- AC-001 -> T1.2, T3.2
- AC-002 -> T2.1, T3.2
- AC-003 -> T2.1, T3.2
- AC-004 -> T2.1, T3.2
- AC-005 -> T2.1, T3.2
- AC-006 -> T2.1, T3.2
- AC-007 -> T2.1, T3.2
- AC-008 -> T4.1
- AC-009 -> T2.1, T3.2
- AC-010 -> T5.1
- AC-011 -> T4.2, T5.2

## Заметки

- T1.1 и T1.2 независимы — можно параллелить
- T2.1 (Resolver) обязателен перед T3.1 (Bootstrap)
- T3.2 (тесты MVP) можно параллелить с T3.1
- T3.3 (relay direct_sources) зависит от T2.1 (Resolver) и может выполняться параллельно с T3.1
- T4.1 (GeoSite) и T4.2 (Refresh) независимы друг от друга
- T5.1 (Web UI) и T5.2 (Android) — последние по порядку
- Все новые поля optional — обратная совместимость полная
