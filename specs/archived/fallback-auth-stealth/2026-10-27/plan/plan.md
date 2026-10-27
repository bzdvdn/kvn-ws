# Stealth fallback и Bearer-auth до upgrade — План

## Phase Contract

Inputs: `spec.md`, текущие server/client config, WS dial/accept, webui/Android поверхности.
Outputs: `plan.md`; data-path и padding не меняются.

## Цель

Добавить opt-in stealth path: Bearer проверяется до upgrade, неавторизованные запросы получают fallback-сайт, legacy path остаётся нетронутым. Клиент пробует stealth path и откатывается на legacy без лишнего RTT.

## MVP Slice

- Сервер: stealth path + Bearer + fallback + legacy.
- Клиент: Bearer на upgrade + fallback на legacy.
- Закрывает AC-001, AC-002, AC-003, AC-005, AC-006.

## First Validation Path

1. `go test -race ./src/internal/bootstrap/server/ ./src/internal/bootstrap/client/ ./src/integration/`.
2. `curl` без токена на stealth path → fallback HTML; с токеном → 101.
3. Старый клиент на legacy path — без изменений.

## Scope

- `config.ServerConfig`: `stealth_path`, `fallback.upstream`, `fallback.title`, `stealth_hide_admin`.
- `config.ClientConfig`: `stealth{enabled,path,legacy_path}`.
- Server mux/auth/fallback; client dial/fallback; WS `Authorization`.
- WebUI client settings; Android fields/Bearer/fallback.
- Tests/benchmark.

## Performance Budget

- Stealth handshake latency ≤ legacy +5%.
- Ноль дополнительных RTT: `Authorization` в том же upgrade-запросе.
- Data-path/padding/allocs не меняются.

## Implementation Surfaces

- `src/internal/config/server.go` — stealth/fallback поля, defaults/validation.
- `src/internal/config/client.go` — `StealthCfg`, defaults.
- `src/internal/bootstrap/server/server.go` — stealth route, fallback, hide-health.
- `src/internal/bootstrap/server/handler.go` — Bearer check before upgrade, pre-auth stream, fallback helpers.
- `src/internal/transport/transport.go`, `websocket/websocket.go` — `Authorization` header на Dial.
- `src/internal/bootstrap/client/dial.go` — stealth URL/Bearer + fallback на legacy.
- `src/internal/webui/frontend/src/types.ts`, `TabbedForm.tsx`, `handler_connect.go` — client settings/merge.
- `src/android/.../config/AppConfig.kt`, `transport/WebSocketClient.kt`, `vpn/KvnVpnService.kt`, UI/QR — parity.
- Tests: server handler, client dial, integration, webui, android.

## Bootstrapping Surfaces

- none: mux/dial/config уже есть.

## Влияние на архитектуру

- Появляется вторая WS-поверхность: legacy (ClientHello auth) и stealth (Bearer pre-auth).
- Невалидный stealth-запрос не создаёт session и не пишет tunnel-ошибки.
- Fallback — обычный HTTP/HTTPS сайт на том же сертификате.
- Legacy path остаётся до миграции; затем отключается конфигом.

## Acceptance Approach

- AC-001 → server handler: Bearer valid → 101 + session.
- AC-002 → handler: invalid/missing → fallback, no 401/101.
- AC-003 → существующие handler/integration + mixed-version.
- AC-004 → httptest fake upstream: HTML/JSON, Authorization stripped, 502 on failure.
- AC-005 → benchmark/integration Dial legacy vs stealth.
- AC-006 → client unit: stealth fails → legacy dial.
- AC-007 → config/webui/android roundtrip + log capture.
- AC-008 → httptest external vs loopback RemoteAddr.
- AC-009 → Android unit/integration.

## Данные и контракты

- Data model: no change.
- Wire: data-path не меняется; добавляется HTTP `Authorization` (внутри TLS) и новый path.
- Server config defaults: `stealth_path=""` (off), fallback title empty, hide-admin true when stealth path set.
- Client defaults: `stealth.enabled=false`, paths empty (legacy behavior).
- `contracts/*` и `data-model.md` не создаются.

## Стратегия реализации

