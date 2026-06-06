# Исправление критических утечек и ошибок — Задачи

## Phase Contract

Inputs: plan (6 DEC, 13 AC, ordering), data-model (no-change).
Outputs: 16 задач в 6 фазах, покрытие всех 13 AC.
Stop if: нет — plan чёткий, surfaces известны.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `session/bolt.go` | T1.1 |
| `bootstrap/client/reconnect.go` | T1.2 |
| `bootstrap/client/proxy.go` | T1.3, T1.4, T3.2, T4.2 |
| `bootstrap/server/server.go` | T1.3 |
| `admin/admin.go` | T1.3 |
| `tunnel/session.go` | T1.3, T3.1, T5.1 |
| `transport/quic/dial.go` | T2.1 |
| `bootstrap/client/tun.go` | T2.2, T4.2 |
| `routing/domain_matcher.go` | T2.2 |
| `webui/server.go` | T2.3 |
| `webui/state.go` | T2.3 |
| `proxy/listener.go` | T3.2 |
| `session/session.go` | T4.1 |
| `bootstrap/client/helpers.go` (new) | T4.2 |
| `config/config.go` | T4.3 |

## Implementation Context

- Цель MVP: закрыть AC-001..AC-008 — убрать утечки горутин/контекстов, BoltDB timeout, time.After
- Инварианты:
  - `quic.Dial` сигнатура: `Dial(ctx, addr, tlsConf, quicConf)` — compile-break, обновить оба callers
  - TUN device.Read — blocking syscall; прерывается только close device
  - `SessionManager.cancelFuncs` — отдельный `sync.Mutex`, не под `sm.mu`
  - sync.Pool буферы не zeroing — OK, читаем ровно n байт
- Ошибки/коды:
  - Swallowed `_ = json.NewEncoder(w).Encode(...)` → `if err := ...; err != nil { s.logger.Error(...) }`
  - Type assertion `err.(net.Error)` → `errors.As(err, &netErr)`
  - `netip.AddrFromSlice` error → логировать и skip
- Контракты/протокол:
  - BoltDB: `bbolt.DefaultOptions` c `Timeout: 1*time.Second`
  - DNS: `context.Background()` → переданный `ctx`
- Proof signals:
  - `runtime.NumGoroutine()` стабилен после 10 reconnect
  - `grep '_ = json.NewEncoder'` = 0
  - `go test -race ./...` pass
- Вне scope: разделение больших файлов, interface{} → any, rate limiter refactor

## Фаза 1: Безопасные изолированные патчи

Цель: быстрые безопасные исправления без изменения сигнатур и lifecycle-логики.

- [x] T1.1 Добавить BoltDB timeout. `session/bolt.go`: передать `bbolt.DefaultOptions` с `Timeout: 1*time.Second` в оба вызова `bbolt.Open`. Touches: `session/bolt.go`
  AC-007. DEC: none.

- [x] T1.2 Исправить утечку `time.After`. `bootstrap/client/reconnect.go`: заменить `time.After(d)` на `time.NewTimer(d)` + `defer timer.Stop()` + `timer.C`. Touches: `bootstrap/client/reconnect.go`
  AC-008. DEC: none.

- [x] T1.3 Логировать swallowed errors во всех местах. Заменить `_ = json.NewEncoder(w).Encode(...)` на `if err := json.NewEncoder(w).Encode(...); err != nil { logger.Error(...) }` в:
  - `bootstrap/server/server.go:335,356`
  - `admin/admin.go:112,129`
  - `tunnel/session.go:202` (`_ = s.stream.WriteMessage(encoded)`)
  + добавлен `logger *zap.Logger` в `admin.AdminServer`, обновлён конструктор и caller в `server.go`
  Touches: `bootstrap/server/server.go`, `admin/admin.go`, `tunnel/session.go`
  AC-009. DEC: none.

- [x] T1.4 Заменить type assertion на `errors.As`. `bootstrap/client/proxy.go:295`: `netErr, ok := err.(net.Error)` → `var netErr net.Error; errors.As(err, &netErr)`. Touches: `bootstrap/client/proxy.go`
  AC-011. DEC: none.

## Фаза 2: Context propagation

Цель: все операции используют переданный контекст для корректного shutdown.

