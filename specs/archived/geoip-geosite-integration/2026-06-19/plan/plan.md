# GeoIP / GeoSite / External Sources для роутинга — План

## Цель

Добавить в конфиг роутинга поддержку динамических источников — GeoIP, GeoSite, CIDR, URL — которые резолвятся в плоские CIDR/домены на старте (или по кнопке Refresh). Движок роутинга (`RuleSet`) не меняется, все расширения локализованы в новом пакете `resolver` и точках bootstrap.

## MVP Slice

GeoIP + CIDR + URL источники (AC-001 — AC-007, AC-009). Этого достаточно, чтобы пользователь мог указать `geoip: "ru"` или кастомный URL и получить работающее исключение/включение маршрутов. GeoSite (AC-008), Refresh button (AC-011) и Web UI (AC-010) — следующие итерации.

## First Validation Path

Собрать бинарник (`go build ./src/cmd/client`), положить тестовый `geoip.dat` рядом с конфигом, добавить `routing.exclude_sources: [{geoip: "ru"}]`. Запустить — в логах "resolved geoip:ru → 42 CIDRs". Подключиться — трафик на российские IP идёт напрямую (проверить через `traceroute` или лог роутинга).

## Scope

- `RoutingCfg`: новые поля `GeoIPPath`, `GeoSitePath`, `GeoIPURL`, `GeoSiteURL`, `IncludeSources`, `ExcludeSources`, `SourceTTLHours`
- Новый тип `SourceRule` и модуль `resolver` в `src/internal/routing/resolver.go`
- Генерация proto-кода для geoip.dat/geosite.dat в `src/internal/routing/geoip/`
- Модификация bootstrap клиента (TUN + proxy): вызов resolver перед созданием `RuleSet`
- Модификация bootstrap релея (relay terminator): аналогично
- Refresh: API endpoint в kvn-web + handler в клиенте для перерезолва
- Web UI: структурированные поля для sources в секции Routing
- Android: ignoreUnknownKeys для новых полей (уже есть), кнопка Refresh — отдельная задача

**Нетронуто:** `RuleSet`, `Router`, `CIDRMatcher`, `DomainMatcher`, `tunnel/session.go`, остальной data-path.

## Implementation Surfaces

| Surface | Роль | Тип |
|---------|------|-----|
| `config/client.go` | `SourceRule` тип + `RoutingCfg` новые поля | existing, light modify |
| `routing/resolver.go` (new) | Резолв источников: скачивание/кеш/парсинг/мерж | new |
| `routing/geoip/geoip.pb.go` (gen) | Proto-код для geoip.dat | new (generated) |
| `routing/geoip/geosite.pb.go` (gen) | Proto-код для geosite.dat | new (generated) |
| `routing/geoip/parser.go` (new) | Обёртки для чтения .dat файлов | new |
| `bootstrap/client/client.go` | Вызов resolver перед Run() | existing, light modify |
| `bootstrap/client/proxy.go` | Аналогично для proxy-режима | existing, light modify |
| `bootstrap/client/tun.go` | Аналогично для TUN-режима | existing, light modify |
| `bootstrap/relay/bootstrap.go` | Вызов resolveDirectSources перед newDirectRuleSet | existing, light modify |
| `bootstrap/relay/router.go` | resolveDirectSources() — резолв DirectSources через Resolver | existing, new method |
| `examples/relay-terminator/relay.yaml` | Пример direct_sources | existing, light modify |
| `webui/handler_config.go` | CRUD для SourceRule в Web UI | existing, modify |
| `webui/frontend/` | Карточки sources в секции Routing | existing, modify |
| `config/client_test.go` | Тесты SourceRule десериализации | existing, new tests |
| `routing/resolver_test.go` (new) | Тесты резолва + мержа | new |

## Bootstrapping Surfaces

- `routing/geoip/geoip.pb.go`, `routing/geoip/geosite.pb.go` — сгенерировать через `protoc` до реализации resolver
- `routing/resolver.go` — scaffolding структуры `Resolver` с методами `Resolve(cfg) → merged`, `Refresh() → error`

