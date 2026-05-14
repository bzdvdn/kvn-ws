# Production Readiness Hardening — План

## Phase Contract

Inputs: `spec.md`, repository context.
Outputs: plan, data model (no-change stub).
Stop if: spec расплывчата для безопасного планирования — нет, spec конкретна.

## Цель

Превратить kvn-ws из бета-кандидата в сервис, готовый к ограниченному production-режиму (до 500 сессий, uptime >72ч). Все изменения — патчи существующего кода без изменения архитектуры или добавления нового функционала.

## MVP Slice

- Устранение 6 критических утечек/уязвимостей: AC-001–AC-006.
- Результат: сервер не теряет горутины/fd, не падает от slow-loris, логи структурированы.

## First Validation Path

```bash
go test -race ./...           # все тесты + race detector
grep -r 'log\.Printf' src/    # только разрешённые места
docker-compose up --build     # smoke test: клиент подключается, пинг проходит
```

## Scope

- `src/cmd/server/main.go` — WebSocket deadlines, sm.Stop(), proxyStreams cleanup, ReadHeaderTimeout
- `src/internal/transport/websocket/` — BatchWriter sync.Once, log.Printf → zap, SetWriteDeadline
- `src/internal/session/` — sm.Stop() lifecycle, bolt.go log.Printf → zap
- `src/internal/routing/` — router.go, rule_set.go, domain_matcher.go log.Printf → zap
- `src/internal/config/server.go` — convertRawTokens safe type assertions
- `src/internal/dns/` — context timeout
- `src/internal/protocol/control/` — log.Printf → zap (websocket.go ping)
- `src/cmd/gatetest/main.go` — log.Printf → zap
- `docker-compose.yml`, `examples/docker-compose.yml` — privileged → capabilities
- `.github/workflows/ci.yml` — race + gosec
- `src/internal/admin/` — pprof registration

## Implementation Surfaces

| Surface | Изменения | Статус |
|---------|-----------|--------|
| `src/cmd/server/main.go` | deadlines, sm.Stop(), proxyStreams cleanup, ReadHeaderTimeout, pprof | существующая |
| `src/internal/transport/websocket/dataframe.go` | BatchWriter sync.Once Close | существующая |
| `src/internal/transport/websocket/websocket.go` | log.Printf → zap | существующая |
| `src/internal/session/session.go` | sm.Stop() public, reclaim lifecycle | существующая |
| `src/internal/session/bolt.go` | log.Printf → zap | существующая |
| `src/internal/routing/*.go` | log.Printf → zap | существующая |
| `src/internal/config/server.go` | safe type assertions | существующая |
| `src/internal/dns/resolver.go` | context timeout | существующая |
| `src/internal/admin/` | pprof handler registration | существующая |
| `docker-compose.yml` | privileged → capabilities | существующая |
| `.github/workflows/ci.yml` | race + gosec шаги | существующая |

## Bootstrapping Surfaces

`none` — вся нужная структура в репозитории уже есть.

## Влияние на архитектуру

- Локальное: zap-логгер пробрасывается глубже в пакеты routing, session, websocket. Меняется сигнатура конструкторов (zap.Logger parameter).
- Интеграции не затрагиваются.
- Миграций/rollout-последствий нет — все изменения обратно совместимы по конфигурации.

## Acceptance Approach

| AC | Подход | Surfaces | Validation |
|----|--------|----------|------------|
| AC-001 | Добавить SetWriteDeadline/SetReadDeadline в циклы wsToTun/tunToWS | `server/main.go`, `dataframe.go` | `TestWebSocketDeadlines` |
| AC-002 | ReadHeaderTimeout в http.Server | `server/main.go` | `TestReadHeaderTimeout` |
| AC-003 | sync.Once в BatchWriter.Close | `dataframe.go` | `TestBatchWriterCloseIdempotent` |
| AC-004 | defer sm.Stop() после sm.Start() | `server/main.go`, `session.go` | код-ревью |
| AC-005 | proxyStreams cleanup по сессии | `server/main.go` | `TestProxyStreamsCleanup` |
| AC-006 | Замена log.Printf на zap во всех пакетах | routing, session, websocket, gatetest | `grep` check |
| AC-007 | safe type assertions в convertRawTokens | `config/server.go` | `TestConvertRawTokensUnsafeTypes` |
| AC-008 | context.WithTimeout для DNS | `dns/resolver.go` | `TestDNSResolveTimeout` |
| AC-009 | privileged → capabilities | `docker-compose.yml` | `grep` check |
| AC-010 | race + gosec в CI | `.github/workflows/ci.yml` | CI run |
| AC-011 | pprof handlers | `admin/` | curl check |
| AC-012 | Health check dependencies | `admin/` | `TestHealthEndpoint` |