- [x] T2.1 Добавить `ctx` параметр в `quic.Dial`. Сигнатура: `Dial(ctx context.Context, addr string, tlsConf *tls.Config, quicConf *quic.Config)`. Убрать внутренний `context.WithTimeout(context.Background(), ...)`. Обновить callers:
  - `bootstrap/client/proxy.go` — передать `ctx`
  - `bootstrap/client/tun.go` — передать `ctx`
  Touches: `transport/quic/dial.go`, `bootstrap/client/proxy.go`, `bootstrap/client/tun.go`
  AC-004. DEC: none.

- [x] T2.2 Исправить DNS lookups — использовать переданный контекст вместо `context.Background()`.
  - `bootstrap/client/tun.go:226` — `context.Background()` → `ctx` (замыкание захватывает ctx)
  - `routing/domain_matcher.go:103` — добавлен `baseCtx context.Context` в `DomainMatcher`, метод `SetCtx(ctx)`
  Touches: `bootstrap/client/tun.go` (DNS lookups), `routing/domain_matcher.go` (refreshCache + NewDomainMatcher)
  AC-005. DEC: none.

- [x] T2.3 WebUI broadcast использует контекст shutdown. `webui/server.go`: передать `ctx` из `Serve()` в `state.broadcastLogs(ctx)` и `state.broadcastStatus(ctx)` вместо `context.Background()`. `webui/state.go`: функции уже принимают `ctx`, ничего не менять. Touches: `webui/server.go`
  AC-006. DEC: none.

## Фаза 3: Goroutine lifecycle

Цель: исправить goroutine leaks в TUN reader, proxy listener, RouteDirect.

- [x] T3.1 Переписать TUN reader — убрать per-read goroutine. `tunnel/session.go`:
  - Удалить `tunReadInterruptible` (или оставить stub)
  - В `tunToWS`: заменить паттерн на постоянную reader-горутину с select на `ctx.Done()`
  - При `ctx.Done()` закрыть `s.tunDev` чтобы прервать blocking `Read()`
  - Закрытие device требует пересоздания при reconnect — это уже есть в reconnect loop
  Touches: `tunnel/session.go`
  AC-001. DEC-001.

- [x] T3.2 Добавить semaphore в proxy Listener. `proxy/listener.go`:
  - Добавить поле `sem chan struct{}` в `Listener` с capacity = `defaultProxyConcurrency` (1000) или параметром
  - В `AcceptLoop` или `handleClient`: `sem <- struct{}{}` / `defer <-sem`
  - При достижении лимита — блокировка (keepalive), а не reject
  Touches: `proxy/listener.go`
  AC-002. DEC-002.

- [x] T3.3 RouteDirect — lifecycle через errgroup. `bootstrap/client/proxy.go`:
  - Заменить `go func() { ... }()` в RouteDirect (2 места, строки 210-221 и 241-254) на `errgroup` + ctx-aware cancel
  - Оба `io.Copy` через `eg.Go()`, ожидание через `eg.Wait()` (оба завершаются)
  - Добавить `ctx` propagation: errgroup.WithContext(ctx) + cleanup goroutine закрывает conns при gctx.Done()
  Touches: `bootstrap/client/proxy.go`
  AC-003. DEC-003.

## Фаза 4: Locking и рефакторинг

Цель: устранить deadlock risk, дублирование кода и глобальное состояние.

- [x] T4.1 Разделить lock в SessionManager. `session/session.go`:
  - Добавить `cancelFuncsMu sync.Mutex` в `SessionManager`
  - `cancelSession`, `SetCancel` используют `cancelFuncsMu` вместо `sm.mu`
  - `expireIdle`, `expireTTL`, `Remove` уже держат `sm.mu` — оставить как есть, но `cancelSession` вызывать после unlock `sm.mu` (defer) или под `cancelFuncsMu`
  - ВАЖНО: `cancelSession` вызывается под `sm.mu` в `expireIdle/expireTTL/Remove`. Нужно либо: (a) вынести вызов `cancelSession` за пределы `sm.mu` — собрать id для отмены под lock, отменить после unlock; либо (b) `cancelSession` использует `cancelFuncsMu`, а не `sm.mu`. Рекомендуется (a): двухфазный подход.
  Touches: `session/session.go`
  AC-010. DEC-004.