## Влияние на архитектуру

- **Локальное:** новый пакет `resolver` в `src/internal/routing/`. Никаких изменений вне роутинга и bootstrap.
- **Data model:** `RoutingCfg` получает 4 новых поля. `RelayRoutingCfg` получает `DirectSources` + поля для geoip/geosite путей/URL. Старые поля (`DirectRanges`, `DirectDomains`, etc.) остаются без изменений.
- **Bootstrap (client):** перед `NewRuleSet(cfg.Routing, logger)` — вызов `routing.NewResolver(cfg, logger).Resolve()`, который возвращает смерженный `*RoutingCfg` (статика + раскрытые источники). Этот смерженный конфиг передаётся в `NewRuleSet`.
- **Bootstrap (relay):** перед `newDirectRuleSet()` — вызов `r.resolveDirectSources()`, которая создаёт временный `*RoutingCfg` с полями путей/URL, резолвит `DirectSources` через `Resolver.ResolveSources()`, и мержит результат в `DirectRanges`/`DirectDomains`.
- **Refresh:** клиент хранит ссылку на `Resolver`. Кнопка Refresh вызывает `resolver.Refresh()` → перескачивание баз + перерезолв + `NewRuleSet(mergedCfg)` → атомарная подмена через `atomic.Pointer[RuleSet]` в `TunRouter` / proxy-сессии.
- **Compatibility:** обратная совместимость полная — старые конфиги без `sources` работают как есть. Новые поля — optional.

## Acceptance Approach

| AC | Подход | Surfaces | Валидация |
|----|--------|----------|-----------|
| AC-001 | `SourceRule` YAML/JSON deserialization | `config/client.go` | Unit test |
| AC-002 | Resolver читает geoip.dat, раскрывает CIDR для кода страны | `routing/resolver.go`, `routing/geoip/parser.go` | Unit test с тестовым .dat |
| AC-003 | Resolver скачивает URL, парсит CIDR | `routing/resolver.go` | Unit test с file:// URL |
| AC-004 | Resolver напрямую добавляет CIDR | `routing/resolver.go` | Unit test |
| AC-005 | Merge + dedup статики и источников | `routing/resolver.go` | Unit test |
| AC-006 | Невалидный URL → warning, graceful | `routing/resolver.go` | Unit test + log check |
| AC-007 | TTL кеша: локальный файл < 24h не скачивается | `routing/resolver.go` | Unit test с mock HTTP |
| AC-008 | Resolver читает geosite.dat, раскрывает домены | `routing/resolver.go`, `routing/geoip/parser.go` | Unit test с тестовым .dat |
| AC-009 | нет `geoip_path`/`geoip_url` → debug, пропуск | `routing/resolver.go` | Unit test |
| AC-010 | Web UI отображает карточки sources | `webui/frontend/` | Visual + API-тест |
| AC-011 | Refresh → новый RuleSet, атомарная подмена | `bootstrap/client/*go`, `routing/resolver.go` | Manual + unit test (mock refresh) |

## Данные и контракты

### SourceRule

```go
type SourceRule struct {
    GeoIP   *string `json:"geoip,omitempty" yaml:"geoip,omitempty"`
    GeoSite *string `json:"geosite,omitempty" yaml:"geosite,omitempty"`
    CIDR    *string `json:"cidr,omitempty" yaml:"cidr,omitempty"`
    URL     *string `json:"url,omitempty" yaml:"url,omitempty"`
}

func (s SourceRule) Type() string   // "geoip" | "geosite" | "cidr" | "url" | "invalid"
func (s SourceRule) Value() string // значение соответствующего поля
func (s SourceRule) Valid() bool   // ровно одно поле задано
```

### RoutingCfg — новые поля

