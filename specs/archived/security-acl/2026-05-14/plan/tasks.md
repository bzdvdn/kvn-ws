# Security & ACL — Tasks

Фаза: tasks
Источник: plan.md, plan.digest.md, data-model.md, spec.md

---

- [x] T1.0: TokenCfg structured config (DEC-002)

**Why**: DEC-002 — текущий `[]string` не позволяет задать `bandwidth_bps` и `max_sessions` per token.

**Surface**: `src/internal/config/server.go`, `src/internal/config/server_test.go`

**Acceptance**:
- `ServerAuth` содержит `Tokens []TokenCfg` вместо `[]string`
- `TokenCfg` имеет поля `Name`, `Secret`, `BandwidthBPS`, `MaxSessions`
- backward-compat парсинг: если `tokens: ["tok1"]` → auto-convert в `[{name:"tok1", secret:"tok1"}]` с unlimited лимитами
- Unit test: LoadServerConfig с обоими форматами

**Dependencies**: none

**Trace markers**: `@sk-task security-acl#T1.0`

---

- [x] T2.0: CIDR matcher package (DEC-001)

**Why**: DEC-001 — CIDR-фильтрация на http.Handler уровне.

**Surface**: `src/internal/acl/acl.go` (новый пакет)

**Acceptance**:
- `NewCIDRMatcher(allowCIDRs, denyCIDRs []string) (*CIDRMatcher, error)` — парсит CIDR-строки в `*net.IPNet`
- `m.Allowed(ip net.IP) bool` — логика: если allow пуст → все разрешены (кроме deny); если allow не пуст → только allow; deny всегда имеет приоритет
- Обработка edge: пустые списки, overlap allow/deny (deny wins), невалидные CIDR строки → ошибка
- Unit test: AC-001 (deny 10.0.0.0/8), AC-002 (allow 192.168.0.0/16)

**Dependencies**: none (чистый Go, без внешних зависимостей)

**Trace markers**: `@sk-task security-acl#T2.0`

---

- [x] T3.0: CIDR ACL middleware + main.go integration (DEC-001)

**Why**: DEC-001 — интеграция CIDR-фильтрации в HTTP-сервер.

**Surface**: `src/cmd/server/main.go`

**Acceptance**:
- `ACLCfg` добавлен в `ServerConfig` (`acl:` секция с `deny_cidrs`, `allow_cidrs`)
- Middleware оборачивает mux: проверяет `r.RemoteAddr` через `CIDRMatcher` до вызова хендлера
- При deny: HTTP 403 + лог `"connection denied by CIDR ACL"` + audit log
- При allow (если allow_cidrs указаны и IP не входит): HTTP 403
- SIGHUP reload пересоздаёт CIDRMatcher

**Dependencies**: T2.0 (CIDRMatcher), T1.0 (новые поля конфига)

**Trace markers**: `@sk-task security-acl#T3.0`

---

- [x] T4.0: Origin/Referer validation (DEC-003)

**Why**: DEC-003 — configurable CheckOrigin через `websocket.Accept` с whitelist и glob-паттернами.

**Surface**: `src/internal/transport/websocket/websocket.go`, `src/internal/config/server.go`

**Acceptance**:
- `OriginCfg` добавлен в `ServerConfig` (`origin:` секция с `whitelist`, `allow_empty`)
- `websocket.Accept(w, r, originFn)` — новая сигнатура с опциональной функцией проверки Origin
- `NewOriginChecker(whitelist []string, allowEmpty bool) func(r *http.Request) bool` — имплементация с glob-паттернами (используя `path.Match` или `gobwas/glob`)
- Старый `Accept(w, r)` остаётся как deprecated wrapper для обратной совместимости
- Unit test: AC-005 (валидный Origin), AC-006 (невалидный Origin + 403), пустой Origin, glob `https://*.example.com`

**Dependencies**: none (независимо от T1.0/T2.0)

**Trace markers**: `@sk-task security-acl#T4.0`

---