- [x] T4.2 Вынести TLS config + backoff parsing в shared helper. `bootstrap/client/`:
  - Создать `helpers.go` с функциями:
    - `clientTLSConfig(cfg *config.ClientConfig) (*tls.Config, error)`
    - `parseBackoff(cfg *config.ReconnectConfig) (min, max time.Duration)`
  - Заменить дубликаты в `proxy.go:30-53` и `tun.go:29-52` на вызовы helpers
  Touches: `bootstrap/client/helpers.go` (new), `bootstrap/client/proxy.go`, `bootstrap/client/tun.go`
  AC-012. DEC-006.

- [x] T4.3 Убрать глобальное мутабельное состояние. `config/config.go`:
  - `envPrefixForWarning` — убрать пакетную переменную
  - `secretFromEnv` и `warnSecretInFile` принимают `prefix string` параметром
  - `load` возвращает prefix или не использует `envPrefixForWarning`
  Touches: `config/config.go`
  Конституция: no global mutable state.

## Фаза 5: Performance (sync.Pool)

Цель: снизить GC pressure под нагрузкой.

- [x] T5.1 Добавить `sync.Pool` для 4KB буферов в proxy stream. `tunnel/session.go`:
  - Создать пакетный `var bufPool = sync.Pool{New: func() any { return make([]byte, 4096) }}`
  - В proxy goroutine (строка 241): `buf := bufPool.Get().([]byte)` / `defer bufPool.Put(buf)`
  - Аналогично для payload (строка 261): пул для `[]byte` переменной длины или отдельный пул
  - Для payload переменной длины — использовать `framing.ReturnBuffer` / `framing.GetBuffer` из существующего пула
  Touches: `tunnel/session.go`
  AC-013. DEC-005.

## Фаза 6: Верификация

Цель: доказать, что все 13 AC закрыты, регрессий нет.

- [x] T6.1 Написать/обновить тесты для AC-001..AC-013.
  - AC-001: `TestTunGoroutineLeak` — 10 reconnect + `NumGoroutine`
  - AC-002: `TestProxySemaphore` — 2000 парал. соединений
  - AC-003: `TestRouteDirectLifecycle` — errgroup завершает обе горутины
  - AC-004: `TestQUICDialContextCancel`
  - AC-005: `TestDNSContextPropagation`
  - AC-006: `TestWebUIBroadcastShutdown`
  - AC-007: `TestBoltDBTimeout`
  - AC-008: `TestSleepWithContextTimerLeak`
  - AC-010: `TestSessionManagerLockOrdering` — parallel expireIdle + SetCancel
  - AC-011: `TestTypeAssertionErrorsAs` — wrapped net.Error
  Touches: тестовые файлы в соответствующих пакетах
  Все AC.

- [x] T6.2 Финальная проверка. Запустить:
  - `go test -race ./...`
  - `golangci-lint run`
  - `grep '_ = json.NewEncoder' src/` → 0 matches
  - `grep 'context\.Background()' src/internal/routing/domain_matcher.go src/internal/bootstrap/client/tun.go src/internal/bootstrap/client/proxy.go` → 0 matches (кроме легитимных)
  - `scripts/test-gate.sh` (если существует)
  Touches: весь проект

## Покрытие критериев приемки

- AC-001 (TUN goroutine leak) -> T3.1, T6.1
- AC-002 (Proxy listener semaphore) -> T3.2, T6.1
- AC-003 (RouteDirect lifecycle) -> T3.3, T6.1
- AC-004 (QUIC ctx cancel) -> T2.1, T6.1
- AC-005 (DNS ctx propagation) -> T2.2, T6.1
- AC-006 (WebUI broadcast shutdown) -> T2.3, T6.1
- AC-007 (BoltDB timeout) -> T1.1, T6.1
- AC-008 (time.After leak) -> T1.2, T6.1
- AC-009 (swallowed errors) -> T1.3, T6.2
- AC-010 (lock ordering) -> T4.1, T6.1
- AC-011 (errors.As) -> T1.4, T6.1
- AC-012 (helpers.go) -> T4.2, T6.2
- AC-013 (sync.Pool) -> T5.1, T6.2

## Заметки

- Фаза 1 (T1.1..T1.4) полностью параллельна — нет пересечения файлов
- T2.1 (QUIC) требует обновления callers в T4.2 из-за helpers.go — учитывать порядок
- T4.1 (lock split) — critical: проверить, что `cancelSession` не вызывается под `sm.mu`
- T6.1 покрывает 10 из 13 AC тестами; AC-009, AC-012, AC-013 проверяются в T6.2 через grep/lint
