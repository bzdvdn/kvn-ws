# Сервер выдаёт уникальный TUN address — план

## Phase Contract

Inputs: `spec.md`, v2 server/client/webui surfaces.
Outputs: pool/lease, session endpoint, client apply, UI/Android.

## Цель

Добавить in-memory пул IPv4 с lease на `client_id`, авторизованный session endpoint и клиентский `client_id`; при отсутствии/исчерпании lease подключение fail-closed.

## MVP Slice

- `tun` pool config + validation.
- Allocator unique address per `client_id` + TTL.
- Session endpoint `GET /` + JSON Accept + Bearer + `X-Request-ID`.
- Desktop client + UI read-only.
- Закрывает `AC-001`–`AC-006`, `AC-008`, `AC-009`, `AC-010`, `AC-011`, `AC-012`.
- Android — в `v2-standard` T4.

## First Validation Path

1. Запустить server с pool `10.66.0.2–10.66.0.10`.
2. Два `curl` с разными `X-Request-ID` → разные `tun_address`; повтор с тем же ID → тот же адрес.
3. Занять пул → новый ID получает 503, клиент не поднимает TUN.
4. Android smoke после mobile core.

## Scope

- Server config/pool/lease/session + persisted lease file.
- Client `client_id`/session/apply/fail-closed.
- Web UI read-only.
- Tests/docs.

## Performance Budget

- Session p95 < 300 ms в lab; lease lookup O(1).
- 100 клиентов в /24 без деградации.
- Один дополнительный HTTP/2 запрос на connect; timeout 2 с.

## Implementation Surfaces

- `src/internal/config/server.go`, `configs/server.example.yaml` — pool config/validation.
- `src/internal/tunpool/` — allocator/lease TTL.
- `src/internal/transport/server.go`, `session.go` — allocator wiring/JSON.
- `src/cmd/server/main.go` — pool init.
- `src/internal/config/client.go`, `src/internal/client/client.go` — `client_id`, session/apply.
- `src/internal/webui` + frontend — read-only assignment.
- Tests/docs.

## Bootstrapping Surfaces

- `TUNPool` interface + in-memory implementation with clock injection.
- `client_id` persistence in unified config.
- `TUNSession` DTO with address/gateway/lease TTL.

## Влияние на архитектуру

- Server становится stateful по leases (in-memory), TCP streams остаются stateless.
- Client ID стабилен; reconnect получает тот же IP до TTL.
- Пулы/TTL не влияют на TCP/UDP wire.
- Android получает тот же DTO.

## Acceptance Approach

- AC-001 → config tests.
- AC-002/003/008/011/012 → tunpool tests (unique/renew/expiry/restart/touch).
- AC-004 → pool exhaustion + client fail-closed.
- AC-005 → client fake session + TUN capture.
- AC-006 → frontend build/API.
- AC-009 → HTTP/2 fallback/active probe.
- AC-010 → config store/import tests + client restart smoke.

## Данные и контракты

- Data model: pool range/subnet/gateway/TTL/lease_file; lease `{client_id, ip, expires_at}` persisted atomically; client `client_id`, no local TUN.
- Session JSON: `{"tun_address":"10.66.0.2/32","tun_gateway":"10.66.0.1","lease_ttl_sec":300}`.
- Path: `GET /`, `Accept: application/json`, `X-Request-ID: client_id`, Bearer.
- 503 on exhaustion; fallback HTML for unauthorized.
- No change to TCP/UDP stream contract.

## Стратегия реализации

- DEC-001 File-backed pool + TTL
  Why: рестарт сервера не должен менять адреса активных клиентов; JSON достаточно для 2.0.
  Tradeoff: один файл состояния, нужен writable path; atomic rename.
  Affects: tunpool, server config.
  Validation: restart persistence test + unique/renew/expiry.

- DEC-002 Ключ lease — `X-Request-ID` (client_id)
  Why: позволяет reconnect получить тот же IP и не требует серверной БД.
  Tradeoff: клиент обязан хранить стабильный ID; QR/JSON не должен требовать его, клиент генерирует сам.
  - DEC-005 Автогенерация client_id на клиенте
    Why: импорт QR/JSON не должен ломаться из-за забытого ID.
    Tradeoff: ID хранится локально (config/DataStore/sidecar), а не в переносимом QR.
    Affects: config/client/store, Android.
    Validation: import без client_id → разные ID у двух устройств, повторный запуск сохраняет ID.
  Affects: client config, session, Android.
  Validation: two clients/two IDs, same ID renew.

- DEC-003 Fail-closed без fallback
  Why: локальный адрес может конфликтовать; ошибка должна быть явной.
  Tradeoff: session должен быть доступен до TUN; timeout 2 с.
  Affects: client, UI.
  Validation: 503/timeout/empty tests.

- DEC-004 `/` + JSON Accept
  Why: сохраняет browser-like path и fallback.
  Tradeoff: content negotiation на `/`; не-JSON идёт в fallback.
  Affects: transport server.
  Validation: JSON/fallback tests.

## Incremental Delivery

### MVP

- Pool/config/session + desktop client/UI; AC-001–006, 008, 009.
- Android — следующий инкремент (AC-007).

## Порядок реализации

1. Config/pool + tests.
2. Session endpoint.
3. Client ID/session/apply.
4. UI.
5. Android.
6. Docs.

## Риски

- Lease leak при рестарте/сбое клиента.
  Mitigation: TTL + lazy cleanup + persisted store; тест expiry/restart.
- Активный поток не продлевает lease.
  Mitigation: `X-Request-ID` на stream + pool.Touch; тест TTL/touch.
- Session endpoint — отпечаток.
  Mitigation: `/` + JSON Accept, fallback, no KVN headers.
- Пул исчерпан.
  Mitigation: 503 + понятная ошибка, конфигурируемый range.

## Rollout и compatibility

- Server без pool не стартует (по спеке).
- Client без `client_id` получает ошибку.
- Рестарт сервера: lease store восстанавливается; HTTP/2 переподключается; TUN не меняется.
- Monitoring: leased/free, exhaustion, lease TTL, restart recovery.

## Проверка

- Config/tunpool/client/server tests.
- HTTP/2 integration + fallback.
- Frontend build/API.
- Android smoke.
- DPI active probe.

## Соответствие конституции

- нет конфликтов.
