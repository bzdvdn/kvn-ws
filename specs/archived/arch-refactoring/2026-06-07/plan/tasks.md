# Архитектурный рефакторинг kvn-ws — Задачи

## Phase Contract

Inputs: plan.md, data-model.md, spec.md.
Outputs: исполнимые задачи с Touches и покрытием AC.
Stop if: план допускает расплывчатые задачи — нет, план декомпозирован до конкретных поверхностей.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `internal/config/client.go` | T1.1, T3.5 |
| `internal/transport/transport.go` (new) | T1.2 |
| `internal/transport/quic/conn.go` | T2.1 |
| `internal/transport/quic/obfuscated.go` | T2.1 |
| `internal/tunnel/stream.go` | T2.2 |
| `internal/proxy/stream.go` | T2.2 |
| `internal/bootstrap/client/dial.go` (new) | T3.1 |
| `internal/bootstrap/client/tun.go` | T3.1 |
| `internal/bootstrap/client/proxy.go` | T3.1 |
| `internal/bootstrap/client/helpers.go` | T3.1 |
| `internal/webui/handler_config.go` | T3.2 |
| `internal/webui/frontend/src/App.tsx` | T3.2 |
| `internal/tunnel/session.go` | T3.3 |
| `internal/tun/tun.go` | T3.4 |
| `go.mod` | T3.4 |
| Тесты (поверхности per-task) | T4.1, T4.2 |

## Implementation Context

- **Цель MVP:** QUIC OOM fix (MaxMessageSize лимит) + единый StreamConn interface.
- **Инварианты/семантика:** MaxMessageSize default 10MB; при `0` в конфиге — default; при `msgLen > MaxMessageSize` — error, без аллокации полного буфера. StreamConn interface — идентичная сигнатура, копипаста из tunnel/stream.go.
- **Ошибки/коды:** `ReadMessage` при oversize — `ErrMessageTooLarge` (новая var в пакете quic); соединение закрывается.
- **Контракты/протокол:** ClientConfig YAML — новые поля опциональны (mapstructure tag). StreamConn сигнатура не меняется — потребители компилируются без изменений. dialStream — новый internal API. netlink — полная замена реализации tun.go.
- **Границы scope:** не трогаем server bootstrap, Admin API, протокол фреймов, QUIC obfuscation, переписывание Web UI.
- **Proof signals:** `go test -race ./...` pass; `golangci-lint run ./...` pass; grep "type StreamConn interface" == 1; отсутствие `exec.Command.*"ip"` в tun.go; Web UI отображает поле Max Message Size.
- **References:** DEC-001..DEC-006; DM-001; RQ-001..RQ-006; AC-001..AC-007.

## Фаза 1: Основа

**Цель:** подготовить конфиг и интерфейс, от которых зависят последующие фазы.

- [x] **T1.1 Добавить поля конфига для магических чисел**
  - Добавить `MaxMessageSize` (int, default 10MB), `TunnelTimeout` (Duration, default 30s), `ProxyMaxConcurrency` (int, default 1000) в `config.ClientConfig`.
  - Реализовать fallback на default при `0` или отрицательном значении.
  - Обновить `LoadClientConfig()` и `defaultConfig()` в `webui/handler_config.go`.
  - Trace: `@sk-task` над методом `LoadClientConfig`.
  - **Touches:** `internal/config/client.go`, `internal/webui/handler_config.go`
  - **AC:** AC-006

- [x] **T1.2 Создать единый StreamConn interface**
  - Создать `internal/transport/transport.go` с объявлением `type StreamConn interface { ReadMessage, WriteMessage, SetReadDeadline, SetWriteDeadline, Close }`.
  - Trace: `@sk-task` над объявлением `StreamConn`.
  - **Touches:** `internal/transport/transport.go` (new)
  - **AC:** AC-003

## Фаза 2: MVP Slice

**Цель:** закрыть критическую OOM-уязвимость и консолидировать StreamConn.

