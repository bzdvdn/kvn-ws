---
report_type: inspect
slug: security-acl
status: concerns
docs_language: ru
generated_at: 2026-05-13
---

# Inspect Report: security-acl

## Scope

- snapshot: проверка spec на полноту, консистентность и реализуемость перед plan
- artifacts:
  - CONSTITUTION.md
  - specs/active/security-acl/spec.md
  - specs/active/security-acl/spec.digest.md
  - specs/active/security-acl/summary.md
  - src/internal/config/server.go
  - src/internal/transport/tls/tls.go
  - src/internal/transport/websocket/websocket.go
  - src/cmd/server/main.go

## Verdict

- status: concerns (3 Warnings, 0 Errors — spec проходима, но требует уточнений на plan)

## Errors

- none

## Warnings

### W-01: spec.digest.md — нумерация AC не соответствует spec

`spec.digest.md` строки 2: AC-002 указан как "ограничение пропускной способности", но в spec.md AC-002 — "CIDR-фильтрация — allow", а AC-003 — "Per-token bandwidth quota". Необходимо исправить digest: строки должны идти как AC-001 (deny), AC-002 (allow), AC-003 (bandwidth), AC-004 (session limit) и т.д.

### W-02: конфигурация токенов — []string против структурированных per-token лимитов

Текущий `ServerAuth.Tokens` — `[]string` (простой список). Для RQ-002 (bandwidth) и RQ-003 (max_sessions) требуется структурированная конфигурация: `map[string]TokenCfg` с полями `bandwidth_bps` и `max_sessions`. Это ломает обратную совместимость конфига. Spec не описывает новую структуру конфига — требуется явно зафиксировать в spec или на plan.

### W-03: CIDR-фильтрация "до TLS-handshake" — архитектурный разрыв

Spec (RQ-001) требует проверки CIDR до TLS-handshake. Текущая архитектура: `tls.Listen("tcp", addr, tlsCfg)` → `http.Server.Serve(tlsListener)`. TLS-прослушиватель не даёт доступа к `RemoteAddr` до завершения handshake. Решения:
  (a) обернуть `net.Listener` с pre-TLS CIDR filter,
  (b) перенести фильтрацию после TLS но до WebSocket upgrade (менее безопасно).
Spec должен выбрать вариант или допустить post-TLS фильтрацию с оговоркой.

### W-04: CheckOrigin — требуется рефакторинг upgrader

Текущий `websocket.Upgrader` — package-level переменная с `CheckOrigin: func(r *http.Request) bool { return true }`. Spec требует configurable whitelist с glob-паттернами. Необходимо либо передавать `CheckOrigin` при `Accept()`, либо сделать upgrader конфигурируемым. Текущий `websocket.Accept(w, r)` не принимает `CheckOrigin`.

### W-05: chi router — новая зависимость

Spec (Допущения) указывает chi для Admin API. Текущий проект использует `net/http.ServeMux`. Добавление chi — это новая внешняя зависимость с лицензионными и maintenance-последствиями. Стоит подтвердить решение на plan: либо chi, либо `http.ServeMux` с ручной маршрутизацией.

## Questions

- Q-01: bandwidth quota применяется к сырым байтам (throughput) или к пакетам? Существующий `sessionPacketLimiter` считает пакеты, а spec пишет про `rate.Limiter` и `bandwidth_bps` — это байтовый лимит. Требуется уточнить метрику перед реализацией.
- Q-02: Admin API аутентификация — static token в `X-Admin-Token`. Как часто ротируется? Только при перезапуске или через SIGHUP? (SIGHUP reload уже есть, но Admin API token не упомянут в reload-сценарии.)

## Suggestions

- S-01: структура конфига для per-token лимитов может быть: `auth.tokens: [{name: "user1", secret: "tok1", bandwidth_bps: 102400, max_sessions: 2}]` — это обратно-совместимое расширение текущего `[]string`.
- S-02: для CIDR фильтрации без изменения архитектуры — вариант (b): фильтр на `http.Handler` уровне после TLS (`r.RemoteAddr` доступен) — проще и не требует новой listener-обёртки.

## Traceability

- AC-001..AC-011: все критерии в формате Given/When/Then, без placeholder.
- Зависимости (1.2, 1.3, 1.5, 1.6, 1.7) — все реализованы или стабильны, блокеров по спринту нет.
- mTLS (AC-009, AC-010) — опциональный P2, не блокирует plan.

## Next Step

- safe to continue to plan после устранения W-02, W-03 (документирование решений) и W-01 (digest)
