# DNS Upstreams List — План

## Phase Contract

Inputs: spec, repo map, ключевые поверхности (config, dnsproxy, tunnel session, bootstrap).
Outputs: plan.md, data-model.md.
Stop if: spec расплывчата — нет, spec полна.

## Цель

Форма реализации замены единственного `upstream` (string) на `upstreams` ([]string) во всех компонентах:
клиент, сервер, relay, web-ui, dnsproxy. Backward compatibility — старый `upstream` маппится в `upstreams[0]`.

## MVP Slice

1. `DNSProxyCfg.Upstreams []string` + custom unmarshal (backward compat) — покрывает AC-001, AC-002, AC-003
2. `ServerConfig.DNSUpstreams []string` — покрывает AC-004
3. `dnsproxy.Server` принимает список, fallback по порядку — покрывает AC-005
4. `Session.handleDNSFrame` использует конфигурируемые upstreams — покрывает AC-006
5. `RelayDNSCfg.Upstreams` + backward compat — покрывает AC-007

## First Validation Path

```bash
go test -run "TestDNSProxyCfgUpstreams|TestDNSProxyCfgBackwardCompat|TestDNSProxyCfgDefaults|TestServerDNSUpstreams|TestDNSProxyFallback|TestServerDNSForwardUsesConfig|TestRelayDNSUpstreamBackwardCompat" ./src/internal/config/... ./src/internal/dnsproxy/... ./src/internal/tunnel/...
```

## Scope

- `config.DNSProxyCfg`: замена `Upstream string` → `Upstreams []string` + custom unmarshal
- `config.RelayDNSCfg`: та же замена
- `config.ServerConfig`: добавление `DNSUpstreams []string`
- `config.defaultWebUIConfig()`: обновление дефолта
- `dnsproxy.Server`: `upstream string` → `upstreams []string`, variadic `New()`, fallback-цикл
- `tunnel.Session`: поле `dnsUpstreams []string`, проброс из конфига, `handleDNSFrame`/`forwardDNS` без хардкода
- `bootstrap/client/tun.go`: передача `Upstreams` в `dnsproxy.New`
- `bootstrap/server/handler.go`: передача `DNSUpstreams` в `tunnel.NewSession`
- WebUI фронтенд `App.tsx`: TypeScript тип `dns_proxy` (add `upstreams: string[]`)
- WebUI фронтенд `App.tsx`: форма DNS proxy — список input'ов с add/remove
- WebUI `handler_connect.go`: обновление `mergeConfig` — проверка `Upstreams` вместо `Upstream`
- YAML-примеры (`configs/*.yaml`, `examples/*.yaml`): обновление
- Документация (`docs/ru/`, `docs/en/`): обновление описания полей

- Out of scope: hot-reload, health-check, graceful fallback (простой перебор по порядку)

## Performance Budget

- none — список upstream итерируется только при недоступности первого, число upstream типично 1–3

## Implementation Surfaces

| Surface | Файл | Статус | Почему участвует |
|---|---|---|---|
| `DNSProxyCfg` | `src/internal/config/client.go` | существующая | core config struct для client/relay |
| `RelayDNSCfg` | `src/internal/config/client.go` | существующая | relay DNS config |
| `ServerConfig` | `src/internal/config/server.go` | существующая | server config (новое поле) |
| `Server` (dnsproxy) | `src/internal/dnsproxy/dnsproxy.go` | существующая | сам DNS proxy |
| `Session` (tunnel) | `src/internal/tunnel/session.go` | существующая | server-side DNS forward |
| `Client.runSession` | `src/internal/bootstrap/client/tun.go` | существующая | создание dnsproxy |
| `Server.handleStream` | `src/internal/bootstrap/server/handler.go` | существующая | создание tunnel.Session |
| `defaultWebUIConfig` | `src/internal/config/webui.go` | существующая | дефолты webui |
| WebUI dns_proxy merge | `src/internal/webui/handler_connect.go` | существующая | mergeConfig для Upstreams |
| WebUI TypeScript types | `src/internal/webui/frontend/src/App.tsx` | существующая | интерфейс dns_proxy |
| WebUI DNS form UI | `src/internal/webui/frontend/src/App.tsx` | существующая | список upstreams с add/remove |