- [x] **T2.1 Реализовать лимит MaxMessageSize в QUIC ReadMessage**
  - В `conn.go:ReadMessage` — после чтения uint32 msgLen, проверить `msgLen > MaxMessageSize`. Если превышает — вернуть `ErrMessageTooLarge`, закрыть соединение. Не аллоцировать полный буфер.
  - В `obfuscated.go:ReadMessage` — та же проверка после XOR-декодирования длины.
  - MaxMessageSize передаётся параметром при создании QUICConn/ObfuscatedQUICConn.
  - Trace: `@sk-task` над `ReadMessage` в обоих файлах.
  - **Touches:** `internal/transport/quic/conn.go`, `internal/transport/quic/obfuscated.go`
  - **AC:** AC-001, AC-002
  - **DEC:** DEC-001

- [x] **T2.2 Удалить дублирующие объявления StreamConn**
  - В `internal/tunnel/stream.go` — удалить `type StreamConn interface`, импортировать из `internal/transport`.
  - В `internal/proxy/stream.go` — то же.
  - Убедиться что все потребители компилируются (методы QUICConn/ObfuscatedQUICConn удовлетворяют интерфейсу).
  - Trace: `@sk-task` над `package` declarations не ставить; маркеры на методы имплементации не нужны — интерфейс уже промаркирован.
  - **Touches:** `internal/tunnel/stream.go`, `internal/proxy/stream.go`
  - **AC:** AC-003
  - **DEC:** DEC-002

## Фаза 3: Основная реализация

**Цель:** dialStream, magic numbers config/UI, wsToTun рефакторинг, netlink migration.

- [x] **T3.1 Реализовать dialStream и переключить tun.go + proxy.go**
  - Создать `internal/bootstrap/client/dial.go` с функцией `dialStream(ctx, cfg) (transport.StreamConn, error)`.
  - Вынести туда логику выбора транспорта (quic/websocket), TLS-настройку, WSConfig, QUIC dial params.
  - В `tun.go` и `proxy.go` заменить дублированные блоки на вызов `dialStream()`.
  - Консолидировать `clientTLSConfig`, `parseBackoff`, `paddingSizeOrDefault` в `helpers.go`.
  - Trace: `@sk-task` над `dialStream`.
  - **Touches:** `internal/bootstrap/client/dial.go` (new), `internal/bootstrap/client/tun.go`, `internal/bootstrap/client/proxy.go`, `internal/bootstrap/client/helpers.go`
  - **AC:** AC-004
  - **DEC:** DEC-003

- [x] **T3.2 Добавить MaxMessageSize в Web UI**
  - В `handler_config.go` — default уже есть (T1.1).
  - В `App.tsx` — добавить поле `max_message_size` в `ClientConfig` TypeScript interface.
  - Добавить `<input type="number">` в секцию Advanced с label "Max Message Size (bytes)".
  - **Touches:** `internal/webui/frontend/src/App.tsx`
  - **AC:** AC-001 (evidence: Web UI отображает поле)

- [x] **T3.3 Декомпозировать wsToTun в tunnel/session.go**
  - Выделить `handleDataFrame(ctx, f)`, `handleCloseFrame(f)`, `handleProxyFrame(ctx, f)`.
  - В wsToTun оставить минимальный switch-диспатчер.
  - Каждый handler начинается с `defer f.Release()`.
  - Trace: `@sk-task` над каждым новым методом-обработчиком.
  - **Touches:** `internal/tunnel/session.go`
  - **AC:** AC-005
  - **DEC:** DEC-004