```go
type RoutingCfg struct {
    // ... существующие поля без изменений

    GeoIPPath   string       `json:"geoip_path,omitempty" yaml:"geoip_path,omitempty"`       // статика
    GeoSitePath string       `json:"geosite_path,omitempty" yaml:"geosite_path,omitempty"`   // статика
    GeoIPURL    string       `json:"geoip_url,omitempty" yaml:"geoip_url,omitempty"`
    GeoSiteURL  string       `json:"geosite_url,omitempty" yaml:"geosite_url,omitempty"`
    SourceTTL   int          `json:"source_ttl_hours,omitempty" yaml:"source_ttl_hours,omitempty"` // default 24
    IncludeSources []SourceRule `json:"include_sources,omitempty" yaml:"include_sources,omitempty"`
    ExcludeSources []SourceRule `json:"exclude_sources,omitempty" yaml:"exclude_sources,omitempty"`
}
```

### Resolver

```go
type Resolver struct {
    cfg      *RoutingCfg  // исходный конфиг с источниками
    cacheDir string       // директория для geoip.dat / geosite.dat
    logger   *zap.Logger
    mu       sync.Mutex
}

func NewResolver(cfg *RoutingCfg, cacheDir string, logger *zap.Logger) *Resolver
func (r *Resolver) Resolve() (*RoutingCfg, error)
func (r *Resolver) Refresh() (*RoutingCfg, error)
```

### API / contracts

- Нет изменений в протоколе или API — всё локально на клиенте/релее.
- Web UI API: GET/PUT для `routing.include_sources` / `routing.exclude_sources` — те же endpoints, новые поля.
- `data-model.md` прилагается.

## Стратегия реализации

### DEC-001: Resolver — отдельный пакет в `src/internal/routing/`

- **Why**: резолв источников ортогонален матчингу маршрутов. Resolver потребляет `RoutingCfg`, производит смерженный `RoutingCfg`. RuleSet не знает о существовании источников.
- **Tradeoff**: новый пакет вместо расширения существующего. Но это сохраняет RuleSet "чистым" и тестируемым независимо.
- **Affects**: `routing/resolver.go`, `routing/geoip/`
- **Validation**: resolver unit-тесты не требуют RuleSet

### DEC-002: Proto-генерация для .dat (без v2fly-зависимости)

- **Why**: не тащить `github.com/v2fly/domain-list-community` (~50MB+ транзитивных зависимостей). Схема geoip.dat тривиальна (3 proto-сообщения), `.proto` файл занимает 15 строк.
- **Tradeoff**: нужно установить `protoc` + `protoc-gen-go` для генерации. Один раз при разработке, CI может использовать предварительно сгенерированный `.pb.go`.
- **Affects**: `routing/geoip/geoip.pb.go`, `routing/geoip/geosite.pb.go`
- **Validation**: unit-тест читает бинарный .dat через сгенерированный парсер

### DEC-003: Resolve на этапе bootstrap, merge до RuleSet

- **Why**: RuleSet принимает готовые плоские списки. Resolver мержит статику с раскрытыми источниками и отдаёт `*RoutingCfg` с заполненными `IncludeRanges`/`ExcludeRanges`/`IncludeDomains`/`ExcludeDomains`.
- **Tradeoff**: источники раскрываются один раз при старте (или Refresh). Изменение geoip.dat в рантайме не подхватится до Refresh.
- **Affects**: `bootstrap/client/*.go`, `bootstrap/relay/*.go`
- **Validation**: интеграционный тест: config with sources → resolved config → RuleSet → Route() возвращает ожидаемые решения

### DEC-004: Атомарная подмена RuleSet при Refresh

- **Why**: чтобы не рвать текущие соединения. Новый RuleSet создаётся полностью, затем атомарно подменяется ссылка (`atomic.Pointer[RuleSet]`). Старые соединения держат старый RuleSet до завершения.
- **Tradeoff**: сложнее, чем полная перезагрузка. Но необходимо для плавного Refresh без дисконнекта.
- **Affects**: `routing/router.go` (TunRouter), `bootstrap/client/proxy.go` (proxy session)
- **Validation**: unit-тест: старое соединение использует старый RuleSet, новое — новый

## Incremental Delivery

### MVP (Первая ценность)

- SourceRule + RoutingCfg расширение
- Resolver: GeoIP (парсинг .dat + раскрытие) + CIDR + URL
- Merge + dedup с плоскими списками
- Graceful degradation
- Bootstrap модификация (TUN + proxy)
- Unit-тесты
- AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-007, AC-009

