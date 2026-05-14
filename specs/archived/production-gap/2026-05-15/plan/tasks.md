# Production Gap Задачи

## Phase Contract

Inputs: `plan.md`, `plan.digest.md`, `data-model.md` и текущая спецификация slug `production-gap`.
Outputs: упорядоченные исполнимые задачи с покрытием `AC-*`, достаточные для реализации production hardening без расширения scope.
Stop if: любая задача требует новой product-surface вне зафиксированных `DEC-*` / `DM-*` или не удаётся привязать `AC-*` к наблюдаемому proof.

## Implementation Context

- Цель MVP: закрыть `AC-001`..`AC-004` через client TLS trust, корректный server mTLS, безопасные root/example demo flows и готовый verify path.
- Инварианты/семантика:
  - `DEC-001`/`DM-001`: клиент не имеет insecure TLS verify как default.
  - `DEC-002`/`DM-002`: `request`, `require`, `verify` различаются явно; trust-enforcing режимы не принимают unknown client cert.
  - `DEC-003`/`DM-003`: `/metrics` и admin surface защищаются общим token gate.
  - Secrets hygiene scope ограничен текущим tree и release artifacts; очистка git-истории вне scope.
- Контракты/протокол:
  - клиентский trust path живёт в `ClientConfig` + TLS builder + runtime wiring.
  - server operational endpoints сохраняют текущие routes; меняется только access gate.
  - verify proof собирается в `specs/active/production-gap/verify.md`.
- Границы scope:
  - не добавляем новые transport/auth features, RBAC или новые admin endpoints.
  - не меняем framing, session payloads, routing engine и persisted session model.
- Proof signals:
  - trusted server cert принимается, untrusted cert отвергается.
  - unknown client cert отвергается в trust-enforcing mTLS режиме.
  - tracked root/example demo flows не содержат private keys, а `check-verify-ready` и финальные quality checks проходят.
