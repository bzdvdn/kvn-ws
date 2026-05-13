# Security & ACL План

## Phase Contract

Inputs: spec (spec.md, spec.digest.md), inspect (inspect.md — concerns, safe to continue), repo context.
Outputs: plan.md, plan.digest.md, data-model.md.
Stop if: нет — inspect явно разрешает продолжение с документированием решений.

## Цель

Форма реализации многоуровневой защиты сервера KVNOS: CIDR-фильтрация, per-token bandwidth/session лимиты, Origin/Referer validation, Admin API, опциональный mTLS.

## MVP Slice

CIDR ACL + Origin/Referer validation — минимальный защитный слой, закрывающий CSWSH и доступ из недоверенных сетей (AC-001, AC-002, AC-005, AC-006).

## First Validation Path

Старт сервера с конфигом `acl.deny_cidrs: ["10.0.0.0/8"]` + `origin.whitelist: ["https://example.com"]`. Подключение клиента с IP 10.0.0.1 — reset/refused. Подключение с валидным Origin и IP вне deny — успешный WS upgrade.

## Scope

- ACL-пакет: CIDR matcher (allow/deny списки, проверка `net.IP` через `net.IPNet.Contains`)
- Per-token лимиты: structured token config (TokenCfg с `bandwidth_bps`, `max_sessions`), проверка в `SessionManager.Create()`, rate.Limiter на запись
- Origin/Referer: configurable whitelist с glob-паттернами, проверка в `websocket.Accept` до upgrade
- Admin API: chi router, localhost:port, эндпоинты GET /admin/sessions, DELETE /admin/sessions/{id}, X-Admin-Token аутентификация
- mTLS: опционально, `tls.ClientCAFile` + `RequireAndVerifyClientCert`
- Явно НЕ трогаем: per-IP rate limiting (уже есть), SIGHUP reload токенов (существующий механизм остаётся), BoltDB-схема (не меняется)

## Implementation Surfaces

- `src/internal/acl/` — новая, CIDR matcher (allow/deny lists)
- `src/internal/config/server.go` — существующая, расширение: `ACL cfg`, `TokenCfg`, `OriginWhitelist`, `AdminAPICfg`, mTLS поля
- `src/internal/protocol/auth/auth.go` — существующая, ValidateToken под новую структуру
- `src/internal/transport/websocket/websocket.go` — существующая, CheckOrigin как параметр Accept
- `src/internal/transport/tls/tls.go` — существующая, mTLS config
- `src/internal/session/session.go` — существующая, max_sessions check в Create()
- `src/internal/session/bandwidth.go` — новая, per-token bandwidth rate.Limiter
- `src/cmd/server/main.go` — существующая, интеграция всех слоёв + Admin API сервер

## Bootstrapping Surfaces

`src/internal/acl/` + `src/internal/session/bandwidth.go` — новые пакеты должны существовать до реализации остального.

## Влияние на архитектуру

- **Локальное**: ACL-пакет — новый модуль без внешних зависимостей. Bandwidth limiter — расширение session.
- **Среднее**: Token config migration (`[]string` → `[]TokenCfg`) — ломает обратную совместимость конфига. requires config migration guide.
- **Admin API**: новый HTTP-сервер на chi (новая зависимость go.mod). Не влияет на основной WS-сервер.
- **mTLS**: опционален, не меняет архитектуру при нулевой конфигурации.
- **CheckOrigin**: рефакторинг `websocket.Accept` — не ломает существующий API, т.к. Accept редко вызывается вне main.go.

## Acceptance Approach

- AC-001/AC-002 → CIDR matcher unit test + integration test с net.Dial. Surf: acl-package, server main.go. Evidence: лог блокировки + reset.
- AC-003 → bandwidth limiter unit test. Surf: bandwidth-limiter, server main.go. Evidence: throughput ≤ limit.
- AC-004 → max_sessions в SessionManager.Create(). Surf: session.go, server main.go. Evidence: третья сессия отклонена.
- AC-005/AC-006 → CheckOrigin в websocket.Accept + unit test. Surf: websocket-upgrade. Evidence: 403/upgrade.
- AC-007/AC-008 → Admin API integration test (curl). Surf: admin-api. Evidence: JSON-ответ + disconnect.
- AC-009/AC-010 → mTLS unit test (tls.Dial с/без сертификата). Surf: tls-mtls. Evidence: успех/ошибка handshake.
- AC-011 → Admin API middleware test. Surf: admin-api. Evidence: 401 без токена.

## Данные и контракты

- data-model.md прилагается (no-change для BoltDB, расширение YAML-конфига).

## Стратегия реализации

### DEC-001: CIDR-фильтрация после TLS (http.Handler level)

Why: W-03 — tls.Listen не даёт RemoteAddr до handshake. Обёртка net.Listener для pre-TLS фильтрации избыточна для MVP. `r.RemoteAddr` доступен на уровне http.Handler.
Tradeoff: TLS-handshake с deny-клиентом всё равно происходит. Приемлемо для защиты от CSWSH/abuse; для DoS-защиты на TLS достаточно существующего rate limiter.
Affects: src/internal/acl/acl.go, src/cmd/server/main.go (middleware на /tunnel)
Validation: AC-001, AC-002 — тест net.Dial из deny-подсети

### DEC-002: Структурированная конфигурация токенов ([]TokenCfg)

