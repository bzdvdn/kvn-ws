# Bootstrap Extraction & God-object Elimination Задачи

## Phase Contract

Inputs: plan, spec, существующая кодовая база.
Outputs: упорядоченные задачи с покрытием AC.
Stop if: задачи пересекаются с другими spec или невозможно сопоставить AC.

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/cmd/server/main.go | T2.1, T2.2 |
| src/cmd/client/main.go | T3.1 |
| src/internal/bootstrap/server/server.go, handler.go | T1.1, T1.2 |
| src/internal/bootstrap/client/client.go, tun.go, proxy.go, reconnect.go, killswitch.go | T3.1, T3.2, T3.3, T3.4, T3.5 |
| src/internal/tunnel/session.go | T2.2, T2.3 |
| src/internal/ratelimit/ratelimit.go | T1.2 |
| src/internal/proxy/stream.go | T4.1 |
| src/pkg/api/ | T4.2 |
| src/internal/bootstrap/server/handler.go (SIGHUP fix) | T4.3 |
| src/internal/bootstrap/client/proxy.go (nil-guard) | T4.4 |

## Implementation Context

- Цель MVP: cmd/server/main.go → 32 строки, Session переиспользуется
- Границы приемки: AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-007
- Ключевые правила: без глобального состояния, без дублирования data-path, cmd/ — только entrypoints
- Инварианты: Session одинаково работает для client и server; proxy — опционально; collectors — опционально
- Proof signals: go build/vet/test/gatetest pass
- Вне scope: изменение поведения, протокола, конфига

## Фаза 1: Основа

Цель: подготовить пакеты для server и ratelimit, создать базовую структуру.

- [x] T1.1 Создать internal/bootstrap/server/ — Server struct, New(), Run(), buildMux(). Touches: src/internal/bootstrap/server/server.go
- [x] T1.2 Извлечь rate-limiter в internal/ratelimit/ — IPRateLimiter + SessionPacketLimiter. Touches: src/internal/ratelimit/ratelimit.go

## Фаза 2: MVP Slice

Цель: cmd/server/main.go → тонкий entrypoint, Session извлечён в internal/tunnel/.

- [x] T2.1 Сократить cmd/server/main.go до вызова server.New(cfgPath).Run(ctx). Touches: src/cmd/server/main.go
- [x] T2.2 Создать internal/tunnel/session.go — Session struct из cmd/client, с nil-guards для sm/collectors/proxyStreams. Touches: src/internal/tunnel/session.go
- [x] T2.3 Подтвердить MVP — go build/vet/test/gatetest проходят. Touches: cmd/server/main.go, internal/tunnel/session.go, internal/bootstrap/server/

## Фаза 3: Основная реализация

Цель: client bootstrap извлечён, cmd/client/main.go → тонкий entrypoint.

- [x] T3.1 Создать internal/bootstrap/client/client.go — Client struct, New(), Run(), dispatch TUN/proxy. Touches: src/internal/bootstrap/client/client.go
- [x] T3.2 Добавить TUN reconnectLoop + runSession. Touches: src/internal/bootstrap/client/tun.go
- [x] T3.3 Добавить proxy reconnect runProxyMode + runProxySession. Touches: src/internal/bootstrap/client/proxy.go
- [x] T3.4 Добавить reconnect backoff computeGateway + sleepWithContext. Touches: src/internal/bootstrap/client/reconnect.go
- [x] T3.5 Добавить kill-switch applyKillSwitch + removeKillSwitch. Touches: src/internal/bootstrap/client/killswitch.go
- [x] T3.6 Сократить cmd/client/main.go до вызова client.New().Run(ctx). Touches: src/cmd/client/main.go

## Фаза 4: Чистка

Цель: убрать глобальное состояние, мёртвый код, починить баги.

- [x] T4.1 Приватизировать SessionStreams.M → m, добавить NewSessionStreams(). Touches: src/internal/proxy/stream.go
- [x] T4.2 Удалить pkg/api/. Touches: src/pkg/api/api.go
- [x] T4.3 Починить SIGHUP reload — startSighupHandler использует сохранённый cfgPath. Touches: src/internal/bootstrap/server/server.go
- [x] T4.4 Добавить nil-guard в FrameTypeProxy handler (proxyStreams). Touches: src/internal/tunnel/session.go
- [x] T4.5 Перенести proxySem из глобальной в поле Session. Touches: src/internal/tunnel/session.go, src/internal/proxy/stream.go
- [x] T4.6 Убрать дублированный data-path (tunToWS, wsToTun, tunReadInterruptible) из cmd/client. Touches: src/cmd/client/main.go

## Покрытие критериев приемки

- AC-001 (тонкие cmd/) → T2.1, T3.6, T4.6
- AC-002 (bootstrap пакеты) → T1.1, T3.1-T3.5
- AC-003 (data-path) → T2.2, T4.6
- AC-004 (сборка/тесты) → T2.3, T4.5, T4.6
- AC-005 (мёртвый код) → T4.2
- AC-006 (глобальный proxySem) → T4.5
- AC-007 (SIGHUP reload) → T4.3

## Заметки

- Все задачи выполнены — рефакторинг завершён.
- Каждая задача содержит trace-маркеры `@sk-task` в изменённых файлах.
- Следующий шаг: verify.
