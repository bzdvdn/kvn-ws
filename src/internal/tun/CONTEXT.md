# Context — src/internal/tun

## Purpose

Платформенная абстракция TUN-устройства: открыть/закрыть интерфейс, назначить IP/MTU, поставить default- и exclude-маршруты, управлять DNS и снимать всё это при закрытии. Три реальные реализации (Linux, Windows, macOS) и заглушка для остальных ОС.

## Responsibilities

- Владеет: интерфейсом `TunDevice`, конкретными `tunDevice` на Linux/Win/macOS, трекингом добавленных маршрутов (`routeMeta`) и их очисткой, сохранением/восстановлением системного default-route и DNS.
- Не владеет: решением, какие маршруты нужны (это `bootstrap/client` + `routing`), DNS-прокси (`dnsproxy`), транспортом и циклом сессии.

## How it works

- `NewTunDevice()` выбирается по build-tag: `tun.go` (linux), `tun_windows.go`, `tun_darwin.go`, `tun_stub.go` (`!linux && !windows && !darwin`).
- Linux: `wireguard/tun.CreateTUN("kvn", 1400)`; адрес/MTU/up — через netlink; default/exclude-маршруты — через `exec.Command("ip", ...)` (намеренно, чтобы `netlink.RouteDel` с частичным совпадением не снёс физический default). `Write` добавляет 12-байтовый virtio-net headroom; `DisableGSO` сбрасывает offload через `TUNSETOFFLOAD`.
- Windows: Wintun с детерминированным GUID (UUIDv5 от `"kvn-ws"`), чтобы адаптер переиспользовался; `Open` делает один повтор через 1с, если адаптер ещё занят; адреса/маршруты/DNS — через `winipcfg` LUID; `SaveDefaultRoute` возвращает LUID строкой в слоте `iface`.
- macOS: utun через `wireguard/tun.CreateTUN("KVN", 1400)`; `ifconfig`/`route`/`networksetup`; `SetDNS` сохраняет текущие DNS сервиса и восстанавливает их в `Close`.
- Общая модель: `AddExcludeRoute` добавляет маршрут и запоминает его; `CleanupExcludeRoutes` удаляет все запомненные и очищает список; `Close` на всех реальных платформах вызывает `CleanupExcludeRoutes` (на darwin ещё и `restoreDNS`).
- `SaveDefaultRoute()` — снимок текущего default-маршрута до поднятия туннеля (gateway + iface/LUID), `CleanupStaleExcludeRoutes(serverIP)` — удаление «осиротевшего» маршрута до IP сервера от убитой сессии.
- `CIDRMaskV4Bits/Total = 24/32`, `CIDRMaskV6Bits/Total = 112/128` — маски, которые клиент передаёт в `SetIP`.

## Key files

- `src/internal/tun/tun_common.go` — интерфейс `TunDevice`, `routeMeta`, константы масок.
- `src/internal/tun/tun.go` — Linux-реализация (`ip`, netlink, GSO).
- `src/internal/tun/tun_offload_linux.go` — `TUNSETOFFLOAD` ioctl.
- `src/internal/tun/tun_windows.go` — Wintun, GUID, winipcfg, LUID.
- `src/internal/tun/tun_darwin.go` — utun, `ifconfig`/`route`/`networksetup`.
- `src/internal/tun/tun_stub.go` — заглушка: все операции возвращают ошибку.

## Public contracts

- `TunDevice`: `Open`, `Close`, `Read`, `Write`, `SetIP`, `SetMTU`, `SetGateway`, `RemoveGateway`, `AddExcludeRoute`, `RemoveExcludeRoute`, `SetDNS`, `CleanupExcludeRoutes`, `DisableGSO`.
- `NewTunDevice() TunDevice`; пакетные `SaveDefaultRoute() (net.IP, string, error)` и `CleanupStaleExcludeRoutes(serverIP string)`.
- Семантика `phyIface` в `AddExcludeRoute`/`RemoveExcludeRoute` платформенная: имя интерфейса на Linux/macOS, LUID-строка на Windows.
- `MockTunDevice` (в `tun_test.go`) — тестовая реализация интерфейса.

## Dependencies

- `golang.zx2c4.com/wireguard/tun`, `github.com/vishvananda/netlink` (Linux), `golang.zx2c4.com/wireguard/windows/tunnel/winipcfg` (Windows).

## Consumed by

- `src/internal/bootstrap/client` — открытие/закрытие TUN, маршруты, DNS, `StopTun`.
- `src/internal/tunnel` — чтение/запись пакетов через переданные функции.
- `src/cmd/gatetest`, `src/cmd/stability`, интеграционные тесты — напрямую или через мок.

## Gotchas

- На Linux и Windows `AddExcludeRoute`/`RemoveExcludeRoute` **молча игнорируют IPv6** (`return nil`) — IPv6-exclude работать не будет.
- Назначение `phyIface` различается: на Windows это строка LUID из `SaveDefaultRoute`, на Linux/macOS — имя интерфейса; передавать между платформами нельзя.
- Linux-маршруты намеренно идут через `ip`, а не netlink: `RouteDel` по частичному совпадению может удалить физический default-route.
- `Close` идемпотентен по `device == nil`, но `CleanupExcludeRoutes` очищает список — повторный вызов не восстановит удалённое.
- На Windows `Open` ретраится один раз через 1с — «tun device busy» после реконнекта ожидаем; клиент дополнительно делает `StopTun`.
- `SetDNS` на Linux — no-op (DNS управляется `dnsproxy`/`resolv.conf`), на macOS меняет системные DNS и восстанавливает в `Close`, на Windows ставит DNS на LUID адаптера.
- `DisableGSO` — no-op на Windows/macOS, на Linux требует файлового дескриптора TUN (`device.File()`); отсутствие fd не считается ошибкой.
- `tun_stub` компилируется на прочих ОС: `NewTunDevice` успешен, но `Open` вернёт «TUN is not supported on this platform».
- Linux `Write` всегда копирует пакет в новый буфер с 12-байтовым headroom — аллокация на каждый пакет.