- References: `AC-001`..`AC-005`, `DEC-001`..`DEC-004`, `DM-001`..`DM-003`

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/config/client.go`, `src/internal/transport/tls/tls.go`, `src/cmd/client/main.go`, `configs/client.yaml`, `examples/client.yaml`, `client.yaml`, `src/internal/transport/tls/tls_test.go` | T1.1, T2.1, T2.2 |
| `src/internal/config/server.go`, `src/internal/transport/tls/tls.go`, `configs/server.yaml`, `examples/server.yaml`, `server.yaml`, `src/internal/transport/tls/tls_test.go` | T1.2, T2.1, T2.2 |
| `src/internal/admin/admin.go`, `src/internal/admin/admin_test.go`, `src/cmd/server/main.go` | T3.1, T4.1 |
| `examples/docker-compose.yml`, `examples/run.sh`, `examples/*.yaml`, `docker-compose.yml`, `run.sh`, `client.yaml`, `server.yaml`, `.gitignore` | T2.3, T4.1 |
| `src/internal/transport/websocket/websocket_test.go`, `src/internal/admin/admin_test.go`, `scripts/test-security.sh`, `scripts/test-gate.sh` | T2.2, T3.2, T4.1 |
| `specs/active/production-gap/tasks.md`, `specs/active/production-gap/verify.md` | T1.3, T4.2 |

## Фаза 1: Основа

Цель: выровнять конфигурационные и process surfaces, от которых зависят все последующие проверки.

- [x] T1.1 Добавить явную client TLS trust surface для CA / server identity / verify semantics, чтобы runtime больше не зависел от insecure default. Touches: `src/internal/config/client.go`, `src/internal/transport/tls/tls.go`, `src/cmd/client/main.go`, `configs/client.yaml`, `examples/client.yaml`, `client.yaml`, `src/internal/transport/tls/tls_test.go`
- [x] T1.2 Нормализовать server mTLS config semantics для `request` / `require` / `verify`, включая требования к `client_ca_file` для trust-enforcing режимов. Touches: `src/internal/config/server.go`, `src/internal/transport/tls/tls.go`, `configs/server.yaml`, `examples/server.yaml`, `server.yaml`, `src/internal/transport/tls/tls_test.go`
- [x] T1.3 Подготовить release-governance baseline текущего slug, чтобы verify-path имел ожидаемые артефакты ещё до финального proof. Touches: `specs/active/production-gap/tasks.md`, `specs/active/production-gap/verify.md`

## Фаза 2: MVP Slice

Цель: поставить минимальную independently reviewable ценность, закрывающую security P0 и process-blockers до operational hardening.

- [x] T2.1 Реализовать client/server TLS behavior end to end: trusted server cert принимается, unknown client cert отвергается в trust-enforcing mTLS режиме. Touches: `src/internal/transport/tls/tls.go`, `src/cmd/client/main.go`, `src/internal/config/client.go`, `src/internal/config/server.go`
- [x] T2.2 Добавить targeted automated proof для TLS trust и mTLS semantics, включая reject paths и различие режимов `request` / `require` / `verify`. Touches: `src/internal/transport/websocket/websocket_test.go`, `src/internal/transport/tls/tls.go`, `src/internal/config/server.go`, `src/internal/config/client.go`
- [x] T2.3 Убрать tracked secret material из root/example demo flows и перевести эти flow на runtime-generated certs или documented secret mount без закоммиченных private keys. Touches: `examples/docker-compose.yml`, `examples/run.sh`, `examples/client.yaml`, `examples/server.yaml`, `docker-compose.yml`, `run.sh`, `client.yaml`, `server.yaml`, `.gitignore`

## Фаза 3: Основная реализация

Цель: закрыть оставшийся production-hardening scope поверх MVP без выхода в новые feature workstreams.

- [x] T3.1 Защитить `/metrics` тем же token gate, что и operational admin perimeter, сохранив существующие routes и без введения новой auth subsystem. Touches: `src/cmd/server/main.go`, `src/internal/admin/admin.go`, `src/internal/config/server.go`
- [x] T3.2 Обновить security/runtime checks под новый hardening path: auth behavior для `/metrics`, smoke/security scripts и observable endpoint proof. Touches: `src/internal/admin/admin_test.go`, `scripts/test-security.sh`, `scripts/test-gate.sh`, `src/cmd/server/main.go`

## Фаза 4: Проверка

Цель: зафиксировать полный observable proof и оставить slug готовым к verify/implement review.

- [x] T4.1 Прогнать и зафиксировать automated coverage для TLS, mTLS, examples-secrets hygiene и token-gated operational endpoints. Touches: `src/internal/transport/websocket/websocket_test.go`, `src/internal/admin/admin_test.go`, `scripts/test-security.sh`, `examples/run.sh`
- [x] T4.2 Собрать release evidence в `verify.md`, включая `check-verify-ready`, targeted tests, lint и privileged smoke results по `AC-004` / `AC-005`. Touches: `specs/active/production-gap/verify.md`, `specs/active/production-gap/tasks.md`

## Фаза 5: Lint gate

Цель: исправить pre-existing lint issues (50 errcheck + 1 staticcheck) во всём репозитории для прохождения lint quality gate.

- [x] T5.1 Исправить errcheck issues в `src/cmd/client/main.go` (6 unchecked Close) и `src/cmd/server/main.go` (10 unchecked Close/Write/Encode). Touches: `src/cmd/client/main.go`, `src/cmd/server/main.go`
- [x] T5.2 Исправить errcheck issues в `src/internal/admin/admin.go` (2 unchecked Encode/Fprintf) и `src/internal/admin/admin_test.go` (1 unchecked Decode). Touches: `src/internal/admin/admin.go`, `src/internal/admin/admin_test.go`
- [x] T5.3 Исправить errcheck issues в `src/internal/proxy/listener.go` (4 unchecked Close/Write) и `src/internal/proxy/stream.go` (1 unchecked Write). Touches: `src/internal/proxy/listener.go`, `src/internal/proxy/stream.go`
- [x] T5.4 Исправить errcheck issues в `src/internal/session/bolt.go` (2 unchecked Close). Touches: `src/internal/session/bolt.go`
- [x] T5.5 Исправить errcheck issues в `src/internal/transport/websocket/websocket.go` (5 unchecked Flush/SetNoDelay/SetCompressionLevel) и `src/internal/transport/websocket/websocket_test.go` (6 unchecked Close/Flush). Touches: `src/internal/transport/websocket/websocket.go`, `src/internal/transport/websocket/websocket_test.go`
- [x] T5.6 Исправить errcheck issues в `src/internal/transport/websocket/dataframe_test.go` (2 unchecked Close). Touches: `src/internal/transport/websocket/dataframe_test.go`
- [x] T5.7 Исправить errcheck issues в `src/internal/tun/tun.go` (1 unchecked Close) и `src/internal/tun/tun_test.go` (8 unchecked Close/Open/Write). Touches: `src/internal/tun/tun.go`, `src/internal/tun/tun_test.go`
- [x] T5.8 Исправить staticcheck S1020 в `src/internal/config/server.go` (лишняя проверка ok после type assertion). Touches: `src/internal/config/server.go`
- [x] T5.9 Прогнать финальный `golangci-lint run ./src/...` и зафиксировать результат в verify.md. Touches: `specs/active/production-gap/verify.md`

## Покрытие критериев приемки

- AC-001 -> T1.1, T2.1, T2.2, T4.1
- AC-002 -> T1.2, T2.1, T2.2, T4.1
- AC-003 -> T2.3, T4.1
- AC-004 -> T1.3, T4.2
- AC-005 -> T3.1, T3.2, T4.1, T4.2
- AC-006 -> T5.1, T5.2, T5.3, T5.4, T5.5, T5.6, T5.7, T5.8, T5.9
- AC-007 -> T6.1, T6.2, T6.3, T6.4, T6.5

## Фаза 6: Production runtime safety

Цель: устранить resource leaks и добавить http.Server таймауты для production-безопасности.

- [x] T6.1 Добавить ReadTimeout/WriteTimeout/IdleTimeout на http.Server для защиты от slow loris. Touches: `src/cmd/server/main.go`
- [x] T6.2 Добавить defer natMgr.Teardown() и Teardown6() для удаления NFTables правил при завершении. Touches: `src/cmd/server/main.go`
- [x] T6.3 Вынести boltStore/boltStore6 на уровень main() и добавить defer Close() при завершении. Touches: `src/cmd/server/main.go`
- [x] T6.4 Добавить startCleanup goroutine для ipRateLimiter и sessionPacketLimiter — удаление неиспользуемых entries раз в 10 минут. Touches: `src/cmd/server/main.go`
- [x] T6.5 Обновить golang.org/x/net c v0.43.0 до v0.54.0 для закрытия известных CVE. Touches: `go.mod`, `go.sum`

## Заметки

- Порядок задач сохраняет MVP-first sequencing: сначала TLS trust и mTLS semantics, затем root/example demo hygiene, затем operational token gate и финальный proof.
- `T1.3` существует не ради процессной бюрократии, а чтобы implementation не уткнулась в missing `verify.md` на позднем этапе.
- Если в implement появится идея разделить токены для `/metrics` и admin surface, это scope change и требует возврата к `spec/plan`, а не тихого расширения.
