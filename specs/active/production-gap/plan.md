# Production Gap План

## Phase Contract

Inputs: `specs/active/production-gap/spec.md`, `specs/active/production-gap/inspect.md` и минимальный repo-контекст вокруг TLS, config, examples, admin/metrics и verify surfaces.
Outputs: `plan.md`, `plan.digest.md`, `data-model.md`; дополнительные contracts не требуются, если не появится новый внешний API shape.
Stop if: в ходе реализации потребуется менять intent спеки или добавлять новый security workstream вне пяти `AC-*`.

## Цель

Реализация должна превратить текущий release-gap в узкий production-hardening pass, а не в широкую переработку продукта. Работа живёт в уже существующих клиентах конфигурации и transport TLS, в server runtime around admin/metrics, в безопасных examples и в Speckeep release artifacts, чтобы production proof получился через существующие поверхности и проверки.

## MVP Slice

- Минимальный independently demonstrable инкремент: устранить insecure client TLS default, выровнять server mTLS semantics, убрать tracked secret material из examples и восстановить формальный verify-path для slug.
- До любого расширения scope этот инкремент должен закрыть `AC-001`, `AC-002`, `AC-003`, `AC-004`.

## First Validation Path

- Сконфигурировать клиент на trusted CA/SNI, доказать accept trusted / reject untrusted через targeted TLS test или scripted e2e.
- Поднять сервер с `client_ca_file`, проверить accept trusted / reject unknown client cert.
- Прогнать проверку tracked examples на отсутствие private keys и запуск examples без закоммиченных секретов.
- Убедиться, что `tasks.md` и `verify.md` подготовлены так, что `./.speckeep/scripts/check-verify-ready.sh .` проходит.

## Scope

- Клиентский TLS trust path: config, runtime wiring, тесты и examples.
- Серверный mTLS verify path: режимы конфигурации, TLS builder, тесты и operator-facing examples.
- Secrets hygiene и operational proof: examples, token-gated `/metrics`, verify artifacts и release checks.
- Не трогаем tunnel protocol, routing logic, session model и новые product capabilities вне production-gap.

## Implementation Surfaces

- `src/internal/config/client.go` и связанные YAML examples/configs — добавление явной client TLS trust surface для CA/SNI/verify mode.
- `src/internal/transport/tls/tls.go` — центральная точка для client/server TLS semantics; сюда сходятся `AC-001` и `AC-002`.
- `src/cmd/client/main.go` — wiring client TLS config в runtime dial path.
- `src/internal/config/server.go` и `configs/server.yaml`/`examples/server.yaml` — нормализация mTLS режима и operational token assumptions.
- `src/cmd/server/main.go` — защита `/metrics`, повторное использование operational token gate, health/admin runtime orchestration.
- `src/internal/admin/admin.go` и `src/internal/admin/admin_test.go` — базовая reusable token-check surface для operational endpoints и proof существующего admin behavior.
- `examples/docker-compose.yml`, `examples/run.sh`, `examples/*.yaml`, `docker-compose.yml`, `run.sh`, `client.yaml`, `server.yaml`, `.gitignore` — безопасные root/example demo flows без tracked keys.
- `specs/active/production-gap/tasks.md` и `specs/active/production-gap/verify.md` — release-governance surfaces для `AC-004` и `AC-005`.

## Bootstrapping Surfaces

- none

## Влияние на архитектуру

- Архитектурное влияние локальное: меняются value-object-like config surfaces и TLS adapter behavior без перестройки bounded contexts.
- Интеграционное влияние затрагивает только runtime handshake до уровня TLS establishment и HTTP operational endpoints; framing/tunnel/session contracts не расширяются.
- Compatibility-последствие: старые insecure client configs перестанут молча работать и потребуют явной trusted TLS настройки; это допустимый breaking hardening в рамках production-gap.

## Acceptance Approach