## Данные и контракты

- Data model не меняется.
- BoltDB schema не меняется.
- API/event contracts не меняются.
- `data-model.md` — no-change stub.

## Стратегия реализации

### DEC-001 Замена log.Printf на zap через DI

Why: глобальный логгер — антипаттерн, zap.Logger должен передаваться через конструктор для тестируемости.
Tradeoff: меняются сигнатуры New-функций в routing, session, websocket. Все call sites обновляются.
Affects: routing/router.go, routing/rule_set.go, routing/domain_matcher.go, session/session.go, session/bolt.go, websocket/websocket.go, server/main.go, gatetest/main.go, config/server.go
Validation: `grep -r 'log\.Printf' src/` не находит ничего в internal/ (кроме main.go)

### DEC-002 proxyStreams per-session вместо global sync.Map

Why: global sync.Map не чистится никогда — утечка fd при переподключениях. Хранить мапу в сессии и закрывать при defer.
Tradeoff: нужно передавать мапу в callback, небольшой оверхед на создание мапы на сессию.
Affects: server/main.go
Validation: `TestProxyStreamsCleanup` — 10 conns, disconnect, все closed.

### DEC-003 WebSocket deadlines через SetWriteDeadline перед каждой операцией

Why: gorilla/websocket не поддерживает context-aware Read/Write. Единственный способ — SetWriteDeadline/SetReadDeadline.
Tradeoff: небольшая задержка на syscall перед каждым Write. Для VPN-трафика — незначительно.
Affects: server/main.go, websocket.go
Validation: deadline срабатывает быстрее контекста сессии.

## Incremental Delivery

### MVP (Первая ценность)

- AC-001 deadlines
- AC-002 ReadHeaderTimeout
- AC-003 BatchWriter sync.Once
- AC-004 sm.Stop()
- AC-005 proxyStreams cleanup
- AC-006 log.Printf → zap

Результат: сервер не течёт ресурсами, защищён от slow-loris, логи структурированы.

### Итеративное расширение

- AC-007 safe convertRawTokens
- AC-008 DNS timeout
- AC-009 Docker capabilities
- AC-010 CI race + gosec
- AC-011 pprof
- AC-012 health deps

Каждый AC независим, порядок не важен.

## Порядок реализации

1. **Фаза 1**: log.Printf → zap (DEC-001) — основа для остальных изменений. Без неё новые логи будут в разных стилях.
2. **Фаза 2**: AC-001, AC-002, AC-003, AC-004, AC-005 — ресурсные патчи. Можно параллельно.
3. **Фаза 3**: AC-007, AC-008, AC-009, AC-010, AC-011, AC-012 — операционные улучшения. Независимы, можно параллельно.

## Риски

- **Риск 1**: Изменение сигнатур конструкторов (zap.Logger) сломает существующие вызовы.
  Mitigation: компилятор Go ловит все несоответствия. CI проверяет `go build ./...`.
- **Риск 2**: SetWriteDeadline может преждевременно обрывать соединение при высокой нагрузке.
  Mitigation: дефолтный таймаут 30s для туннеля, мониторинг через метрики.
- **Риск 3**: sync.Once в BatchWriter.Close не защищает от гонки между Write и Close.
  Mitigation: sync.Once гарантирует однократное закрытие канала. Write должен проверять флаг перед записью.

## Rollout и compatibility

- Все изменения обратно совместимы.
- Docker-compose смена privileged → capabilities требует пересоздания контейнера.
- CI изменения вступают в силу после merge.
- Специальных rollout-действий не требуется.

## Проверка

- `go test -race ./...` — все AC
- `go vet ./...` — code quality
- `grep -r 'log\.Printf' src/` — AC-006
- `grep 'privileged' docker-compose.yml` — AC-009
- `curl http://localhost:9090/debug/pprof/heap?debug=1` — AC-011
- `curl http://localhost:9090/health` — AC-012
- docker-compose up 1h smoke test — SC-001, SC-002

## Соответствие конституции

Нет конфликтов. Изменения соответствуют:
- Clean Architecture (zap через DI)
- Traceability (@sk-task / @sk-test)
- Наблюдаемость (structured logs, метрики)
- Graceful shutdown (sm.Stop, conn cleanup)
- Docker multi-stage (не меняется)
