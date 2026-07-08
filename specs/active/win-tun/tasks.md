# Windows TUN Device Support — Задачи

## Phase Contract

Inputs: plan (DEC-001..DEC-006), spec (AC-001..AC-011), data-model (no-change).
Outputs: 6 фаз, 12 задач с Touches и AC-покрытием.
Stop if: задачи получаются расплывчатыми — все Touches конкретны, AC покрыты.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/tun/tun_windows.go` | T1.1, T2.1, T3.1, T3.2, T4.1, T4.2 |
| `src/internal/tun/tun_stub.go` | T1.1 |
| `go.mod` | T1.1 |
| `scripts/build.sh` | T1.2 |
| `src/internal/webui/server.go` | T5.1 |
| `src/internal/webui/handler_connect.go` | T5.1 |
| `src/internal/webui/frontend/src/types.ts` | T5.2 |
| `src/internal/webui/frontend/src/context.tsx` | T5.2 |
| `src/internal/webui/frontend/src/TabbedForm.tsx` | T5.2 |
| `src/internal/tun/tun_windows_test.go` | T6.1 |
| `docs/ru/` / `docs/en/` | T6.2 |

## Implementation Context

- Цель MVP: Wintun TUN на Windows: data path + IP/MTU + default route + web UI селектор. AC-001, AC-002, AC-003, AC-005, AC-008, AC-009, AC-011.
- Invariants:
  - `TunDevice` interface не меняется — только добавляется windows-impl
  - Write на Windows без headroom (нет virtio-net-header)
  - Wintun — Layer 3 only (raw IP-пакеты, без Ethernet)
- DEC to follow:
  - DEC-001: Wintun через `wireguard/tun` (единый API с Linux)
  - DEC-002: winipcfg для routes/IP/DNS (не exec netsh)
  - DEC-003: UUIDv5 для детерминированного GUID
  - DEC-004: phyIface как LUID string
  - DEC-005: DNS через `luid.SetDNS()`, dnsproxy без изменений (post-MVP)
  - DEC-006: Web UI — динамический флаг `tun_supported`
- Build tags: `//go:build windows` — tun_windows.go, `//go:build !linux,!windows` — tun_stub.go
- DLL: wintun.dll загружается из директории приложения по fully qualified path
- Вне scope: Windows Service, WFP kill-switch, desktop UI обёртка, производительность >1Gbps

## Фаза 1: Core data path

Цель: создать файл tun_windows.go с имплементацией Open/Close/Read/Write через Wintun и cross-compile target.

- [x] T1.1 Implement `tun_windows.go` scaffold
  - Add `golang.zx2c4.com/wireguard/windows` to `go.mod`
  - Create `tun_windows.go` with `//go:build windows`, type `tunDevice`, `NewTunDevice()`
  - Implement `Open()`: `tun.CreateTUN("KVN", 1400)`, `Device.Name()`
  - Implement `Close()`: `device.Close()`
  - Implement `Read()`: single-buf read via `device.Read()` without virtio headroom
  - Implement `Write()`: direct write via `device.Write()` without headroom
  - Update `tun_stub.go` build tag from `!linux` to `!linux,!windows`
  - Touches: `src/internal/tun/tun_windows.go`, `src/internal/tun/tun_stub.go`, `go.mod`
  - AC: AC-002, AC-009

- [x] T1.2 Add Windows cross-compile target
  - Add `case "windows"` with `GOOS=windows GOARCH=amd64`
  - Output `bin/client.exe`, `bin/kvn-web.exe`
  - Touches: `scripts/build.sh`
  - AC: AC-008

## Фаза 2: IP/MTU конфигурация

Цель: реализовать SetIP и SetMTU через winipcfg LUID API.

- [x] T2.1 Implement `SetIP()` and `SetMTU()` via winipcfg.
  - Store `luid` from `tun.NativeTun.LUID()` during Open
  - `SetIP()`: `luid.SetIPAddressesForFamily(winipcfg.AddressFamilyIPv4, []netip.Prefix{...})`, `luid.IPInterface()` → set NLMTU
  - `SetMTU()`: read `IPInterface()`, update `NLMTU`, call `Set()`
  - `DisableGSO()`: noop on Windows
  - Touches: `src/internal/tun/tun_windows.go`
  - AC: AC-001, AC-005

## Фаза 3: Маршрутизация

Цель: реализовать маршрутизацию через winipcfg — default route, exclude routes, save default route.

- [x] T3.1 Implement `SetGateway()`, `RemoveGateway()`, `SaveDefaultRoute()`.
  - `SaveDefaultRoute()`: enumerate interfaces via `winipcfg.GetAdaptersAddresses()`, find best default route, return `(gateway IP, LUID.String())`
  - `SetGateway()`: `luid.SetRoutesForFamily()` with `0.0.0.0/0` → gateway, metric 0
  - `RemoveGateway()`: `luid.FlushRoutes()` or delete specific route
  - Touches: `src/internal/tun/tun_windows.go`
  - AC: AC-003, AC-010