### Итеративное расширение

1. **GeoSite** (AC-008, RQ-012): парсер geosite.dat, поддержка `geosite` в SourceRule, тесты. Независим от MVP.
2. **Refresh button** (AC-011, RQ-015): `resolver.Refresh()`, атомарная подмена RuleSet, кнопка в Web UI/Android.
3. **Web UI sources** (AC-010, RQ-013): карточки источников в секции Routing.
4. **Android sources** (RQ-014, RQ-015): кнопка Refresh, отображение sources в Android UI.

## Порядок реализации

1. **Proto-генерация** для geoip.dat / geosite.dat (независимый шаг, можно сразу)
2. **RoutingCfg + SourceRule** — новые поля и тип (config/client.go)
3. **Resolver: GeoIP + CIDR + URL** (resolver.go + parser.go + unit-тесты)
4. **Merge + dedup** в resolver
5. **Bootstrap: интеграция resolver** в client и relay
6. **GeoSite** поддержка (параллельно с UI)
7. **Refresh button** — resolver.Refresh + атомарная подмена
8. **Web UI** — карточки sources и кнопка Refresh
9. **Android** — ignoreUnknownKeys (уже есть), кнопка Refresh

Шаги 1-5 обязательны для MVP. Шаги 6-9 — последовательное расширение.

## Риски

- **Формат geoip.dat изменится в новой версии v2fly.** Mitigation: фиксируем известную proto-схему, парсим строго. Если дата-сет обновится несовместимо — пользователь увидит warning и будет использовать кеш.
- **Размер geosite.dat (больше geoip.dat).** Mitigation: оба файла < 5MB, скачиваются один раз при старте. Для медленных сетей — таймаут 30s.
- **Refresh может создать Race Condition при подмене RuleSet.** Mitigation: `atomic.Pointer`, старый RuleSet живёт пока есть ссылки из активных соединений (GC сам разберётся). Все новые соединения читают через `Load()`.
- **Web UI API изменится.** Mitigation: новые поля возвращаются только если присутствуют в конфиге. Старые конфиги без sources работают без изменений.

## Rollout и compatibility

- Старые конфиги без `*_sources` и `*_url` — без изменений.
- Новые поля в `RoutingCfg` опциональны, zero-value = отключено.
- `ignoreUnknownKeys = true` в конфиге (уже есть) — старые клиенты не упадут при наличии новых полей (хотя и не обработают их).
- Feature flag не требуется.
- После релиза: мониторить логи на warnings от resolver (битые базы, недоступные URL).

## Проверка

| Вид | Что проверяем | AC/DEC |
|-----|--------------|--------|
| Unit test | SourceRule YAML/JOSN десериализация (4 типа + invalid) | AC-001 |
| Unit test | GeoIP раскрытие: тестовый .dat с 3 записями → 3 CIDR | AC-002, DEC-001 |
| Unit test | URL источник: file:// с CIDR → добавлены в exclude_ranges | AC-003 |
| Unit test | CIDR источник: напрямую в include_ranges | AC-004 |
| Unit test | Merge + dedup: статика + источники → без дубликатов | AC-005, DEC-003 |
| Unit test | Graceful: битый URL → warning, не падает | AC-006 |
| Unit test | TTL: свежий файл не перекачивается | AC-007, DEC-002 |
| Unit test | GeoSite раскрытие: тестовый .dat → домены | AC-008 |
| Unit test | Нет geoip_url → error, пропуск | AC-009 |
| Integration | Bootstrap: config → resolved → RuleSet → Route() | DEC-003 |
| Unit test | Atomic подмена RuleSet при Refresh | AC-011, DEC-004 |
| Manual | Запуск клиента с geoip:ru → в логах "resolved N CIDRs" | AC-002 |
| Visual | Web UI: карточки sources отображаются | AC-010 |

## Соответствие конституции

- нет конфликтов: Go 1.22+, DDD, @sk-task/@sk-test на function/method/type declarations, разделение документации
