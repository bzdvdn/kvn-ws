# Windows TUN Device Support — План

## Phase Contract

Inputs: spec, inspect-pass, минимальный repo-контекст.
Outputs: plan, data-model stub.
Stop if: spec слишком расплывчата — spec пройдена inspect pass.

## Цель

Реализовать `TunDevice` для Windows через Wintun + winipcfg. Существующий интерфейс не меняется — добавляется platform-specific файл `tun_windows.go`. Bootstrap слой (`bootstrap/client/tun.go`) не требует модификации, т.к. уже работает через абстракцию `TunDevice`.

## MVP Slice

Wintun adapter create + read/write data path + IP/MTU assignment + default route + web UI TUN selector.

AC-001, AC-002, AC-003, AC-005, AC-008, AC-009, AC-011 — закрываются в MVP.

## First Validation Path

1. Cross-compile `GOOS=windows GOARCH=amd64 go build ./src/cmd/client` на Linux — успех
2. Cross-compile `GOOS=windows GOARCH=amd64 go build ./src/cmd/web` на Linux — успех
3. На Windows: запустить `kvn-web.exe` → открыть UI → селектор режимов показывает "TUN"
4. Выбрать TUN → Connect → TUN-интерфейс появляется в `ipconfig`
5. Ping до gateway через TUN — ICMP-пакеты читаются из Wintun ring buffer

## Scope

- `src/internal/tun/tun_windows.go` — новая Windows-реализация `TunDevice`
- `src/internal/tun/tun_stub.go` — build tag меняется с `!linux` на `!linux,!windows`
- `go.mod` — добавление `golang.zx2c4.com/wireguard/windows` (winipcfg)
- `scripts/build.sh` — добавление `windows` target
- `src/internal/webui/server.go` — `/api/platform` возвращает `tun_supported`
- `src/internal/webui/handler_connect.go` — снятие блокировки TUN на Windows
- `src/internal/webui/frontend/src/TabbedForm.tsx` — селектор режима учитывает `tun_supported`
- `src/internal/webui/frontend/src/context.tsx` — проброс `tun_supported` в состояние
- `src/internal/webui/frontend/src/types.ts` — тип для `tun_supported`
- `data-model` — не меняется

### Границы, которые не меняются

- `TunDevice` interface (`tun_common.go`)
- `bootstrap/client/tun.go` — платформонезависим
- `tun_offload_linux.go` / `tun_offload_other.go`
- `tun_test.go` (MockTunDevice) — уже кроссплатформенный
- `src/cmd/desktop/` — Desktop UI обёртка, не меняется

## Performance Budget

- `none` — для MVP нет performance-ограничений (batch read/write, spin-lock оптимизации вне scope)

## Implementation Surfaces

| Surface | Тип | Почему |
|---|---|---|
| `src/internal/tun/tun_windows.go` | NEW | Windows impl `TunDevice` через Wintun |
| `src/internal/tun/tun_stub.go` | MODIFY | build tag `!linux,!windows` вместо `!linux` |
| `go.mod` | MODIFY | добавить `golang.zx2c4.com/wireguard/windows` |
| `scripts/build.sh` | MODIFY | добавить `case "windows"` с `GOOS=windows` |
| `src/internal/webui/server.go` | MODIFY | добавить `tun_supported` в `/api/platform` |
| `src/internal/webui/handler_connect.go` | MODIFY | убрать блокировку TUN для Windows |
| `src/internal/webui/frontend/src/TabbedForm.tsx` | MODIFY | селектор режима по `tun_supported` |
| `src/internal/webui/frontend/src/context.tsx` | MODIFY | проброс `tun_supported` |
| `src/internal/webui/frontend/src/types.ts` | MODIFY | тип `tun_supported?: boolean` |

## Bootstrapping Surfaces

- `none` — структура `src/internal/tun/` уже существует

## Влияние на архитектуру

- Локальное: новый platform-specific файл в пакете `tun`
- Web UI: добавление `tun_supported` в `/api/platform` и условный селектор режима — обратно совместимо
- Нет изменений в интерфейсах, контрактах, data model
- Winipcfg — изолирован в `tun_windows.go`, не просачивается в другие пакеты

## Acceptance Approach