- [x] T3.2 Implement `AddExcludeRoute()`, `RemoveExcludeRoute()`, `CleanupExcludeRoutes()`.
  - `AddExcludeRoute()`: parse `phyIface` string to `winipcfg.LUID`, add route via `luid.SetRoutesForFamily()` with metric 0 on physical interface
  - `RemoveExcludeRoute()`: delete specific route from physical interface
  - `CleanupExcludeRoutes()`: track added routes in `[]routeMeta`, delete all on cleanup
  - `CleanupStaleExcludeRoutes()`: scan and remove stale /32 routes for server IP
  - Touches: `src/internal/tun/tun_windows.go`
  - AC: AC-004

## Фаза 4: NLA + Graceful shutdown

Цель: детерминированный GUID и чистый shutdown с удалением адаптера.

- [x] T4.1 Implement deterministic GUID via UUIDv5.
  - `deterministicGUID(serverURL string) windows.GUID` using SHA-1 + DNS namespace
  - Use `tun.CreateTUNWithRequestedGUID("KVN", &guid, mtu)` in Open
  - Import `crypto/sha1`, `encoding/binary`, `golang.org/x/sys/windows`
  - Touches: `src/internal/tun/tun_windows.go`
  - AC: AC-006

- [x] T4.2 Implement graceful shutdown with full cleanup.
  - `Close()`: call `device.Close()` which destroys Wintun adapter
  - Ensure `Close()` is idempotent (guarded by `sync.Once` or nil check)
  - Ensure routes are flushed before adapter destroy (if winipcfg doesn't auto-clean)
  - Touches: `src/internal/tun/tun_windows.go`
  - AC: AC-007

## Фаза 5: Web UI integration

Цель: web UI показывает селектор TUN-режима на Windows.

- [x] T5.1 Backend: add `tun_supported` to `/api/platform` and unblock TUN on Windows.
  - `server.go`: add `"tun_supported": runtime.GOOS == "linux" || runtime.GOOS == "windows"` to platform response
  - `handler_connect.go`: change block from `if runtime.GOOS != "linux"` to `if runtime.GOOS != "linux" && runtime.GOOS != "windows"` (or check `tun_supported`)
  - If TUN is selected on macOS or other: still reject with "not supported"
  - Touches: `src/internal/webui/server.go`, `src/internal/webui/handler_connect.go`
  - AC: AC-011 (backend part)

- [x] T5.2 Frontend: conditionally show TUN option based on `tun_supported`.
  - `types.ts`: add `tun_supported?: boolean` to platform response type
  - `context.tsx`: pass `tun_supported` through AppState
  - `TabbedForm.tsx`: in mode selector, disable/hide TUN option when `!tun_supported`
  - Touches: `src/internal/webui/frontend/src/types.ts`, `src/internal/webui/frontend/src/context.tsx`, `src/internal/webui/frontend/src/TabbedForm.tsx`
  - AC: AC-011 (frontend part)

## Фаза 6: Проверка

Цель: automated тесты + документация.

- [x] T6.1 Add unit tests for Windows TUN.
  - `tun_windows_test.go`: use `MockTunDevice` pattern for platform-independent testing
  - Test `deterministicGUID` produces stable UUIDv5 for same input
  - Test LUID string roundtrip (parse → format → parse)
  - Test error paths: nil device, empty routes, invalid LUID
  - Integration test (manual, documented): Wintun loopback write/read
  - Touches: `src/internal/tun/tun_windows_test.go`
  - AC: AC-001..AC-010 (coverage via tests)

- [x] T6.2 Documentation updates for Windows TUN.
  - `docs/ru/`: add Windows-specific instructions (wintun.dll, admin rights, build)
  - `docs/en/`: same in English
  - Update cross-compile section in build docs
  - Touches: `docs/ru/`, `docs/en/`
  - AC: — (docs, no AC)

## Покрытие критериев приемки

- AC-001 (Wintun adapter + IP) → T1.1, T2.1, T6.1
- AC-002 (read/write loop) → T1.1, T6.1
- AC-003 (default route) → T3.1, T6.1
- AC-004 (exclude routes) → T3.2, T6.1
- AC-005 (MTU config) → T2.1, T6.1
- AC-006 (deterministic GUID) → T4.1, T6.1
- AC-007 (graceful shutdown) → T4.2, T6.1
- AC-008 (cross-compile) → T1.2
- AC-009 (stub on non-Windows) → T1.1
- AC-010 (SaveDefaultRoute) → T3.1, T6.1
- AC-011 (Web UI TUN selectable) → T5.1, T5.2
