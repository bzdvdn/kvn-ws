# Configurable DNS Upstreams List — Задачи

## Phase Contract

Inputs: plan.md, data-model.md, spec.md
Outputs: tasks.md с coverage AC-001–AC-010
Stop if: план расплывчат — нет, план полон.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/config/client.go` | T1.1, T1.3 |
| `src/internal/config/server.go` | T1.2 |
| `src/internal/config/webui.go` | T1.3 |
| `src/internal/dnsproxy/dnsproxy.go` | T2.1 |
| `src/internal/tunnel/session.go` | T2.2 |
| `src/internal/bootstrap/client/tun.go` | T3.1 |
| `src/internal/bootstrap/server/handler.go` | T3.2 |
| `src/internal/webui/handler_connect.go` | T3.3 |
| `src/internal/webui/frontend/src/App.tsx` | T3.4 |
| `src/internal/config/*_test.go` | T4.1 |
| `src/internal/dnsproxy/*_test.go` | T4.1 |
| `src/internal/webui/*_test.go` | T4.1 |
| `configs/*.yaml`, `examples/*.yaml` | T4.2 |
| `docs/ru/`, `docs/en/` | T4.2 |

## Implementation Context

- **Цель MVP:** замена `upstream string` → `upstreams []string` во всех компонентах с backward compat и fallback
- **Инварианты/семантика:**
  - Старый `upstream:` маппится в `upstreams[0]`, приоритет у `upstreams` (с warning)
  - `upstreams: []` или null → `DefaultDNSUpstreams`
  - `DefaultDNSUpstreams = ["1.1.1.1:53", "8.8.8.8:53"]` — единый source of truth
  - DNS proxy fallback: перебор upstream по порядку при ошибке dial/read
  - server-side `handleDNSFrame`: берёт первый upstream из конфигурируемого списка
- **Контракты/протокол:**
  - Custom `UnmarshalYAML`/`UnmarshalJSON` на `DNSProxyCfg` (паттерн как `DNSCacheCfg`)
  - `dnsproxy.New(listen string, upstreams ...string)` — varargs, старые вызовы совместимы
  - `tunnel.NewSession` — новый параметр `dnsUpstreams []string` (nil = дефолты)
- **Границы scope:**
  - Не делаем: hot-reload, health-check, graceful fallback, авто-обнаружение DNS
- **Proof signals:**
  - `go test -race ./...` pass
  - `go vet ./...` pass
  - Все 10 AC покрыты тестами
  - WebUI форма отображает список upstreams с add/remove
- **References:** DEC-001 (custom unmarshal), DEC-002 (variadic New), DEC-003 (Session.dnsUpstreams)

## Фаза 1: Конфигурация и data model

Цель: подготовить config-структуры с `Upstreams []string`, backward compat и централизованными дефолтами.

- [x] T1.1 Добавить `DNSProxyCfg.Upstreams []string` + `RelayDNSCfg.Upstreams []string` + `DefaultDNSUpstreams`, custom `UnmarshalYAML`/`UnmarshalJSON` (backward compat для `upstream`), обновить дефолты в `LoadClientConfig`/`LoadRelayConfig`. Touches: `src/internal/config/client.go`
  - AC-001, AC-002, AC-003, AC-007

- [x] T1.2 Добавить `ServerConfig.DNSUpstreams []string` с дефолтом `DefaultDNSUpstreams` в `LoadServerConfig`. Touches: `src/internal/config/server.go`
  - AC-004

- [x] T1.3 Обновить `defaultWebUIConfig()` — замена `Upstream` на `Upstreams`. Touches: `src/internal/config/webui.go`
  - AC-003

## Фаза 2: DNS proxy и server-side forward

Цель: реализовать поддержку списка upstream в dnsproxy и tunnel.Session.

- [x] T2.1 Сделать `dnsproxy.New` variadic (`upstreams ...string`), заменить `upstream string` на `upstreams []string` в `Server`, реализовать fallback-цикл в `forward` (перебор upstream при ошибке). Touches: `src/internal/dnsproxy/dnsproxy.go`
  - AC-005

- [x] T2.2 Добавить поле `dnsUpstreams []string` в `tunnel.Session`, параметр в `NewSession`, использовать в `handleDNSFrame`/`forwardDNS` вместо хардкода `8.8.8.8:53`. Touches: `src/internal/tunnel/session.go`
  - AC-006

## Фаза 3: Интеграция и UI

Цель: пробросить новые поля через bootstrap и webui.

- [x] T3.1 Передать `cfg.DNSProxy.Upstreams` в `dnsproxy.New` в `Client.runSession`. Touches: `src/internal/bootstrap/client/tun.go`
  - AC-001, AC-005

- [x] T3.2 Передать `cfg.DNSUpstreams` в `tunnel.NewSession` в `Server.handleStream`. Touches: `src/internal/bootstrap/server/handler.go`
  - AC-006

- [x] T3.3 Обновить `mergeConfig` в `handler_connect.go` — проверка `Upstreams` вместо `Upstream`, backward compat для старого формата. Touches: `src/internal/webui/handler_connect.go`
  - AC-009, AC-010

- [x] T3.4 Обновить TypeScript тип `dns_proxy` (добавить `upstreams: string[]`, сохранить `upstream?` для backward compat) и форму DNS proxy — заменить одиночный input на список input'ов с add/remove. Touches: `src/internal/webui/frontend/src/App.tsx`
  - AC-008, AC-010

## Фаза 4: Проверка

Цель: доказать, что фича работает, и оставить пакет в reviewable состоянии.

- [x] T4.1 Добавить тесты:
  - `config` пакет: `TestDNSProxyCfgUpstreams` (AC-001), `TestDNSProxyCfgBackwardCompat` (AC-002), `TestDNSProxyCfgDefaults` (AC-003), `TestServerDNSUpstreams` (AC-004), `TestRelayDNSUpstreamBackwardCompat` (AC-007)
  - `dnsproxy` пакет: `TestDNSProxyFallback` (AC-005) — mock upstream
  - `webui` пакет: `TestMergeConfigDNSUpstreams` (AC-009), `TestWebUIDNSUpstreamBackwardCompat` (AC-010)
  - Интеграционный тест `TestServerDNSForwardUsesConfig` (AC-006)
  - Touches: `src/internal/config/client_test.go`, `src/internal/dnsproxy/dnsproxy_test.go`, `src/internal/webui/multiserver_test.go`

- [x] T4.2 Обновить YAML-примеры (`configs/*.yaml`, `examples/*.yaml`) — замена `upstream` на `upstreams`. Обновить документацию `docs/ru/`, `docs/en/` — описание нового поля. Touches: `configs/client.yaml`, `configs/server.yaml`, `examples/client.yaml`, `docs/ru/`, `docs/en/`
  - AC-001, AC-004

## Покрытие критериев приемки

- AC-001 → T1.1, T3.1, T4.1, T4.2
- AC-002 → T1.1, T4.1
- AC-003 → T1.1, T1.3, T4.1
- AC-004 → T1.2, T4.1, T4.2
- AC-005 → T2.1, T3.1, T4.1
- AC-006 → T2.2, T3.2, T4.1
- AC-007 → T1.1, T4.1
- AC-008 → T3.4, T4.1
- AC-009 → T3.3, T4.1
- AC-010 → T3.3, T3.4, T4.1

## Заметки

- T1.1–T1.3 независимы, могут параллелиться
- T2.1–T2.2 зависят от T1.1
- T3.1–T3.4 зависят от T1.1–T1.2 и T2.1–T2.2
- T4.1 пишется параллельно с T2–T3
- Trace-маркеры `@sk-task dns-upstreams-list#T*.*` ставить над owning function/method/test
- Проверка: `go test -race ./...` + `go vet ./...`

---

## End Block

- **Slug:** dns-upstreams-list
- **Status:** implement: done
- **Artifacts:**
  - `src/internal/config/client.go` — DNSProxyCfg, RelayDNSCfg, DefaultDNSUpstreams, custom marshal/unmarshal, LoadClientConfig/LoadRelayConfig migration
  - `src/internal/config/server.go` — ServerConfig.DNSUpstreams, LoadServerConfig defaults
  - `src/internal/config/webui.go` — defaultWebUIConfig Upstreams
  - `src/internal/dnsproxy/dnsproxy.go` — variadic New, upstreams slice, fallback in forward
  - `src/internal/tunnel/session.go` — dnsUpstreams field, NewSession param, forwardDNS fallback
  - `src/internal/bootstrap/client/tun.go` — pass Upstreams to dnsproxy.New
  - `src/internal/bootstrap/server/handler.go` — pass DNSUpstreams to tunnel.NewSession
  - `src/internal/webui/handler_connect.go` — mergeConfig checks Upstreams
  - `src/internal/webui/frontend/src/App.tsx` — upstreams UI with add/remove
  - `src/internal/config/client_test.go` — DNSProxyCfg tests
  - `src/internal/config/config_test.go` — ServerDNSUpstreams tests
  - `src/internal/dnsproxy/dnsproxy_test.go` — fallback + variadic tests
  - `src/internal/webui/multiserver_test.go` — merge + roundtrip tests
  - `src/internal/tunnel/session_test.go` — forwardDNS config test
  - `docs/en/config.md` — updated dns_proxy.upstreams field
  - `docs/ru/config.md` — обновлено поле dns_proxy.upstreams
- **Blockers:** none
- **Готово к:** `/speckeep.verify dns-upstreams-list`