- [x] T5.0: max_sessions in SessionManager (RQ-003, AC-004)

**Why**: AC-004 — отклонение новых сессий при превышении `max_sessions` per token.

**Surface**: `src/internal/session/session.go`, `src/internal/session/session_test.go`

**Acceptance**:
- `SessionManager` отслеживает количество сессий на токен (map token_name → count)
- `Create(sessionID, tokenName string, maxSessions int)` — новая сигнатура; если `maxSessions > 0` и count ≥ maxSessions → ошибка `"max sessions exceeded"`
- `Remove` декрементирует счётчик
- `SessionManager` получает конфиг токенов (мапа `tokenName → TokenCfg`)
- Unit test: AC-004 (2 сессии, третья отклонена), max_sessions=0 (unlimited), корректный decrement после Remove

**Dependencies**: T1.0 (TokenCfg структура)

**Trace markers**: `@sk-task security-acl#T5.0`

---

- [x] T6.0: Per-token bandwidth limiter (DEC-005, AC-003)

**Why**: DEC-005 — байтовый `rate.Limiter` на write path (TUN→WS).

**Surface**: `src/internal/session/bandwidth.go` (новый файл)

**Acceptance**:
- `NewBandwidthLimiter(bps int) *rate.Limiter` — создаёт rate.Limiter с `rate.Limit(bps/8)` (bps → bytes/s, т.к. rate.Limiter работает в событиях) и burst = bps/8
- `NewTokenBandwidthManager()` — управляет per-token лимитерами, ленивое создание при первом использовании
- `m.Allow(tokenName string, bytes int) bool` — проверяет, можно ли отправить N байт
- При `bandwidth_bps = 0` — unlimited (nil limiter, Allow всегда true)
- Unit test: AC-003 (throughput ≤ limit), unlimited mode

**Dependencies**: T1.0 (TokenCfg для bandwidth_bps)

**Trace markers**: `@sk-task security-acl#T6.0`

---

- [x] T7.0: Bandwidth limiter — интеграция в write path (AC-003)

**Why**: AC-003 — применение bandwidth quota в serverTunToWS.

**Surface**: `src/cmd/server/main.go`

**Acceptance**:
- `handleTunnel` получает `*session.TokenBandwidthManager`
- После аутентификации токена создаётся/per-token bandwidth limiter
- В `serverTunToWS` перед `conn.WriteMessage(data)` вызывается `bwMgr.Allow(tokenName, len(data) + frameOverhead)`
- Если лимит превышен — delay/sleep до восстановления токенов (rate.Limiter.Wait или блокирующий AllowN с sleep)

**Dependencies**: T6.0, T1.0

**Trace markers**: `@sk-task security-acl#T7.0`

---

- [x] T8.0: Admin API — основная структура (DEC-004)

**Why**: DEC-004 — отдельный chi HTTP-сервер для администрирования.

**Surface**: `src/internal/admin/admin.go` (новый пакет), `src/cmd/server/main.go`

**Acceptance**:
- `AdminCfg` добавлен в `ServerConfig` (`admin:` секция с `enabled`, `listen`, `token`)
- `NewAdminServer(cfg AdminCfg, sm *session.SessionManager) *AdminServer` — создаёт chi router
- `s.ListenAndServe() error` — запускает `http.ListenAndServe` на `cfg.Listen`
- `s.Shutdown(ctx) error` — graceful shutdown
- Middleware: проверка `X-Admin-Token` header → 401 если не совпадает (AC-011)
- Если `admin.enabled: false` — сервер не запускается
- Добавлен chi в go.mod: `go get github.com/go-chi/chi/v5`

**Dependencies**: T5.0 (SessionManager)

**Trace markers**: `@sk-task security-acl#T8.0`

---

- [x] T9.0: Admin API — handlers (AC-007, AC-008)

**Why**: AC-007 (GET /admin/sessions), AC-008 (DELETE /admin/sessions/{id}).

**Surface**: `src/internal/admin/admin.go`

