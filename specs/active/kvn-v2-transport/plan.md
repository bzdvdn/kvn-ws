# KVN v2: TCP-first stealth transport — План

## Phase Contract

Inputs: `spec.md`, текущие config/routing/DNS/TUN/NAT/webui.
Outputs: `plan.md`; v2 несовместим с v1, v1 остаётся отдельно.

## Цель

Сделать новый v2 data plane: TLS 1.3 + HTTP/2 + per-flow streams, TCP split-connection, stealth по умолчанию и минимальный UI. Не пытаться сохранить v1 wire-совместимость.

## MVP Slice

- Go v2 server/client: HTTP/2 streams, Bearer, TCP split, fallback.
- Минимальный multi-server web UI.
- Закрывает AC-001, AC-002, AC-003, AC-005, AC-007.

## First Validation Path

1. `go test -race` v2 packages.
2. Локальный v2 server/client: TCP target через stream, без inner TCP в pcap.
3. UI: добавить 2 сервера, выбрать, подключиться.
4. Fallback: запрос без Bearer → сайт.

## Scope

- v2 protocol/session/transport packages (new, parallel to v1).
- Нативный HTTP/2 bidirectional streaming per TCP flow (gRPC/Connect — опционально позже).
- Userspace TCP termination (gVisor) or OS transparent proxy fallback.
- UDP MVP best-effort stream; H3 datagrams phase 2.
- Stealth default: standard path/Bearer/fallback, no legacy path.
- Minimal primary UI + multi-server; Advanced (routing/DNS/TLS/proxy) доступен.
- Benchmarks/DPI checks.

## Performance Budget

- p95 TCP latency ≤ v1 на том же канале.
- Throughput ≥ v1; allocs/op не растут.
- Per-flow stream queue bounded; backpressure, не drop.
- Outer TCP: BBR + fq_codel; 2–4 соединения для снижения общего HOL.

## Implementation Surfaces

- `src/internal/v2/protocol/` — protobuf/Connect messages, stream frames.
- `src/internal/v2/transport/` — HTTP/2 client/server, Bearer, TLS/ALPN, fallback.
- `src/internal/v2/tcpstack/` — gVisor netstack/tun2socks adapter, flow ↔ stream mapping.
- `src/internal/v2/session/` — flow lifecycle, stream limits, backpressure.
- `src/internal/v2/client/`, `src/internal/v2/server/` — bootstrap, config, TUN/proxy.
- `src/internal/config/` — v2 config schema, multi-server, defaults.
- `src/internal/webui/` — v2 UI: multi-server, 3 поля в primary, Advanced-вкладки Routing/DNS/TLS/Proxy.
- `src/cmd/web/`, `src/cmd/server/` — v2 entrypoints (separate binaries or `--v2`).
- Tests/bench/DPI.

## Bootstrapping Surfaces

- New `src/internal/v2/...` packages; existing v1 packages не переписываем.
- Reuse config patterns, routing, DNS, TUN, NAT, webui shell.

## Влияние на архитектуру

- v2 — отдельный стек, не эволюция v1.
- TCP flows больше не идут raw IP через TUN; TUN используется как вход в userspace TCP stack.
- UDP остаётся raw IP/TUN в MVP; позже H3 datagrams.
- v1 и v2 работают на разных endpoints/серверах; общий код только инфраструктурный.
- UI v2 не показывает padding/multiplex/secondary/stealth-toggle, но сохраняет routing/DNS/TLS/proxy в Advanced.

## Acceptance Approach

- AC-001 → gVisor flow termination + HTTP/2 stream; integration + pcap.
- AC-002 → two flows, slow target; per-stream flow control test.
- AC-003 → httptest fallback; no legacy/custom headers.
- AC-004 → ClientHello/JA3/JA4 capture series; one fixed profile.
- AC-005 → frontend build + UI test; minimal fields.
- AC-006 → benchmark v1 vs v2.
- AC-007 → mixed deployment test (v1 stack untouched).
- AC-008 → UDP best-effort + H3 feature flag test.
- AC-009 → log/metric test.

## Данные и контракты

- Data model: new v2 config schema; v1 schema не меняется.
- Wire contract: v2 отдельный, несовместим с v1.
- Auth: standard `Authorization: Bearer` before stream; no ClientHello token.
- TLS: TLS 1.3, ALPN `h2`; один фиксированный профиль (uTLS Chrome или Go TLS, решается в T1).
- `contracts/*` — v2 protobuf/Connect contract.

## Стратегия реализации

- DEC-001 TCP split через gVisor netstack
  Why: убирает TCP-in-TCP; userspace stack даёт контроль над flow lifecycle.
  Tradeoff: CPU/RAM, сложность; на слабых устройствах — OS transparent proxy fallback.
  Affects: `v2/tcpstack`, client TUN, server target dial.
  Validation: pcap + latency/retransmit benchmark.