## Bootstrapping Surfaces

- none — все изменения в существующих файлах

## Влияние на архитектуру

- Локальное: расширение полей struct, custom unmarshal, variadic constructor
- Интеграции: `NewSession` получает новый параметр (ломающий API), но вызывается только в `handler.go` и `tun.go` — мигрируется за один коммит
- Migration: старый `upstream:` продолжает работать; при одновременном указании `upstreams` приоритет у `upstreams` + warning

## Acceptance Approach

- AC-001: `TestDNSProxyCfgUpstreams` — unit-тест в `config` пакете. surfaces: `DNSProxyCfg`, custom unmarshal
- AC-002: `TestDNSProxyCfgBackwardCompat` — unit-тест в `config` пакете. surfaces: `DNSProxyCfg` old→new map
- AC-003: `TestDNSProxyCfgDefaults` — unit-тест в `config` пакете. surfaces: `LoadClientConfig` defaults
- AC-004: `TestServerDNSUpstreams` — unit-тест в `config` пакете. surfaces: `ServerConfig.DNSUpstreams`
- AC-005: `TestDNSProxyFallback` — unit-тест в `dnsproxy` пакете (с mock upstream). surfaces: `dnsproxy.Server.forward`
- AC-006: `TestServerDNSForwardUsesConfig` — integration-style тест. surfaces: `Session.handleDNSFrame`
- AC-007: `TestRelayDNSUpstreamBackwardCompat` — unit-тест в `config` пакете. surfaces: `RelayDNSCfg`
- AC-008: `TestWebUIDNSUpstreamsRoundtrip` — integration-тест в `webui` пакете. surfaces: handler_connect.go, App.tsx
- AC-009: `TestMergeConfigDNSUpstreams` — unit-тест в `webui` пакете. surfaces: mergeConfig
- AC-010: `TestWebUIDNSUpstreamBackwardCompat` — unit-тест в `webui` пакете. surfaces: handler_connect.go

## Данные и контракты

- data-model.md: создаётся (описывает `Upstreams []string` + backward compat)
- Изменений API/event contracts нет
- dnsproxy constructor: `New(listen, upstream string)` → `New(listen string, upstreams ...string)` — varargs, обратно совместим на уровне Go source, старые вызовы `New(addr, "x:53")` продолжают работать
- `tunnel.NewSession` — новый параметр `dnsUpstreams []string` (ломающий) — оба call site (handler.go:190, tun.go:430) обновляются

## Стратегия реализации

### DEC-001: Custom unmarshal для backward compat

Why: `DNSProxyCfg` не может просто заменить `Upstream string` на `Upstreams []string` — старые конфиги перестанут парситься. Custom `UnmarshalYAML`/`UnmarshalJSON` (по аналогии с `DNSCacheCfg`) позволяет принять оба формата. Альтернатива: viper pre-processing — хрупко и непрозрачно.

Tradeoff: custom unmarshal — дополнительный код; но паттерн уже есть в `DNSCacheCfg`.

Affects: `DNSProxyCfg`, `RelayDNSCfg`

Validation: `TestDNSProxyCfgBackwardCompat` — старый YAML с `upstream: "1.1.1.1:53"` даёт `Upstreams=["1.1.1.1:53"]`

### DEC-002: Variadic `New()` для dnsproxy

Why: не менять существующие call site, которые передают один upstream. `New(addr, upstreams ...string)` — если не передавать upstreams, используются дефолты.

Tradeoff: минимальный.

Affects: `dnsproxy.New`, `dnsproxy.Server.upstreams`