- `AC-001` -> расширить client config surface и client TLS builder; результат наблюдается через accept trusted / reject untrusted tests и example config without insecure default.
- `AC-002` -> привести `client_auth` к явным режимам `request` / `require` / `verify` в server config и TLS builder; результат наблюдается через automated tests на trusted/unknown client cert и понятную operator-facing config semantics.
- `AC-003` -> убрать tracked private keys из root/example demo flows, перевести эти flows на runtime-generated certs или documented secret mount path, добавить guardrail для повторного коммита secrets; результат наблюдается через tracked file check и runnable root/example flow.
- `AC-004` -> подготовить полный slug artifact set для verify-path и привязать release evidence к текущему slug; результат наблюдается через успешный `check-verify-ready`.
- `AC-005` -> защитить `/metrics` shared token gate'ом, сохранить admin token semantics, прогнать privileged smoke/lint/security checks и зафиксировать proof в `verify.md`; результат наблюдается через endpoint auth checks и verify artifact.
- `AC-006` -> исправить все pre-existing lint issues (50 errcheck + 1 staticcheck) и подтвердить `golangci-lint run ./src/...` с exit 0; результат наблюдается через clean lint output.

## Данные и контракты

- `AC-001`, `AC-002`, `AC-005` требуют изменения configuration payload shapes и runtime validation rules.
- Data model меняется только на уровне конфигурационных value objects: client TLS trust settings, server client-auth semantics и operational token usage.
- API/event contracts туннеля, framing и session payload shapes не меняются.
- Отдельные `contracts/*` не требуются: новые внешние HTTP routes не добавляются, а существующие `/metrics` и admin routes сохраняют shape, меняя только access gate.

## Стратегия реализации

### DEC-001 Явная client TLS trust surface

Why: production TLS trust должен задаваться конфигом и использовать стандартную проверку цепочки/имени сервера, а не implicit insecure fallback.
Tradeoff: операторам придётся явно указывать trusted CA/SNI при self-managed сертификатах; dev-setup станет чуть строже.
Affects: `src/internal/config/client.go`, `src/internal/transport/tls/tls.go`, `src/cmd/client/main.go`, client YAML/examples.
Validation: targeted TLS tests и e2e proof показывают success только для trusted server cert.

### DEC-002 Явные mTLS режимы без двусмысленности

Why: текущая `require -> RequireAnyClientCert` semantics создаёт ложное чувство безопасности; режимы должны соответствовать operator intent.
Tradeoff: часть старых конфигов может потребовать migration по названию/ожиданиям режима.
Affects: `src/internal/config/server.go`, `src/internal/transport/tls/tls.go`, server config docs/examples, TLS tests.
Validation: automated tests показывают отличия `request`, `require`, `verify` и reject unknown cert в production paths.

### DEC-003 Shared operational token gate

Why: `/metrics` и admin surface уже находятся в одном operational perimeter; shared token gate минимизирует объём work и не создаёт новую auth subsystem.
Tradeoff: один operational token защищает две поверхности; более тонкое разделение прав остаётся вне scope.
Affects: `src/cmd/server/main.go`, `src/internal/admin/admin.go`, related tests/config examples.
Validation: `/metrics` и admin endpoints возвращают auth failure без токена и success с валидным токеном.

### DEC-005 Lint quality gate — 0 pre-existing issues

Why: `golangci-lint` выявил 51 pre-existing issue (50 errcheck + 1 staticcheck) во всём репозитории; без их исправления lint gate блокирует archive, хотя issues не относятся к production-gap.
Tradeoff: исправление errcheck в defer-выражениях и test-хелперах может быть многословным, но не меняет поведение.
Affects: `src/cmd/client/main.go`, `src/cmd/server/main.go`, `src/internal/admin/admin.go`, `src/internal/admin/admin_test.go`, `src/internal/proxy/listener.go`, `src/internal/proxy/stream.go`, `src/internal/session/bolt.go`, `src/internal/transport/websocket/*.go`, `src/internal/tun/*.go`, `src/internal/config/server.go`.
Validation: `golangci-lint run ./src/...` exit 0.

### DEC-004 Release proof через существующие surfaces