- DEC-001 Dual-path, opt-in
  Why: нельзя ломать старых клиентов; stealth path — отдельная поверхность.
  Tradeoff: legacy path остаётся fingerprint, пока оператор его не отключит.
  Affects: server mux/config, client dial/config.
  Validation: AC-003, mixed-version.

- DEC-002 Bearer до upgrade + constant-time compare
  Why: неавторизованный запрос должен получить fallback, а не tunnel-ошибку.
  Tradeoff: токен в стандартном заголовке (внутри TLS); нужен запрет логирования.
  Affects: `handler.go`, config/auth.
  Validation: AC-001/002/007.

- DEC-003 Fallback static/reverse-proxy с очисткой Authorization
  Why: prober должен видеть реальный сайт; upstream не должен получить токен.
  Tradeoff: нужен реальный домен/сайт; upstream — доверенный.
  Affects: `server.go` fallback, config.
  Validation: AC-004.

- DEC-004 Без дополнительного RTT
  Why: скрытность не должна ухудшать скорость.
  Tradeoff: Bearer идёт в upgrade-запросе, а не отдельным запросом.
  Affects: client dial/transport.
  Validation: AC-005 benchmark.

- DEC-005 Клиентский fallback на legacy path
  Why: новый клиент должен работать со старым сервером.
  Tradeoff: при ошибке stealth path выполняется второй dial.
  Affects: `dial.go`.
  Validation: AC-006.

- DEC-006 Health/admin скрыты в stealth-режиме
  Why: `/healthz`, `/metrics`, pprof выдают инфраструктуру.
  Tradeoff: loopback-доступ сохраняется для оператора.
  Affects: server mux.
  Validation: AC-008.

- DEC-007 Relay вне scope
  Why: bridge/terminator — отдельная поверхность; сначала client↔server.
  Tradeoff: stealth path через relay не гарантируется в этой фиче.
  Affects: none.
  Validation: docs/out-of-scope.

## Incremental Delivery

### MVP

- Server stealth path/Bearer/fallback + legacy; client Bearer/fallback.
- Готовность: AC-001..AC-006.

### Расширение

- Fallback upstream, hide-health, webui, Android, benchmark.
- Готовность: AC-007..AC-009.

## Порядок реализации

1. Config server/client + validation/defaults.
2. Server Bearer/fallback/legacy + tests.
3. Transport Authorization header + client fallback + tests.
4. Hide health/admin + fallback proxy tests.
5. WebUI settings/merge.
6. Android parity.
7. Docs/CHANGELOG + benchmark + gate.

## Риски

- Токен может попасть в fallback upstream или логи.
  Mitigation: strip `Authorization`, log tests, no token in error strings.
- Fallback upstream = SSRF/open proxy.
  Mitigation: только фиксированный оператором upstream, без user-controlled URL.
- Legacy path остаётся fingerprint.
  Mitigation: docs + возможность отключить legacy path после миграции.
- Новый клиент зациклится на fallback.
  Mitigation: один fallback на legacy, без повторов.
- Неверный path/сертификат ломает fallback.
  Mitigation: config validation + clear errors.

## Rollout и compatibility

- Сервер обновляется первым: stealth path + legacy одновременно.
- Клиенты включают `stealth.enabled` opt-in; старые не затронуты.
- Android обновляется после Go/kvn-web; старый APK работает через legacy.
- После миграции legacy path можно отключить.
- Мониторинг: доля fallback-ответов, auth failures, latency stealth/legacy.

## Проверка

- Unit: Bearer valid/invalid, fallback static/upstream, hide-health, client fallback.
- Integration: stealth handshake, mixed-version, data path.
- Benchmark: Dial latency legacy vs stealth; no data-path alloc regression.
- Commands: `go test -race ./src/internal/bootstrap/server/ ./src/internal/bootstrap/client/ ./src/internal/transport/websocket/ ./src/integration/`, `go vet ./...`, Android `./gradlew test assembleDebug`.
- Покрывает AC-001–AC-009, DEC-001–DEC-007.

## Соответствие конституции

- нет конфликтов: TLS 1.3, стандартный Authorization, токены не логируются, data-path не меняется, trace-маркеры на новых декларациях.
