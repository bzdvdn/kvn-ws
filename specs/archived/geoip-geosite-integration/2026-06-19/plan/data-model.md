# Data Model: geoip-geosite-integration

## Status

- `changed` — `RoutingCfg` расширяется источниками; новый тип `SourceRule`; новый пакет `Resolver`.

## Текущая модель (до изменений)

```go
type RoutingCfg struct {
    DefaultRoute   string
    IncludeRanges  []string
    ExcludeRanges  []string
    IncludeIPs     []string
    ExcludeIPs     []string
    IncludeDomains []string
    ExcludeDomains []string
}
```

Все списки — плоские `[]string`. Источники не поддерживаются.

## Новая модель (после изменений)

```go
// SourceRule — один источник правил. Ровно одно поле должно быть задано.
type SourceRule struct {
    GeoIP   *string `json:"geoip,omitempty" yaml:"geoip,omitempty"`
    GeoSite *string `json:"geosite,omitempty" yaml:"geosite,omitempty"`
    CIDR    *string `json:"cidr,omitempty" yaml:"cidr,omitempty"`
    URL     *string `json:"url,omitempty" yaml:"url,omitempty"`
}

type RoutingCfg struct {
    // существующие поля (без изменений)
    DefaultRoute   string
    IncludeRanges  []string
    ExcludeRanges  []string
    IncludeIPs     []string
    ExcludeIPs     []string
    IncludeDomains []string
    ExcludeDomains []string

    // новые поля
    GeoIPPath      string       `json:"geoip_path,omitempty"`       // статический путь, без автообновления
    GeoSitePath    string       `json:"geosite_path,omitempty"`     // статический путь, без автообновления
    GeoIPURL       string       `json:"geoip_url,omitempty"`        // URL для скачивания (если geoip_path не указан)
    GeoSiteURL     string       `json:"geosite_url,omitempty"`      // URL для скачивания (если geosite_path не указан)
    SourceTTL      int          `json:"source_ttl_hours,omitempty"`  // default 24
    IncludeSources []SourceRule `json:"include_sources,omitempty"`
    ExcludeSources []SourceRule `json:"exclude_sources,omitempty"`
}
```

## Resolver

```go
type Resolver struct {
    cfg      *RoutingCfg
    cacheDir string
    logger   *zap.Logger
    mu       sync.Mutex
}

func NewResolver(cfg *RoutingCfg, cacheDir string, logger *zap.Logger) *Resolver
func (r *Resolver) Resolve() (*RoutingCfg, error)
func (r *Resolver) Refresh() (*RoutingCfg, error)
```

- `Resolve()`: проходит `IncludeSources`/`ExcludeSources`, раскрывает каждый source в CIDR или домены, смерживает со статическими списками, возвращает новый `*RoutingCfg` с заполненными плоскими списками.
- `Refresh()`: перескачивает базы (если есть URL), затем вызывает `Resolve()`.

## Сериализация

- `SourceRule` использует `omitempty` — пустые поля не сериализуются.
- Валидация: `len(SourceRule.nonNilFields()) == 1` — иначе warning, source игнорируется.
- Формат URL-списка: одна запись на строку. Строки с `#` в начале — комментарии. Пустые строки игнорируются. Запись с `/` — CIDR, без `/` — домен.

## Схема proto (geoip.dat)

```protobuf
message GeoIPList { repeated GeoIP entry = 1; }
message GeoIP {
  string country_code = 1;
  repeated CIDR cidr = 2;
}
message CIDR { bytes ip = 1; uint32 prefix = 2; }
```

## Схема proto (geosite.dat)

```protobuf
message GeoSiteList { repeated GeoSite entry = 1; }
message GeoSite {
  string category_code = 1;
  repeated Domain domain = 2;
}
message Domain {
  string value = 1;
  Type type = 2;
  enum Type { Plain = 0; Regex = 1; Domain = 2; Full = 3; }
}
```

## Затрагиваемые файлы

- `config/client.go` — `SourceRule` тип, `RoutingCfg` новые поля
- `routing/resolver.go` (new) — резолв источников
- `routing/geoip/geoip.pb.go` (new, generated) — proto GeoIPList
- `routing/geoip/geosite.pb.go` (new, generated) — proto GeoSiteList
- `routing/geoip/parser.go` (new) — обёртки для чтения .dat
- `routing/router.go` — atomic.Pointer[RuleSet] для Refresh (опционально)
- `config/client_test.go` — тесты SourceRule десериализации
- `routing/resolver_test.go` (new) — тесты резолва