**Acceptance**:
- `GET /admin/sessions` → JSON `{"sessions": [{"id": "...", "token_name": "...", "remote_addr": "...", "connected_at": "..."}]}`
- `DELETE /admin/sessions/{id}` → HTTP 200 + disconnect сессии (вызов `sm.Remove(id)`)
- Если сессия не найдена → HTTP 404
- Unit test: handler тесты с mock SessionManager, AC-007, AC-008, AC-011
- Integration test: curl к Admin API

**Dependencies**: T8.0 (AdminServer structure)

**Trace markers**: `@sk-task security-acl#T9.0`

---

- [x] T10.0: Admin API — интеграция в main.go

**Why**: Admin API сервер запускается параллельно с основным.

**Surface**: `src/cmd/server/main.go`

**Acceptance**:
- Admin API стартует в `errgroup` параллельно с основным HTTP-сервером
- Graceful shutdown: при SIGTERM оба сервера останавливаются
- Если `admin.listen` не localhost → warning в лог (R-03 mitigation)
- Admin сервер использует собственный net.Listener (не tlsListener)

**Dependencies**: T8.0, T9.0

**Trace markers**: `@sk-task security-acl#T10.0`

---

- [x] T11.0: mTLS config (DEC-006, AC-009, AC-010) — P2 optional

**Why**: DEC-006 — опциональная mTLS-аутентификация.

**Surface**: `src/internal/transport/tls/tls.go`, `src/internal/config/server.go`

**Acceptance**:
- `TLSCfg` расширен полями `ClientCAFile string`, `ClientAuth string` (values: `require`, `verify`, ``)
- `NewServerTLSConfig` загружает `ClientCAFile` (если указан) и устанавливает `ClientAuth`
- Если `ClientCAFile` не указан — поведение без mTLS (как сейчас)
- Unit test: tls.Dial с сертификатом (AC-009), без сертификата (AC-010)

**Dependencies**: none (опционально, P2)

**Trace markers**: `@sk-task security-acl#T11.0`

---

- [x] T12.0: Config migration guide + backward compat (R-01)

**Why**: R-01 — TokenCfg migration ломает обратную совместимость.

**Surface**: `src/internal/config/server.go`

**Acceptance**:
- `LoadServerConfig` проверяет тип `tokens`: если `[]interface{}` (старый формат) → ConvertToTokenCfgs
- Если `[]map[string]interface{}` (новый формат) → парсинг через mapstructure
- Если mixed/ошибка → warning + fallback на unlimited
- SIGHUP reload корректно обрабатывает оба формата
- `configs/server.yaml.example` обновлён

**Dependencies**: T1.0

**Trace markers**: `@sk-task security-acl#T12.0`

---

## Порядок реализации

```
T1.0 (TokenCfg) ─────┬──→ T5.0 (max_sessions) ──→ T8.0 (Admin struct) ──→ T9.0 (Admin handlers) ──→ T10.0 (Admin integration)
                      ├──→ T6.0 (bandwidth pkg) ──→ T7.0 (bandwidth integrate)
T2.0 (CIDR pkg) ────────→ T3.0 (CIDR middleware)
T4.0 (Origin/Referer)
T11.0 (mTLS, optional) ──→ can be done anytime after T1.0
T12.0 (Config compat) ───→ after T1.0
```

Параллельные треки:
- (T2.0 → T3.0) независим от T1.0
- (T4.0) независим от T1.0
- T11.0 (mTLS) может быть выполнен в любое время
- T12.0 после T1.0

## Покрытие критериев приемки

- AC-001 -> T2.0, T3.0
- AC-002 -> T2.0, T3.0
- AC-003 -> T6.0, T7.0
- AC-004 -> T5.0
- AC-005 -> T4.0
- AC-006 -> T4.0
- AC-007 -> T8.0, T9.0, T10.0
- AC-008 -> T8.0, T9.0, T10.0
- AC-009 -> T11.0
- AC-010 -> T11.0
- AC-011 -> T8.0, T9.0