| AC | Подход | Surfaces | Валидация |
|---|---|---|---|
| AC-001 | Wintun adapter create + SetIP | `tun_windows.go`, winipcfg | `ipconfig` на Windows |
| AC-002 | read/write loop | `tun_windows.go` | loopback тест (write -> read) |
| AC-003 | default route via winipcfg | `tun_windows.go` | `route print -4` |
| AC-004 | exclude routes on physical iface | `tun_windows.go` | `route print` до/после |
| AC-005 | SetMTU via winipcfg | `tun_windows.go` | `netsh interface ipv4 show interfaces` |
| AC-006 | deterministic GUID (UUIDv5) | `tun_windows.go` | GUID не меняется между запусками |
| AC-007 | Close cleans up adapter+routes | `tun_windows.go` | `ipconfig` после Close |
| AC-008 | cross-compile | `scripts/build.sh` | `file client.exe` = PE32+ |
| AC-009 | stub error on non-Windows | `tun_stub.go` | `go test ./src/internal/tun/...` на Linux |
| AC-010 | SaveDefaultRoute | `tun_windows.go`, winipcfg | `GetBestRoute2` результат |
| AC-011 | Web UI TUN selectable | `server.go`, `handler_connect.go`, `TabbedForm.tsx` | селектор показывает TUN, Connect не падает |

## Данные и контракты

- Data model не меняется — `data-model.md: no-change`
- API/event контракты не затрагиваются
- `TunDevice` interface остаётся неизменным

## Стратегия реализации

### DEC-001: Wintun через wireguard/tun (не raw DLL)

- **Why:** проект уже использует `golang.zx2c4.com/wireguard/tun` для Linux. Единый API на обе платформы. Библиотека абстрагирует Wintun ring buffer + wait event.
- **Tradeoff:** меньше контроля над batch-size и spin-lock оптимизациями, но для MVP избыточно.
- **Affects:** `tun_windows.go` — импорт `wireguard/tun`
- **Validation:** AC-001, AC-002

### DEC-002: winipcfg для routes/IP/DNS (не exec netsh)

- **Why:** IP Helper API — каноничный способ управления сетью на Windows. `exec("netsh")` — медленно, локаль-зависимо, хрупко. winipcfg использует WireGuard и Tailscale.
- **Tradeoff:** новая зависимость `golang.zx2c4.com/wireguard/windows` (v1.0.1), но она от того же автора, что `wireguard/tun`.
- **Affects:** `go.mod` (новая dep), `tun_windows.go`
- **Validation:** AC-003, AC-004, AC-005, AC-010

### DEC-003: UUIDv5 для детерминированного GUID

- **Why:** RFC 4122, без внешних зависимостей, стандарт для name-based UUID. Используется самим WireGuard Windows. SHA-1 для GUID достаточен (криптостойкость не требуется).
- **Tradeoff:** SHA-1 считается слабым для security — нерелевантно для uniqueness в NLA.
- **Affects:** `tun_windows.go` — функция `deterministicGUID(serverURL) windows.GUID`
- **Validation:** AC-006

### DEC-004: phyIface как LUID string

- **Why:** LUID (Local Unique Identifier) — нативный ключ winipcfg, стабилен между ребутами (в отличие от InterfaceIndex). `SaveDefaultRoute()` получает LUID через `GetBestRoute2`, возвращает как string. `AddExcludeRoute` внутри парсит LUID обратно.
- **Tradeoff:** LUID string нечитаема человеком, но используется только внутри.
- **Affects:** `tun_windows.go` — конвертация string ↔ LUID
- **Validation:** AC-004, AC-010

### DEC-005: DNS через luid.SetDNS (dnsproxy без изменений)

- **Why:** dnsproxy (встроенный DNS-сервер для domain-based routing) работает идентично Linux. Разница только в регистрации: вместо перезаписи `/etc/resolv.conf` DNS на TUN-интерфейсе конфигурируется через `luid.SetDNS()`, указывая на `127.0.0.1:<port>`.
- **Tradeoff:** bootstrap/client/tun.go потребуется минимальная Windows-специфичная ветка для DNS. Пока — noop, т.к. dnsproxy registration не входит в MVP (AC-003, AC-004).
- **Affects:** `bootstrap/client/tun.go` (минимально, post-MVP)
- **Validation:** AC-003 (default route без DNS — достаточно для MVP)

