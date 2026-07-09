# Arch Fix: Critical Paths — Задачи

## Phase Contract

Inputs: plan.md, data-model.md, контекст репозитория.
Outputs: исполнимые задачи с Touches и покрытием AC.
Stop if: задачи расплывчаты — нет, каждый fix конкретен.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/bootstrap/relay/router.go` | T1.1, T1.2, T3.1, T3.2 |
| `src/internal/bootstrap/relay/handler.go` | T4.1, T5.1 |
| `src/internal/bootstrap/relay/bootstrap.go` | T2.3 |
| `src/internal/bootstrap/server/server.go` | T2.2 |
| `src/internal/transport/quic/listen.go` | T2.1 |
| `src/internal/bootstrap/relay/router_test.go` | T1.1T, T1.2T, T3.1T, T3.2T |
| `src/internal/bootstrap/relay/handler_test.go` | T5.1 |
| `src/internal/bootstrap/server/server_test.go` | T2.2T |
| `src/internal/transport/quic/export_test.go` | T2.1T |

## Implementation Context

- **Цель MVP**: QUIC backoff (AC-001) — сервер/relay не падают при transient ошибках Accept.
- **Инварианты/семантика**:
  - Backoff: base=10ms, max=5s, multiplier=×2, capped. Context.Canceled = immediate return без backoff.
  - DNS pool: sync.Pool[net.Conn], dial per upstream addr, deadline 5s.
  - DNS cache: max 10000, при превышении — TTL-scan + oldest-first eviction.
  - Relay session: прямой вызов sm.Create()/SetCancel()/Remove() — паттерн идентичен server handler.
  - Все guard-проверки: молчаливый return zero/nil/false — без паник и ошибок.
- **Ошибки/коды**: никаких новых error sentinel-значений. Все ошибки логируются (zap.Warn/Error).
- **Контракты/протокол**: не меняются. Фреймовый протокол, handshake, конфигурация — без изменений.
- **Границы scope**:
  - Не меняем transport interface (StreamConn, TransportFactory).
  - Не меняем session manager interface.
  - Не добавляем config-параметры (константы: 10ms, 5s, 10000).
- **Proof signals**:
  - `go test -race -count=1 ./...` — зелёный.
  - `gosec ./...` — без новых warning.
  - Каждый AC → тест, который красный до фикса и зелёный после.
- **References**: DEC-001 (backoff helper), DEC-002 (DNS pool), DEC-003 (cache eviction), DEC-004 (relay session integration).

## Фаза 1: Быстрые патчи (AC-005, AC-006)

Цель: закрыть самые локальные проблемы — uint16 overflow и boundary checks. Независимые, без риска.

- [x] T1.1 Добавить overflow guard в buildDNSRespPacket — проверка `totalLen > 65535 → return nil` после расчёта длины в `buildDNSRespPacket()`. Touches: `src/internal/bootstrap/relay/router.go`
- [x] T1.1T Добавить unit-тест: buildDNSRespPacket с payload=70000 → assert nil. Touches: `src/internal/bootstrap/relay/router_test.go`
- [x] T1.2 Добавить boundary guards в extractDestIP, isDNSQuery, cacheDNSResponse — каждая функция получает проверку `len(packet) < minimalHeader → return false/zero`. Touches: `src/internal/bootstrap/relay/router.go`
- [x] T1.2T Добавить unit-тесты: каждую raw-функцию прогнать с len=0, len=1, truncated header. Touches: `src/internal/bootstrap/relay/router_test.go`

## Фаза 2: QUIC backoff (AC-001)

Цель: сервер и relay не падают при transient QUIC Accept ошибках.

- [x] T2.1 Реализовать `AcceptWithBackoff(ctx, acceptFn, logger)` — helper в quic/listen.go. Backoff: 10ms → 5s, ×2, capped. Экспортируемый. Touches: `src/internal/transport/quic/listen.go`
- [x] T2.1T 4 unit-теста: transient errors, context.Canceled, first-try success, context.Timeout. Touches: `src/internal/transport/quic/listen_test.go`
- [x] T2.2 Применить backoff в server QUIC accept loop. Touches: `src/internal/bootstrap/server/server.go`
- [x] T2.2T Добавить handler-тесты (isWebSocketRequest, allowedWSPath). Touches: `src/internal/bootstrap/server/server_test.go`
- [x] T2.3 Применить backoff в relay QUIC accept loop. Touches: `src/internal/bootstrap/relay/bootstrap.go`
- [x] T2.3T Backoff validated in quic unit tests (T2.1T); relay handler tests cover integration (T5.1). Touches: `src/internal/bootstrap/relay/router_test.go` (existing)

## Фаза 3: DNS subsystem (AC-002, AC-003)

Цель: DNS upstream через pool соединений, DNS cache с limit eviction.

- [x] T3.1 Реализовать DNS upstream pool: sync.Pool + getDNSConn/putDNSConn helpers. Touches: `src/internal/bootstrap/relay/router.go`, `bridge.go`, `bootstrap.go`
- [x] T3.1T Unit-тест: TestGetDNSConnPool — pool reuse и dial count. Touches: `src/internal/bootstrap/relay/router_test.go`
- [x] T3.2 Добавить limit DNS cache: insertDNSCache с eviction (expired + oldest). Touches: `src/internal/bootstrap/relay/router.go`
- [x] T3.2T Unit-тесты: TestInsertDNSCacheLimit, TestInsertDNSCacheEvictExpired. Touches: `src/internal/bootstrap/relay/router_test.go`

## Фаза 4: Relay session + handler tests (AC-004, AC-007)

Цель: relay сессии видны в мониторинге; handler покрыт тестами.

- [x] T4.1 Зарегистрировать relay-сессию в SessionManager: handleTerminatorStream использует sm.Create + SetCancel + Remove. Touches: `src/internal/bootstrap/relay/handler.go`
- [x] T4.1T Handler test: TestHandleTerminatorStreamSessionLifecycle — создание и удаление сессии. Touches: `src/internal/bootstrap/relay/handler_test.go`
- [x] T5.1 Написать handler_test.go для relay: mock StreamConn, mock TunDevice. Тесты: happy path session lifecycle, invalid token rejection. Touches: `src/internal/bootstrap/relay/handler_test.go`

## Фаза 5: Проверка

Цель: финальная верификация всей spec.

- [x] T6.1 Запустить `go test -race ./...`, `go vet ./...`, `gosec ./...`, `golangci-lint run ./...`. Touches: все изменённые файлы.

## Покрытие критериев приемки

- AC-001 -> T2.1, T2.1T, T2.2, T2.2T, T2.3, T2.3T
- AC-002 -> T3.1, T3.1T
- AC-003 -> T3.2, T3.2T
- AC-004 -> T4.1, T4.1T
- AC-005 -> T1.1, T1.1T
- AC-006 -> T1.2, T1.2T
- AC-007 -> T5.1

## Заметки

- T1.x и T2.x можно безопасно параллелить (независимые файлы).
- T3.x зависит от T1.x (те же функции в router.go — merge conflict).
- T4.1 зависит от существующей sm — не требует config-изменений.
- T5.1 — новые тесты, может выполняться параллельно с T3.x и T4.1 (но ссылается на те же handler.go — merge conflict на уровне git, не логики).
