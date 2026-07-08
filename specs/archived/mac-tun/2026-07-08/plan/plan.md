# macOS TUN Device Support — План

## Phase Contract

Inputs: spec (11 AC, 9 RQ), inspect (pass).
Outputs: plan, data model (no-change).
Stop if: spec слишком расплывчата — все AC конкретны, scope чёткий.

## Цель

Реализовать TUN-режим на macOS через `wireguard/tun` (utun) + `ifconfig`/`route`. Web UI показывает TUN-опцию. LaunchDaemon для root-демона. Build target darwin/amd64 + arm64.

## MVP Slice

TUN data path + IP/MTU + default route + exclude routes + cross-compile.
AC: AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-008, AC-009, AC-010.

## First Validation Path

```bash
GOOS=darwin go build ./src/cmd/client
# На macOS:
sudo ./kvn-client --config client.yaml  # mode: tun
# Проверить:
ifconfig utunX  # IP/MTU
netstat -rn -f inet | grep utun  # default route
```

## Scope

- `src/internal/tun/tun_darwin.go` — новая имплементация TunDevice для macOS (build tag: darwin)
- `src/internal/tun/tun_stub.go` — обновить build tag до `!linux,!windows,!darwin`
- `scripts/build.sh` — добавить `darwin` target
- `src/internal/webui/server.go` — `tun_supported` включает darwin
- `scripts/com.kvn.tun.plist` — LaunchDaemon (root, TUN-клиент)
- `scripts/com.kvn.web.plist` — LaunchAgent (user, kvn-web)
- `src/internal/tun/tun_darwin_test.go` — unit-тесты

## Performance Budget

- none — macOS TUN не имеет жёстких performance-ограничений сверх общих для TUN

## Implementation Surfaces

| Surface | Статус | Зачем |
|---------|--------|-------|
| `src/internal/tun/tun_darwin.go` | новая | Имплементация TunDevice для macOS |
| `src/internal/tun/tun_stub.go` | изменение | Build tag: `!linux,!windows,!darwin` |
| `scripts/build.sh` | изменение | + `darwin` target (GOOS=darwin) |
| `src/internal/webui/server.go` | изменение | tun_supported += darwin |
| `src/internal/tun/tun_darwin_test.go` | новая | Unit-тесты |
| `scripts/com.kvn.tun.plist` | новая | LaunchDaemon |
| `scripts/com.kvn.web.plist` | новая | LaunchAgent |

## Bootstrapping Surfaces

- none — все нужные пакеты существуют (tun_common.go, TunDevice interface, tun_stub.go)

## Влияние на архитектуру

- Локальное: новый файл `tun_darwin.go` в пакете `tun` — не меняет интерфейсы
- `tun_stub.go` build tag расширяется с `!linux,!windows` до `!linux,!windows,!darwin`
- `SaveDefaultRoute` — уже экспортированная функция в `tun_common.go` (linux impl), на darwin своя имплементация
- Никаких изменений в data model, API-контрактах или БД

## Acceptance Approach

- AC-001 (utun создание) → `tun_darwin.go`: `tun.CreateTUN()` + проверка имени
- AC-002 (Read/Write) → `tun_darwin.go`: Read/Write через device; unit-тесты для non-root функций
- AC-003 (IP/MTU) → `tun_darwin.go`: SetIP/SetMTU через `exec.Command("ifconfig")`
- AC-004 (Default route) → `tun_darwin.go`: SetGateway/RemoveGateway через `exec.Command("route")`
- AC-005 (Exclude routes) → `tun_darwin.go`: AddExcludeRoute/RemoveExcludeRoute через `route add/del`
- AC-006 (Cleanup) → `tun_darwin.go`: Close() вызывает CleanupExcludeRoutes() + device.Close()
- AC-007 (LaunchDaemon) → `scripts/com.kvn.tun.plist`, `scripts/com.kvn.web.plist`
- AC-008 (Cross-compile) → `scripts/build.sh`: `darwin)` case
- AC-009 (Stub) → `tun_stub.go`: build tag `!darwin`
- AC-010 (SaveDefaultRoute) → `tun_darwin.go`: `route -n get default` parsing
- AC-011 (Web UI) → `server.go`: `runtime.GOOS == "linux" || runtime.GOOS == "windows" || runtime.GOOS == "darwin"`

## Данные и контракты

- Data model не меняется — status: no-change
- API: `/api/platform` расширяется полем `tun_supported: true` на darwin (обратно совместимо)
- Контракты: нет изменений

## Стратегия реализации

### DEC-001: utun через wireguard/tun

