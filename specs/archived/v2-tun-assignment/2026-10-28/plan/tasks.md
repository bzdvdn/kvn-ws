# Сервер выдаёт TUN address/gateway — задачи

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/config/v2.go | T1.1 |
| src/internal/config/server.go | T1.1 |
| configs/server.example.yaml | T1.1 |
| src/internal/tunpool/ | T1.2 |
| src/internal/transport/server.go | T1.2 |
| src/cmd/server/main.go | T1.2 |
| src/internal/config/client.go | T2.1 |
| src/internal/client/client.go | T2.1 |
| src/internal/webui/frontend/src/ | T2.2 |
| src/internal/transport/transport_test.go, src/internal/client/client_test.go | T4.1 |
| docs/ru/v2.md, docs/en/v2.md | T4.2 |

## Implementation Context

- Цель MVP: сервер ведёт пул и отдаёт уникальный `tun_address` на `client_id`; client применяет и падает при ошибке.
- Инварианты: два active `client_id` не получают один IP; gateway общий и входит в subnet; lease TTL; lease store переживает рестарт; локального fallback нет.
- Contract: `GET /` + `Accept: application/json` + `X-Request-ID: client_id` + Bearer → `{"tun_address":"10.66.0.2/32","tun_gateway":"10.66.0.1","lease_ttl_sec":300}`; без токена — fallback; пул занят — 503.
- Client: `client_id` автогенерируется при отсутствии и хранится локально; QR/JSON может его не содержать; пустой/ошибочный ответ → ошибка `tun assignment unavailable/invalid/exhausted`, TUN не поднимается.
- Server: `tun` обязателен; невалидный range/gateway → ошибка старта.
- Android: тот же JSON, применить до `establishTun`; DNS routing не менять.
- Proof: unit/integration tests + build/UI/Android smoke; без `Proof:` задача не закрыта.
- Вне scope: per-client pool, IPv6, rotation.

## Фаза 1: Server contract

- [x] T1.1 Добавить pool-конфиг `tun.subnet/range_start/range_end/gateway/lease_ttl_sec` — outcome: поля парсятся, range/gateway валидируются, без них сервер не стартует; Touches: src/internal/config/v2.go, configs/server.example.yaml
      Proof: test src/internal/config/v2_test.go TestV2ServerTUNValidation
- [x] T1.2 Реализовать file-backed TUN pool — outcome: уникальный IPv4 на `client_id`, повторный запрос продлевает lease, TTL освобождает адрес, restart восстанавливает leases; Touches: src/internal/tunpool/
      Proof: test src/internal/tunpool/pool_test.go TestPersistenceAcrossRestart
- [x] T1.3 Реализовать session endpoint на `GET /` — outcome: JSON-Accept + Bearer + `X-Request-ID` получает уникальный DTO, пул занят → 503, остальные — fallback; Touches: src/internal/transport/server.go, src/internal/transport/session.go, src/cmd/server/main.go
      Proof: test src/internal/transport/transport_test.go TestSessionEndpoint

## Фаза 2: Client и UI

- [x] T2.1 Resolve TUN на клиенте без fallback — outcome: клиент автогенерирует/хранит `client_id`, передаёт его в session и streams, применяет серверные значения; при ошибке/пустом ответе TUN не поднимается; Touches: src/internal/config/client.go, src/internal/client/client.go
      Proof: code src/internal/client/client.go runTUN
- [x] T2.2 Обновить v2-only UI — outcome: TUN assignment read-only, без локальных полей; `client_id` скрыт; Touches: src/internal/webui/frontend/src/, src/internal/webui/handler_config.go
      Proof: code src/internal/webui/frontend/src/TabbedForm.tsx

## Фаза 4: Проверка и документация

- [x] T4.1 Покрыть pool/session/client тестами — outcome: uniqueness, renew, TTL expiry, exhaustion, invalid auth, fail-closed, auto client_id; Touches: src/internal/tunpool/, src/internal/transport/transport_test.go, src/internal/client/client_test.go, src/internal/config/server_test.go
      Proof: test src/internal/tunpool/pool_test.go TestPersistenceAcrossRestart
- [x] T4.2 Обновить docs/примеры — outcome: server pool/lease и авто-`client_id` описаны, fallback отсутствует; Touches: docs/ru/v2.md, docs/en/v2.md, configs/client.example.yaml
      Proof: docs docs/ru/v2.md

## Покрытие критериев приемки

- AC-001 -> T1.1, T4.1
- AC-002 -> T1.2, T1.3, T4.1
- AC-003 -> T1.2, T1.3, T4.1
- AC-004 -> T1.3, T2.1, T4.1
- AC-005 -> T2.1, T4.1
- AC-006 -> T2.2
- AC-008 -> T1.2, T4.1
- AC-009 -> T1.3, T4.1
- AC-010 -> T2.1, T4.1
- AC-011 -> T1.2, T4.1
- AC-012 -> T1.2, T2.1, T4.1

## Заметки

- T1.2 не должен добавлять `/api/...` path; используется `/` и `Accept: application/json`.
- T2.1 таймаут session — 2 с; ошибка фатальна, fallback нет.