Validation: `TestDNSProxyFallback` — создание `New(":53", "10.0.0.1:53", "1.1.1.1:53")`

### DEC-003: Session.dnsUpstreams — поле-срез вместо хардкода

Why: убрать `"8.8.8.8:53"` из `handleDNSFrame` без усложнения API Session. Поле инициализируется в конструкторе.

Tradeoff: `NewSession` получает ещё один nil-able параметр (7-й по счёту, но nullable — nil = fallback к `DefaultDNSUpstreams`).

Affects: `tunnel.Session`, `tunnel.NewSession`, `bootstrap/server/handler.go`

Validation: `TestServerDNSForwardUsesConfig` — интеграционный тест

## Incremental Delivery

### MVP (Первая ценность)

- `DNSProxyCfg.Upstreams`, `ServerConfig.DNSUpstreams`, `RelayDNSCfg.Upstreams` — config changes
- `dnsproxy.New` variadic + fallback
- `Session.handleDNSFrame` — конфигурируемый upstream
- Обновление `tun.go` и `handler.go` — передача новых полей
- Тесты: AC-001–AC-007
- Критерий: `go test -run "TestDNSProxy|TestServerDNS|TestRelayDNS" ./...` pass

## Порядок реализации

1. `src/internal/config/client.go`: `DNSProxyCfg`, `RelayDNSCfg`, `DefaultDNSUpstreams`, `LoadClientConfig`/`LoadRelayConfig` defaults
2. `src/internal/config/server.go`: `ServerConfig.DNSUpstreams`, `LoadServerConfig` defaults
3. `src/internal/config/webui.go`: обновление `defaultWebUIConfig`
4. `src/internal/dnsproxy/dnsproxy.go`: variadic `New`, fallback-цикл в `forward`
5. `src/internal/tunnel/session.go`: `dnsUpstreams` field, `NewSession` param, `handleDNSFrame`/`forwardDNS`
6. `src/internal/bootstrap/client/tun.go`: передача `Upstreams`
7. `src/internal/bootstrap/server/handler.go`: передача `DNSUpstreams`
8. `src/internal/webui/handler_connect.go`: обновление `mergeConfig` — проверка `Upstreams`
9. `src/internal/webui/frontend/src/App.tsx`: TypeScript `dns_proxy` тип + UI список upstreams
10. Тесты (можно параллелить с 4–9)
11. YAML-примеры и документация

П.1–3 независимы и могут параллелиться.
П.4–7 зависят от config (1–3).
Тесты (10) можно писать параллельно с 4–9.

## Риски

- **Риск 1:** Забыть обновить `defaultWebUIConfig` — WebUI создаст конфиг со старым `Upstream`.
  Mitigation: тест `TestDNSProxyCfgDefaults` проверяет и `LoadClientConfig`, и `defaultWebUIConfig`.

- **Риск 2:** Новый параметр `dnsUpstreams` в `NewSession` — ломающий change.
  Mitigation: только 2 call site в кодовой базе, оба идентифицированы; обновляются в одном коммите.

## Rollout и compatibility

- Старый `upstream:` → `upstreams[0]` (warn если оба)
- `upstreams: []` или `null` → дефолты
- Приоритет: `upstreams` > `upstream` (с warning)
- Специальных rollout-действий не требуется

## Проверка

- Unit-тесты в `config`: AC-001, AC-002, AC-003, AC-004, AC-007
- Unit-тест в `dnsproxy`: AC-005 (mock upstream)
- Integration-тест: AC-006
- `go test -race ./...` — все существующие тесты проходят
- `go vet ./...` — без ошибок

## Соответствие конституции

- нет конфликтов

---

## End Block

- **Slug:** dns-upstreams-list
- **Status:** plan: ready
- **Artifacts:**
  - `specs/active/dns-upstreams-list/plan.md`
  - `specs/active/dns-upstreams-list/data-model.md`
- **Blockers:** none
- **Готово к:** `/speckeep.tasks dns-upstreams-list`
