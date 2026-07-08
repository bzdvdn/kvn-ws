# macOS TUN Device Support — Задачи

## Phase Contract

Inputs: plan (DEC-001..DEC-006), spec (AC-001..AC-011), data-model (no-change).
Outputs: 6 фаз, ~12 задач с Touches и AC-покрытием.
Stop if: задачи получаются расплывчатыми — все Touches конкретны, AC покрыты.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/tun/tun_darwin.go` | T1.1, T2.1, T3.1, T3.2 |
| `src/internal/tun/tun_stub.go` | T1.1 |
| `scripts/build.sh` | T1.2 |
| `src/internal/webui/server.go` | T4.1 |
| `scripts/com.kvn.tun.plist` | T5.1 |
| `scripts/com.kvn.web.plist` | T5.1 |
| `src/internal/tun/tun_darwin_test.go` | T6.1 |
| `docs/ru/` / `docs/en/` | T6.2 |

## Implementation Context

- Цель MVP: TUN data path + IP/MTU + default route + exclude routes + Web UI флаг. AC-001..006, AC-008..011.
- Инварианты:
  - `TunDevice` interface не меняется — только добавляется darwin-impl
  - Write/Read без virtio headroom (как на Windows, в отличие от Linux)
  - SetIP/SetMTU/маршруты — через `exec.Command("ifconfig"/"route")`
  - Парсинг вывода `route -n get default` для SaveDefaultRoute
  - macOS не требует GUID (в отличие от Windows)
- DEC to follow:
  - DEC-001: utun через `wireguard/tun` (единый API с Linux)
  - DEC-002: ifconfig/route через exec.Command (нет нативного API)
  - DEC-003: LaunchDaemon для root (аналог systemd)
  - DEC-004: Web UI флаг inline в server.go
  - DEC-005: DNS — post-MVP
  - DEC-006: Build target `darwin`
- Build tag: `//go:build darwin` — tun_darwin.go
- Stub: `//go:build !linux,!windows,!darwin` — tun_stub.go
- root required: utun + route требуют sudo. На macOS нет CAP_NET_ADMIN
- Вне scope: SMJobBless, NetworkExtension, notarization, DNS через networksetup

## Фаза 1: Core data path

Цель: создать файл tun_darwin.go с имплементацией Open/Close/Read/Write через utun и darwin cross-compile target.

- [x] T1.1 Implement `tun_darwin.go` scaffold
  - Create `tun_darwin.go` with `//go:build darwin`, type `tunDevice`, `NewTunDevice()`
  - Implement `Open()`: `tun.CreateTUN("KVN", defaultMTU)`, `Device.Name()`
  - Implement `Close()`: `device.Close()` + `CleanupExcludeRoutes()` (idempotent)
  - Implement `Read()`: single-buf read via `device.Read()` without virtio headroom
  - Implement `Write()`: direct write via `device.Write()` without headroom
  - Update `tun_stub.go` build tag from `!linux,!windows` to `!linux,!windows,!darwin`
  - Touches: `src/internal/tun/tun_darwin.go`, `src/internal/tun/tun_stub.go`
  - AC: AC-001, AC-009

- [x] T1.2 Add darwin cross-compile target
  - Add `build_darwin()` function and `case "darwin"` in `scripts/build.sh`
  - Output `bin/client-darwin-amd64`, `bin/server-darwin-amd64`, `bin/relay-darwin-amd64`
  - Touches: `scripts/build.sh`
  - AC: AC-008

## Фаза 2: IP/MTU конфигурация

Цель: реализовать SetIP и SetMTU через ifconfig.

- [x] T2.1 Implement `SetIP()` and `SetMTU()` via ifconfig.
  - `SetIP()`: `exec.Command("ifconfig", name, ip.String(), "dstaddr", dst.String())`; MTU default
  - `SetMTU()`: `exec.Command("ifconfig", name, "mtu", strconv.Itoa(mtu))`
  - `DisableGSO()`: noop on macOS
  - Touches: `src/internal/tun/tun_darwin.go`
  - AC: AC-003