### DEC-006: Web UI — динамический флаг tun_supported

- **Why:** единый бэкенд/фронтенд для всех платформ. Backend `/api/platform` уже возвращает `os` и `transparent_supported`. Добавляем `tun_supported: runtime.GOOS == "linux" || runtime.GOOS == "windows"`. Frontend проверяет флаг и скрывает/отключает TUN опцию когда `false`.
- **Tradeoff:** минимальное изменение — 5 строк бэкенд + 10 строк фронтенд. Не нужно делать отдельную сборку UI для разных платформ.
- **Affects:** `server.go`, `handler_connect.go`, `TabbedForm.tsx`, `context.tsx`, `types.ts`
- **Validation:** AC-011

## Incremental Delivery

### MVP (Фаза 1): Core data path

- `tun_windows.go`: Open (Wintun create), Close, Read, Write
- Build tag fix в `tun_stub.go`
- Cross-compile target в `scripts/build.sh`
- AC-001 (частично: адаптер создаётся, IP - после Фазы 2), AC-002, AC-008, AC-009

### Фаза 2: IP/MTU конфигурация

- `tun_windows.go`: SetIP + SetMTU через winipcfg
- AC-001 (полностью), AC-005

### Фаза 3: Маршрутизация

- `tun_windows.go`: SaveDefaultRoute, SetGateway/RemoveGateway, AddExcludeRoute/RemoveExcludeRoute/CleanupExcludeRoutes
- AC-003, AC-004, AC-010

### Фаза 4: NLA + Graceful shutdown

- `tun_windows.go`: deterministic GUID (UUIDv5), cleanup адаптера при Close
- AC-006, AC-007

### Фаза 5: Web UI integration

- `server.go`: `tun_supported` в `/api/platform`
- `handler_connect.go`: разрешить TUN на Windows
- `TabbedForm.tsx` + `context.tsx` + `types.ts`: фронтенд проверка флага
- AC-011

### Фаза 6: Integration test + docs

- Windows integration test (Wintun loopback)
- Документация `docs/ru/` и `docs/en/`
- Полное покрытие всех AC

## Порядок реализации

1. **Фаза 1** — должна быть первой (data path — основа)
2. **Фаза 2** — сразу после (без IP нет маршрутизации)
3. **Фаза 3** — после Фазы 2 (зависит от адреса)
4. **Фаза 4** — может идти параллельно с Фазой 3 (независимы)
5. **Фаза 5** — после Фаз 1-4 (зависит от работающего TUN, но может идти параллельно с Фазами 3-4)
6. **Фаза 6** — последняя (зависит от всех предыдущих)

## Риски

| Риск | Mitigation |
|---|---|
| Wintun требует admin прав — первый запуск без прав упадёт | Проверка `IsElevated()` + понятная ошибка в Open |
| `wintun.dll` отсутствует | Падать с описанием, куда положить DLL |
| GOSUMDB при добавлении `wireguard/windows` | Использовать `GONOSUMDB` или `GOFLAGS=-insecure` для первого скачивания |
| `golang.zx2c4.com/wireguard/windows` тянет лишние зависимости | Проверить `go mod why` — если firewall лишний, добавить только winipcfg |
| Wintun не поддерживает Windows < 1803 | Spec заявляет Windows 10 1803+ |

## Rollout и compatibility

- Новая функциональность изолирована за build tag `windows` — нет риска для Linux
- Сборка Linux клиента не меняется
- Специальных rollout-действий не требуется

## Проверка

- `go vet ./src/internal/tun/...` — на Linux (убедиться, что stub не сломан)
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./src/cmd/client` — проверка компиляции
- `go test ./src/internal/tun/...` — Linux unit tests (MockTunDevice + stub)
- Ручная проверка на Windows: запуск client.exe, проверка `ipconfig`, `route print`
- `@sk-test` маркеры на unit-тестах для каждой завершённой фазы

## Соответствие конституции

- нет конфликтов — TUN реализация через `wireguard/tun` уже разрешена конституцией
- Cgo не требуется — Windows TUN идёт через `syscall`, не Cgo
- Trace-маркеры `@sk-task` / `@sk-test` — обязательны, размещаются над объявлениями