- Why: единый API с Linux и Windows, уже в go.mod, no CGo
- Tradeoff: нет тонкого контроля над созданием utun (номер интерфейса), но wireguard/tun выбирает первый свободный
- Affects: `tun_darwin.go`
- Validation: `ifconfig -l` показывает utunX после Open()

### DEC-002: ifconfig/route через exec.Command

- Why: macOS не имеет встроенного Go API для управления IP/маршрутами (аналога winipcfg или netlink)
- Tradeoff: парсинг stdout, зависимость от формата вывода утилит; медленнее нативного API
- Affects: `tun_darwin.go` — SetIP, SetMTU, SetGateway, RemoveGateway, AddExcludeRoute, RemoveExcludeRoute, SaveDefaultRoute
- Validation: `ifconfig utunX`, `netstat -rn -f inet`

### DEC-003: LaunchDaemon для root (аналог systemd)

- Why: macOS требует root для utun + route, LaunchDaemon — штатный механизм без пароля
- Tradeoff: нужно `sudo` для установки plist; kvn-web не может управлять LaunchDaemon из user-space
- Affects: `scripts/com.kvn.tun.plist`
- Validation: `sudo launchctl load /Library/LaunchDaemons/com.kvn.tun.plist` → TUN активен

### DEC-004: Web UI флаг — server.go inline

- Why: минимальное изменение, паттерн уже есть (transparent_supported, win-tun)
- Tradeoff: нет динамической проверки наличия wintun.dll-аналога (но на macOS utun встроен в ядро)
- Affects: `server.go`
- Validation: `curl /api/platform | jq .tun_supported`

### DEC-005: DNS — post-MVP

- Why: DNS через `networksetup` требует root и не влияет на data path; пользователь может настроить вручную
- Tradeoff: TUN работает, но DNS не перенаправляется автоматически (аналогично Windows DEC-005)
- Validation: post-MVP

### DEC-006: Build target `darwin` (не `macos`)

- Why: `GOOS=darwin` — стандартное значение Go для macOS
- Tradeoff: пользователи могут ожидать название `macos`, но это консистентно с Go-экосистемой
- Affects: `scripts/build.sh`
- Validation: `GOOS=darwin GOARCH=amd64 go build ./...` проходит

## Incremental Delivery

### MVP (Первая ценность)

- TUN data path: Open/Close/Read/Write + stub update + build target
- Default: IP/MTU (ifconfig), default route (route), SaveDefaultRoute
- Exclude routes: Add/Remove/Cleanup
- Web UI флаг
- Cleanup: Close() + CleanupExcludeRoutes()
- Unit-тесты для non-root функций
- AC: 001-006, 008-011

### Итеративное расширение

- LaunchDaemon/LaunchAgent plist (AC-007) — после MVP, т.к. можно тестировать через `sudo ./kvn-client`
- DNS через `networksetup` — post-MVP

## Порядок реализации

1. **Фаза 1**: `tun_darwin.go` scaffold — Open/Close/Read/Write + stub + build target (AC-001, AC-002, AC-008, AC-009)
2. **Фаза 2**: SetIP + SetMTU через ifconfig (AC-003)
3. **Фаза 3**: Маршрутизация — SetGateway/RemoveGateway, SaveDefaultRoute, AddExcludeRoute/RemoveExcludeRoute, CleanupExcludeRoutes (AC-004, AC-005, AC-006, AC-010)
4. **Фаза 4**: Web UI флаг (AC-011)
5. **Фаза 5**: LaunchDaemon/LaunchAgent plist (AC-007)
6. **Фаза 6**: Unit-тесты + docs

## Риски

- **Риск 1**: SIP может блокировать ifconfig/route даже от root на некоторых конфигурациях
  Mitigation: документировать требование отключения SIP для TUN-режима
- **Риск 2**: Разный вывод ifconfig/route между версиями macOS (Sequoia vs Sonoma)
  Mitigation: парсинг с fallback-режимом; unit-тесты на разных версиях
- **Риск 3**: LaunchDaemon требует codesigning для notarization на macOS 15+
  Mitigation: MVP через sudo; notarization — post-MVP

## Rollout и compatibility

- Новый бинарник для darwin — не затрагивает существующие платформы
- LaunchDaemon plist не устанавливается автоматически — пользователь копирует вручную (или через install-скрипт)
- No migration, no feature flag

## Проверка

- Automated: `go vet ./...`, `go test ./src/internal/tun/...` (non-root тесты)
- Cross-compile: `GOOS=darwin GOARCH=amd64 go build ./...` на Linux
- Manual на macOS: `sudo ./kvn-client --config client.yaml`, проверка ifconfig/netstat
- Покрытие: каждый AC — конкретный observable check (команда, curl, тест)

## Соответствие конституции

- нет конфликтов
