---
report_type: inspect
slug: performance-scope-p2
status: pass
docs_language: ru
generated_at: 2026-07-21
---

# Inspect Report: performance-scope-p2

## Scope

- snapshot: adversarial review spec на оптимизацию аллокаций, блокировок и contention в transport/QUIC, transport/WebSocket, ratelimit, dnsproxy
- artifacts:
  - CONSTITUTION.md
  - specs/active/performance-scope-p2/spec.md

## Verdict

- status: pass (замечания исправлены)

## Errors

- none

## Warnings

### W-001 AC-001/AC-005: evidence «pprof показывает снижение аллокаций» неверифицируем в CI

Evidence требует ручного запуска pprof. Для behavioural acceptance критерия нужно проверяемое assertion.

**Статус:** исправлено — evidence заменено на `go test -race -bench=. -benchmem ./...` PASS + code review подтверждает использование `sync.Pool`.

### W-002 AC-009: `go test -race` не ловит «pong без ожидания wmu»

Race detector не проверяет, какой mutex захвачен. Нужен assertion в тесте.

**Статус:** исправлено — evidence заменено на `go test -race -run TestWSControlPlane ./...` PASS, где тест проверяет: (1) pong handler не вызывает `wmu.Lock()`, (2) контрольное сообщение через отдельный канал, (3) keepalive не блокируется при удержании wmu.

### W-003 WS control plane: gorilla/websocket.Conn.WriteMessage внутренне сериализован своим `muW`

Даже при полном обходе `wmu`, control и data writer'ы конкурируют за gorilla's internal write mutex.

**Статус:** исправлено — добавлен абзац в «Контекст» о gorilla's internal muW.

## Questions

### Q-001 Lock ordering DNS split: configMu → pendingMu vs pendingMu → configMu

`forward()` держит `configMu.RLock` → отпускает → держит `pendingMu.Lock`.
`HandleDNSResponse()` держит `pendingMu.Lock` → отпускает → держит `configMu.RLock`.

В текущем коде оба освобождают первый mutex до захвата второго — deadlock'а нет. Но это хрупкое. Если forwardViaTunnel когда-либо понадобится читать config под `configMu` после захвата `pendingMu` (напр. tracker для DNS-ответа), возникнет обратный порядок захвата. Нужно задокументировать lock ordering.

**Рекомендация:** добавить в spec явный пункт «Lock ordering: configMu → pendingMu (forward), pendingMu → configMu (HandleDNSResponse) — always release before acquire».

## Suggestions

### S-001 `maxMessageSize` — existing data race, fix в scope

`QUICConn.ReadMessage` (conn.go:45) читает `c.maxMessageSize` без `mu`. `SetMaxMessageSize` (conn.go:34) пишет его. Это data race, существующий до этой фичи. Spec предлагает убрать `mu` из WriteMessage, что оголит race ещё сильнее.

**Рекомендация:** добавить в scope замену `maxMessageSize int` → `atomic.Int32`. Touches: conn.go.

### S-002 ObfuscatedQUICConn.WriteMessage — дублирует mu removal (RQ-007)

Spec RQ-007 описывает удаление `c.mu` из `QUICConn.WriteMessage`. Но `ObfuscatedQUICConn.WriteMessage` (obfuscated.go:85) тоже захватывает `oc.mu.Lock()` и пишет напрямую в `oc.stream.Write()`. RQ-007 должен явно включать ObfuscatedQUICConn.

**Рекомендация:** расширить RQ-007: «QUICConn.WriteMessage и ObfuscatedQUICConn.WriteMessage НЕ ДОЛЖНЫ захватывать `mu` для stream writes».

### S-003 `nonceInit` — использовать `CompareAndSwap` вместо `Load`+`Store`

`initNonce()`: `if !nonceInit.Load() { ... nonceInit.Store(true) }` — две горутины могут войти в секцию инициации. TLS ExportKeyingMaterial вызовется дважды. Результат одинаковый, но лишний syscall.

**Рекомендация:** `if nonceInit.CompareAndSwap(false, true) { ... }` — одна atomic-операция, гарантия one-shot.

### S-004 AC-004: `math/rand/v2` требует Go 1.22+

Конституция: Go 1.22+. Если в CI используется 1.21 — `math/rand/v2` недоступен. Проверить `go.mod`.

**Рекомендация:** подтвердить Go version в go.mod перед реализацией.

## Traceability

- AC-001→RQ-001: QUIC ReadMessage sync.Pool — evidence исправлен (benchmem + review)
- AC-002→RQ-002: Obfuscated xorBuf sync.Pool — OK
- AC-003→RQ-003: nonceInit atomic.Bool — OK
- AC-004→RQ-004: crypto/rand → math/rand/v2 — OK
- AC-005→RQ-005: BatchWriter.Flush sync.Pool — evidence исправлен (benchmem + review)
- AC-006→RQ-006: rate limiter lock-free — OK
- AC-007→RQ-007: QUIC mu removal — OK (scope включает conn.go + obfuscated.go)
- AC-008→RQ-008: DNS split — OK (lock ordering задокументирован)
- AC-009→RQ-009: WS control plane — evidence исправлен (TestWSControlPlane + gorilla muW в контексте)
- AC-010→RQ-010/RQ-011: all tests pass — OK

Слепых зон AC не найдено.

## Next Step

- safe to continue to plan (с учётом W-001, W-002, S-001, S-002, S-003, S-004)
