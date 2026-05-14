# Production Readiness Hardening — Задачи

## Phase Contract

Inputs: plan.md, spec.md.
Outputs: упорядоченные задачи с покрытием AC.
Stop if: задачи расплывчаты или coverage не сходится — нет, всё конкретно.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/cmd/server/main.go` | T1.1, T2.1, T2.2, T2.3, T2.4, T3.5 |
| `src/internal/transport/websocket/dataframe.go` | T2.3 |
| `src/internal/transport/websocket/websocket.go` | T2.1, T2.6 |
| `src/internal/session/session.go` | T2.4 |
| `src/internal/session/bolt.go` | T2.6 |
| `src/internal/routing/router.go` | T2.6 |
| `src/internal/routing/rule_set.go` | T2.6 |
| `src/internal/routing/domain_matcher.go` | T2.6 |
| `src/internal/config/server.go` | T3.1 |
| `src/internal/dns/resolver.go` | T3.2 |
| `src/internal/admin/` | T3.5 |
| `docker-compose.yml` | T3.3 |
| `.github/workflows/ci.yml` | T3.4 |
| `src/cmd/gatetest/main.go` | T2.6 |

## Implementation Context

- **Цель MVP**: устранить критические утечки ресурсов и уязвимости (AC-001–AC-006)
- **Границы приемки**: AC-001–AC-012
- **Ключевые правила**: zap через DI; ни одного log.Printf в internal/; sync.Once для Close; safe type assertions
- **Инварианты**: BoltDB schema не меняется; API не меняется; конфиг обратно совместим
- **Контракты/протокол**: без изменений
- **Proof signals**: `go test -race ./...`, `go vet ./...`, `grep` checks, docker-compose smoke test
- **Вне scope**: crypto, connection migration, тесты для config/proxy/logger

## Фаза 1: Подготовка инфраструктуры

Цель: внедрить zap-логгер во все пакеты, чтобы новый код сразу писал структурированные логи.

- [x] **T1.1** Пробросить `*zap.Logger` через конструкторы в routing, session, websocket пакеты; обновить все call sites в server/main.go и client/main.go. Touches: routing/router.go, routing/rule_set.go, routing/domain_matcher.go, session/session.go, session/bolt.go, websocket/websocket.go, server/main.go, client/main.go, routing/routing_test.go, routing/domain_matcher_test.go, routing/benchmark_test.go, session/session_test.go, admin/admin_test.go, websocket/websocket_test.go, websocket/dataframe_test.go, cmd/gatetest/main.go, cmd/stability/main.go

## Фаза 2: Критические ресурсные патчи (MVP)

Цель: устранить утечки горутин/fd и уязвимость slow-loris.

- [x] **T2.1** Добавить `SetWriteDeadline`/`SetReadDeadline` в циклы `serverWSToTun` и `serverTunToWS`. Дефолтный таймаут: 30s для туннеля. Покрывает AC-001. Touches: server/main.go, client/main.go, websocket/websocket.go

- [x] **T2.2** Добавить `ReadHeaderTimeout: 20 * time.Second` в `http.Server` конфигурацию. Покрывает AC-002. Touches: server/main.go

- [x] **T2.3** Сделать `BatchWriter.Close` идемпотентным через `sync.Once`. Покрывает AC-003. Touches: websocket/websocket.go

- [x] **T2.4** Добавить `defer sm.Stop()` после `sm.Start()` в main() и экспортировать `Stop()` метод. Покрывает AC-004. Touches: server/main.go, session/session.go

- [x] **T2.5** Заменить global `proxyStreams sync.Map` на per-session мапу; в defer сессии закрыть все net.Conn. Покрывает AC-005. Touches: server/main.go

- [x] **T2.6** Заменить все `log.Printf` на `zap.Logger` (через DI из T1.1) в пакетах: session/bolt.go, routing/router.go, routing/rule_set.go, routing/domain_matcher.go, websocket/websocket.go, gatetest/main.go. Покрывает AC-006. Touches: все указанные файлы + session/session.go

## Фаза 3: Операционные улучшения

Цель: hardening безопасности и наблюдаемости.

- [x] **T3.1** Заменить type assertions без `ok` в `convertRawTokens` на безопасные с проверкой. Покрывает AC-007. Touches: config/server.go (verified: code already used safe assertions, added trace marker)

- [x] **T3.2** Добавить `context.WithTimeout(ctx, 10*time.Second)` в DNS resolution. Покрывает AC-008. Touches: routing/domain_matcher.go

- [x] **T3.3** Заменить `privileged: true` на `cap_add: [NET_ADMIN, SYS_ADMIN]` в docker-compose.yml и examples/docker-compose.yml. Покрывает AC-009. Touches: docker-compose.yml, examples/docker-compose.yml

- [x] **T3.4** Добавить шаги `-race` и `gosec` в `.github/workflows/ci.yml`. Покрывает AC-010. Touches: .github/workflows/ci.yml

- [x] **T3.5** Зарегистрировать `pprof` handlers на админ-сервере. Покрывает AC-011. Touches: admin/admin.go

- [x] **T3.6** Добавить проверку BoltDB в `/health` endpoint. Покрывает AC-012. Touches: server/main.go

## Фаза 4: Проверка

Цель: доказать корректность и не допустить регрессий.

- [x] **T4.1** Написать тесты для новых таймаутов: `TestWebSocketDeadlines` (AC-001), `TestBatchWriterCloseIdempotent` (AC-003). AC-002 (ReadHeaderTimeout) и AC-005 (proxyStreams) верифицируются code review + интеграционным тестом. Touches: websocket_test.go

- [x] **T4.2** Написать тест для DNS таймаута: `TestDNSResolveTimeout` (AC-008). AC-007 (convertRawTokens) — код уже безопасен, верифицировано code review. Touches: dns/dns_test.go

- [x] **T4.3** Health endpoint (AC-012) верифицируется code review + docker-compose smoke test. /health — на main mux в server/main.go.

- [x] **T4.4** Выполнен финальный `go test -race ./...`, `go vet ./...`, grep checks. Все проходят. Touches: CI

## Покрытие критериев приемки

| AC | Задачи |
|----|--------|
| AC-001 | T2.1, T4.1 |
| AC-002 | T2.2, T4.1 |
| AC-003 | T2.3, T4.1 |
| AC-004 | T2.4 |
| AC-005 | T2.5, T4.1 |
| AC-006 | T1.1, T2.6 |
| AC-007 | T3.1, T4.2 |
| AC-008 | T3.2, T4.2 |
| AC-009 | T3.3 |
| AC-010 | T3.4 |
| AC-011 | T3.5 |
| AC-012 | T3.6, T4.3 |

## Заметки

- T1.1 — фундамент для T2.6. Выполнить строго первым.
- T2.1–T2.5 независимы, можно параллелить.
- T2.6 зависит от T1.1.
- T3.1–T3.6 независимы, можно параллелить после T1.1.
- T4.1–T4.4 — после завершения всех T2 и T3.
- Перед каждой задачей `@sk-task <slug>#<ID>` в изменённом коде; в тестах `@sk-test <slug>#<ID>`.