- DEC-002 HTTP/2 bidirectional streams per TCP flow
  Why: стандартный мультиплексированный протокол, flow control, правдоподобный HTTP2-вид. Нативный HTTP/2 streaming проще gRPC/Connect и не требует protobuf для MVP; gRPC/Connect можно добавить позже.
  Tradeoff: один TCP HOL на внешнем соединении; gRPC-совместимость не гарантируется.
  Affects: `v2/protocol`, `v2/transport`.
  Validation: AC-001/002/006.

- DEC-003 2–4 внешних HTTP/2 соединения с flow affinity
  Why: потеря одного TCP не должна вешать все потоки.
  Tradeoff: больше handshake/ресурсов; сложнее балансировка.
  Affects: client connection pool, server listener.
  Validation: loss/HOL test.

- DEC-004 UDP MVP best-effort, H3 datagrams phase 2
  Why: TCP-only сеть — приоритет; нельзя обещать надёжность UDP поверх TCP.
  Tradeoff: real-time UDP в MVP будет страдать; это явно в логах/UI.
  Affects: UDP path, config.
  Validation: AC-008.

- DEC-005 Stealth по умолчанию
  Why: пользователь не должен включать stealth вручную.
  Tradeoff: нужен реальный домен/сертификат/fallback; без них v2 не production.
  Affects: server config/mux, client URL, TLS profile.
  Validation: AC-003/004.

- DEC-006 Один фиксированный TLS-профиль
  Why: случайная смена браузера — аномалия.
  Tradeoff: меньше гибкости; профиль нужно обновлять при изменениях браузеров.
  Affects: `v2/transport`, uTLS config.
  Validation: AC-004.

- DEC-007 UI: 3 поля в primary + Advanced-вкладки
  Why: primary flow должен быть простым, но routing/DNS/TLS/proxy — важные фичи VPN и не должны исчезать.
  Tradeoff: UI остаётся насыщенным; нужно аккуратно сгруппировать настройки, а не скрывать их.
  Affects: webui frontend, v2 config.
  Validation: AC-005.

- DEC-008 v1 не трогаем
  Why: старые серверы и клиенты должны работать.
  Tradeoff: две кодовые базы/бинарника на время миграции.
  Affects: cmd entrypoints, deploy.
  Validation: AC-007.

- DEC-009 Android через Cronet/gRPC — отдельная фаза
  Why: OkHttp fingerprint слабее; Cronet даёт Chrome H2/H3.
  Tradeoff: зависимость от Cronet, отдельный объём.
  Affects: Android module.
  Validation: Android AC позже.

## Incremental Delivery

### MVP (Go)

- v2 protocol/transport, split TCP, stealth fallback, minimal UI.
- Готовность: AC-001, AC-002, AC-003, AC-005, AC-007.

### Phase 2

- UDP best-effort + H3 datagrams, multi-connection tuning, benchmark.
- Готовность: AC-006, AC-008, AC-009.

### Phase 3

- Android (Cronet/gRPC), relay v2, CDN/ECH ops.

## Порядок реализации

1. v2 config schema + entrypoints (parallel binaries).
2. Protocol/transport skeleton: HTTP/2 + Bearer + fallback.
3. gVisor TCP stack + per-flow streams.
4. Client TUN/proxy wiring.
5. Minimal multi-server UI.
6. UDP best-effort.
7. Benchmark/DPI checks.
8. H3/Android/relay.

## Риски

- gVisor CPU/RAM на 1 vCPU/1GB.
  Mitigation: benchmark; OS-level transparent proxy fallback.
- HTTP/2 TCP HOL.
  Mitigation: 2–4 connections + flow affinity; H3 later.
- gRPC/protobuf overhead.
  Mitigation: streaming, buffer reuse, benchmark.
- TLS/ALPN fingerprint.
  Mitigation: fixed uTLS profile, DPI lab.
- UI simplification ломает привычные настройки.
  Mitigation: advanced hidden, not removed.
- Миграция v1/v2.
  Mitigation: separate endpoints/servers; no auto-downgrade.

## Rollout и compatibility

- v2 server/client — отдельные бинарники/endpoints.
- v1 серверы остаются; v1 клиенты не обновляются принудительно.
- Пользователь добавляет v2-серверы в UI и постепенно переводит устройства.
- v2-клиент не должен молча подключаться к v1 endpoint: явная ошибка/подсказка.
- Monitoring: stream errors, flow count, p95, fallback rate.

## Проверка

- Unit: stream framing, Bearer/fallback, flow mapping, backpressure.
- Integration: TCP target, slow flow, UDP best-effort, v1 untouched.
- Benchmark: v1 vs v2 throughput/p95/allocs.
- DPI: ClientHello/JA3/JA4 series, fallback probe.
- Commands: `go test -race ./src/internal/v2/... ./src/internal/config/... ./src/internal/webui/...`, `go vet ./...`, frontend build, Android later.

## Соответствие конституции

- нет конфликтов: TLS 1.3, стандартный HTTP2/Bearer, токены не логируются, v1 не меняется, trace-маркеры на новых декларациях.