## Фаза 3: Маршрутизация

Цель: реализовать маршрутизацию через route command — default route, exclude routes, save default route.

- [x] T3.1 Implement `SetGateway()`, `RemoveGateway()`, `SaveDefaultRoute()`.
  - `SaveDefaultRoute()`: `exec.Command("route", "-n", "get", "default")`; parse stdout for gateway + interface
  - `SetGateway()`: `exec.Command("route", "add", "-net", "0.0.0.0/0", gateway.String())`
  - `RemoveGateway()`: `exec.Command("route", "delete", "-net", "0.0.0.0/0", gateway.String())`
  - Touches: `src/internal/tun/tun_darwin.go`
  - AC: AC-004, AC-010

- [x] T3.2 Implement `AddExcludeRoute()`, `RemoveExcludeRoute()`, `CleanupExcludeRoutes()`.
  - `AddExcludeRoute()`: `exec.Command("route", "add", "-net", cidr, gateway.String(), "-ifscope", iface)`
  - `RemoveExcludeRoute()`: `exec.Command("route", "delete", "-net", cidr, gateway.String())`
  - `CleanupExcludeRoutes()`: iterate `t.routes`, delete each (same pattern as Linux/Windows)
  - Touches: `src/internal/tun/tun_darwin.go`
  - AC: AC-005, AC-006

## Фаза 4: Web UI integration

Цель: web UI показывает TUN-опцию на macOS.

- [x] T4.1 Add `darwin` to `tun_supported` in `/api/platform`.
  - `server.go`: `tunSupported := runtime.GOOS == "linux" || runtime.GOOS == "windows" || runtime.GOOS == "darwin"`
  - Touches: `src/internal/webui/server.go`
  - AC: AC-011

## Фаза 5: LaunchDaemon/LaunchAgent

Цель: plist-файлы для запуска kvn-client (root) и kvn-web (user) через launchd.

- [x] T5.1 Create LaunchDaemon and LaunchAgent plists.
  - `com.kvn.tun.plist`: LaunchDaemon, `/Library/LaunchDaemons/`, root, `kvn-client --mode tun`
  - `com.kvn.web.plist`: LaunchAgent, `~/Library/LaunchAgents/`, user, `kvn-web`
  - Touches: `scripts/com.kvn.tun.plist`, `scripts/com.kvn.web.plist`
  - AC: AC-007

## Фаза 6: Проверка

Цель: automated тесты + документация.

- [x] T6.1 Add unit tests for macOS TUN.
  - `tun_darwin_test.go`: test non-root functions (parsing route output, ifconfig arg formatting)
  - Test error paths: nil device, empty routes, parse failures
  - Integration test (manual, documented): utun loopback write/read
  - Touches: `src/internal/tun/tun_darwin_test.go`
  - AC: AC-001..AC-010 (coverage via tests)

- [x] T6.2 Documentation updates for macOS TUN.
  - `docs/ru/`: add macOS-specific instructions (sudo, LaunchDaemon, build)
  - `docs/en/`: same in English
  - Update cross-compile section in build docs
  - Touches: `docs/ru/`, `docs/en/`
  - AC: — (docs, no AC)

## Покрытие критериев приемки

- AC-001 (utun adapter) → T1.1, T6.1
- AC-002 (read/write loop) → T1.1, T6.1
- AC-003 (IP + MTU) → T2.1, T6.1
- AC-004 (default route) → T3.1, T6.1
- AC-005 (exclude routes) → T3.2, T6.1
- AC-006 (cleanup on disconnect) → T3.2, T6.1
- AC-007 (LaunchDaemon) → T5.1
- AC-008 (cross-compile) → T1.2
- AC-009 (stub on non-darwin) → T1.1
- AC-010 (SaveDefaultRoute) → T3.1, T6.1
- AC-011 (Web UI tun_supported) → T4.1