Why: фича направлена на release-readiness, поэтому лучше усилить уже существующие test/verify surfaces, чем строить отдельный orchestration layer.
Tradeoff: verify будет зависеть от дисциплины ведения артефактов и reproducible scripts.
Affects: `scripts/test-security.sh`, `scripts/test-gate.sh`, `.github/workflows/ci.yml`, `specs/active/production-gap/verify.md`.
Validation: `check-verify-ready`, smoke, lint и targeted tests дают observable proof без ручных допущений.

## Incremental Delivery

### MVP (Первая ценность)

- Внедрить client TLS trust config и enforcement.
- Исправить server mTLS semantics и reject unknown client cert.
- Очистить root/example demo flows от tracked secret material и перевести их на безопасный запуск.
- Подготовить slug artifacts так, чтобы verify path больше не ломался.
- Критерий готовности MVP: закрыты `AC-001`..`AC-004`, а базовая manual/scripted validation проходит без privileged runtime.

### Итеративное расширение

- Добавить shared token gate на `/metrics` и зафиксировать expected admin/metrics auth behavior.
- Прогнать privileged smoke для TUN/NAT/reconnect и quality gates, затем собрать финальный `verify.md`.
- Критерий готовности расширения: закрыт `AC-005`, release review имеет полный observable proof.

### Lint quality gate

- Исправить 51 pre-existing issue (50 errcheck + 1 staticcheck) во всех затронутых файлах.
- Проверить `golangci-lint run ./src/...` — 0 issues.
- Критерий готовности: закрыт `AC-006`, lint gate exit 0.

## Порядок реализации

- Сначала client TLS trust path, потому что он убирает P0 insecure default и задаёт config surface для examples/tests.
- Затем server mTLS semantics, поскольку она тесно связана с TLS fixtures и тем же test harness.
- После этого secrets hygiene в examples, чтобы сразу перевести примеры на новый trusted flow.
- Далее operational token gate и финальные security/runtime checks.
- В конце — Speckeep tasks/verify artifacts и сводка release proof.
- После фиксации proof — исправление pre-existing lint issues (50 errcheck + 1 staticcheck) и финальный lint quality gate.

## Риски

- Риск 1: migration friction для существующих локальных/dev конфигов, которые рассчитывают на insecure TLS.
  Mitigation: explicit config fields, обновлённые examples и targeted tests на trusted dev flow.
- Риск 2: попытка расширить scope до полноценного RBAC/ops-plane redesign из-за `/metrics` hardening.
  Mitigation: удерживать shared token gate как единственное решение в рамках `AC-005` и не добавлять новые роли/эндпоинты.
- Риск 3: release proof останется неполным из-за несогласованности code/tests/spec artifacts.
  Mitigation: готовить `tasks.md` и `verify.md` как часть implementation path, а не как постфактум.

## Rollout и compatibility

- Специальный feature flag не нужен; rollout идёт через обновление конфигурации и examples.
- Нужно явно донести compatibility note: insecure client TLS defaults больше не поддерживаются, а `/metrics` требует operational token.
- После релиза оператору нужно проверить наличие trusted CA/material и operational token перед первым запуском.

## Проверка

- Automated tests: обновить/добавить TLS unit/integration tests для client trust и mTLS semantics; это подтверждает `AC-001`, `AC-002`, `DEC-001`, `DEC-002`.
- Endpoint/security tests: обновить admin/metrics auth checks и example/secrets guardrails; это подтверждает `AC-003`, `AC-005`, `DEC-003`.
- Quality/runtime checks: `golangci-lint` (0 issues), targeted `go test`, privileged smoke для TUN/NAT/reconnect, `check-verify-ready`; это подтверждает `AC-004`, `AC-005`, `AC-006`, `DEC-004`, `DEC-005`.
- Review evidence: `verify.md` должен собрать ссылки на команды, тесты и изменённые файлы для всех пяти `AC-*`.

## Соответствие конституции

- нет конфликтов: план остаётся в одном slug, использует существующие bounded contexts, требует observable proof, сохраняет двуязычную docs-policy и не выходит за пределы заявленного scope.