Why: W-02 — текущий `[]string` не позволяет задать `bandwidth_bps` и `max_sessions` per token. Новая структура позволяет расширять токен без ломки схемы.
TokenCfg: `{name, secret_hash?, bandwidth_bps, max_sessions}`. Имя токена используется в логах и Admin API.
Tradeoff: ломает обратную совместимость конфига (старый `tokens: ["tok1"]` → новый `tokens: [{name: "user1", secret: "tok1", bandwidth_bps: 0, max_sessions: 0}]`). Нули = unlimited.
Affects: config/server.go, auth/auth.go, server/main.go
Validation: AC-003, AC-004 — тесты лимитов

### DEC-003: CheckOrigin через конфигурируемый upgrader

Why: W-04 — текущий package-level upgrader c `CheckOrigin: true`. Нужен whitelist с glob-паттернами (`https://*.example.com`). Решение: добавить `CheckOrigin` как параметр `websocket.Accept(w, r, originFn)`.
Tradeoff: меняет сигнатуру `Accept`, но это internal API, не публичный контракт.
Affects: websocket.go, server/main.go
Validation: AC-005, AC-006

### DEC-004: Admin API на chi router

Why: W-05 — chi обеспечивает `{id}` path parameter без ручного разбора. Лёгкая зависимость (MIT).
Tradeoff: новая внешняя зависимость go.mod. Альтернатива: http.ServeMux + `strings.Split(r.URL.Path, "/")` — менее надёжно.
Affects: server/main.go, go.mod
Validation: AC-007, AC-008, AC-011

### DEC-005: Bandwidth quota — байтовый rate.Limiter на write path

Why: Q-01 — spec указывает `rate.Limiter` и `bandwidth_bps`, это throughput в байтах. Лимит на write path (TUN→WS) контролирует скорость отправки клиенту.
Tradeoff: не лимитирует upload клиента. Для симметричного лимита потребуется два rate.Limiter — отложено до P2.
Affects: session/bandwidth.go, server/main.go
Validation: AC-003

### DEC-006: mTLS опционально через TLS config

Why: AC-009, AC-010 — P2, опционально. Включается полями `tls.client_ca_file` и `tls.client_auth: require` (или `verify`). Используется стандартный `crypto/tls.ClientAuth`.
Tradeoff: увеличение времени handshake (CA verification). Не влияет на сервер без mTLS-конфига.
Affects: tls/tls.go, config/server.go, server/main.go
Validation: AC-009, AC-010 — tls.Dial тесты

## Incremental Delivery

### MVP (Первая ценность)

1. CIDR ACL (AC-001, AC-002) — acl-package + middleware в main.go
2. Origin/Referer validation (AC-005, AC-006) — configurable CheckOrigin
Критерий: сервер блокирует deny-CIDR и невалидный Origin за один проход.

### Итеративное расширение

3. Per-token session limits (AC-004) — max_sessions в SessionManager
4. Per-token bandwidth quota (AC-003) — bandwidth limiter
5. Admin API (AC-007, AC-008, AC-011) — chi сервер + handlers
6. mTLS (AC-009, AC-010) — опционально, после всего P1

## Порядок реализации

1. Структура TokenCfg и config —必须先, т.к. от неё зависят лимиты.
2. ACL-пакет + CIDR middleware — независимо от TokenCfg.
3. Origin/Referer validation — независимо, только конфиг + websocket.
4. max_sessions в SessionManager.Create() — после TokenCfg.
5. Bandwidth limiter — после TokenCfg.
6. Admin API — после session (нужен доступ к SessionManager).
7. mTLS — опционально, в конце.

Параллельно: (1) и (2), (1) и (3).

## Риски

- **R-01 (Config breaking)**: TokenCfg migration требует обновления всех server.yaml. Mitigation: backward-compat парсинг — если `tokens` — `[]string`, конвертировать в `[]TokenCfg` с default-лимитами.
- **R-02 (CIDR performance)**: Проверка каждого соединения против N CIDR-сетей. Mitigation: для MVP линейный поиск ок (<100 CIDR); при >1000 — trie/radix tree (отложено).
- **R-03 (Admin API security)**: Admin API на localhost случайно открыт снаружи. Mitigation: default localhost:port; явный warning в логе если слушает не на loopback.

## Rollout и compatibility

- Config migration: старый `tokens: ["tok1"]` → новый `tokens: [{name: "tok1", secret: "tok1"}]` с fallback для `[]string`.
- New go.sum entry: chi.
- Admin API по умолчанию выключен (admin.enabled: false). Включается оператором явно.
- mTLS по умолчанию выключен.
- SIGHUP reload работает для всех новых секций (viper reload).

## Проверка

- `go test ./...` с race detector — покрытие AC-001..AC-011.
- CIDR: unit test (acl pkg) + integration (поднять сервер, net.Dial).
- Origin: unit test (glob match) + integration (HTTP запрос с Origin).
- Bandwidth: unit test с rate.Limiter.
- Session limit: unit test (SessionManager).
- Admin API: unit test handlers + integration test (curl).
- mTLS: tls.Dial с сертификатом/без.
- Manual: проверить server.yaml + SIGHUP reload.

## Соответствие конституции

нет конфликтов. DDD сохраняется — ACL и bandwidth limiter выделены в отдельные внутренние пакеты без внешних зависимостей. Trace-маркеры `@sk-task` добавляются. docker-compose не меняется.