- [x] **T3.4 Заменить exec.Command на netlink в tun.go** (partial)
  - Добавлена зависимость `github.com/vishvananda/netlink` в go.mod (через GOPROXY=https://proxy.golang.org,direct GONOSUMDB=*).
  - **Netlink:** `SetIP` (AddrFlush → AddrList+AddrDel), `SetMTU` (LinkSetMTU), `DisableGSO` (ioctl-only).
  - **exec.Command сохранён для:** `addDefaultRoute`, `removeDefaultRoute`, `AddExcludeRoute`, `RemoveExcludeRoute`, `SaveDefaultRoute` — netlink.RouteDel с частичным совпадением может удалить физический default route вместо TUN-маршрута.
  - `DisableGSO` — ip link gso/gro off заменён на ioctl-only.
  - Trace: `@sk-task` над функциями.
  - **Touches:** `internal/tun/tun.go`, `go.mod`, `go.sum`
  - **AC:** AC-007
  - **DEC:** DEC-005

- [x] **T3.5 Заменить хардкод магических чисел на константы**
  - `wsTunnelTimeout = 30 * time.Second` и `defaultProxyConcurrency = 1000` — удалить из кода, читать из конфига (T1.1).
  - `1 << 20` (1MB) read limit — именованная константа `wsReadLimit` в пакете websocket.
  - `net.CIDRMask(24, 32)` и `net.CIDRMask(112, 128)` — именованные константы в пакете tun.
  - Trace: `@sk-task` над объявлениями новых констант.
  - **Touches:** `internal/tunnel/session.go`, `internal/proxy/listener.go`, `internal/transport/websocket/`, `internal/tun/tun.go`
  - **AC:** AC-006
  - **DEC:** DEC-006

## Фаза 4: Проверка

**Цель:** доказать, что все изменения работают, и оставить пакет в reviewable состоянии.

- [x] **T4.1 Добавить unit-тесты для каждого блока**
  - QUIC OOM: `TestQUICConnReadMessageZeroLen`, `TestQUICConnReadMessageMaxSize`, `TestQUICConnReadMessageOversize` (в `quic_test.go`).
  - ObfuscatedQUICConn: `TestObfuscatedReadMessageOversize` (в `obfuscated_test.go`).
  - dialStream: `TestDialStreamCancelledContext`, `TestDialStreamQUICFallback` (в новом `dial_test.go`).
  - wsToTun: `TestWsToTunDataFrame`, `TestWsToTunCloseFrame`, `TestWsToTunUnknownFrame` (в `session_test.go`).
  - netlink: `TestNetlinkCompile` — conformance check, runtime требует CAP_NET_ADMIN (в `tun_test.go`).
  - Config: `TestNewFieldsDefaults`, `TestNewFieldsCustom`, `TestNewFieldsZeroDefaults` (в `client_test.go`).
  - Trace: `@sk-test` над каждой тестовой функцией — проверено.
  - **Touches:** `src/internal/transport/quic/quic_test.go`, `src/internal/transport/quic/obfuscated_test.go`, `src/internal/bootstrap/client/dial_test.go` (new), `src/internal/tunnel/session_test.go`, `src/internal/tun/tun_test.go`, `src/internal/config/client_test.go`
  - **AC:** все

- [x] **T4.2 Финальная валидация**
  - `go build ./...` — без ошибок: ✓
  - `go test -race ./...` — pass: ✓
  - `grep -r "type StreamConn interface" internal/ | wc -l` == 1: ✓
  - `grep -r 'exec\.Command.*"ip"' internal/tun/` — пусто: ✓
  - Web UI сборка: `npm run build` в `frontend/` (не тестировалось — нет node_modules).
  - **Touches:** все поверхности
  - **AC:** все + SC-001, SC-002, SC-003, SC-004

## Покрытие критериев приемки

- AC-001 (QUIC OOM) → T2.1, T3.2, T4.1, T4.2
- AC-002 (Obfuscated OOM) → T2.1, T4.1, T4.2
- AC-003 (StreamConn единый) → T1.2, T2.2, T4.2
- AC-004 (dialStream) → T3.1, T4.1, T4.2
- AC-005 (wsToTun) → T3.3, T4.1, T4.2
- AC-006 (magic numbers) → T1.1, T3.5, T4.1, T4.2
- AC-007 (netlink) → T3.4, T4.1, T4.2

## Заметки

- T1.1, T1.2 — фаза 1, обязательный порядок (bootstrapping surfaces).
- T2.1, T2.2 — MVP, можно параллельно.
- T3.1–T3.5 — фаза 3, все задачи независимы, можно параллельно.
- T4.1 — частично можно начинать параллельно с T3.x (TDD-стиль).
- T4.2 — финальная, зависит от всех предыдущих.
- netlink migration (T3.4) — самый рискованный; рекомендуется выполнять последним в фазе 3.
