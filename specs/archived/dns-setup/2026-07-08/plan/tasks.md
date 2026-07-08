# DNS Setup: Windows + macOS TUN Registration — Задачи

## Phase Contract

Inputs: plan, spec.
Outputs: упорядоченные исполнимые задачи с покрытием критериев.
Stop if: задачи получаются расплывчатыми — нет, покрытие ясное.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `src/internal/dnsproxy/dnsproxy.go` | T1.1 |
| `src/internal/dnsproxy/dnsproxy_linux.go` | T1.1 |
| `src/internal/dnsproxy/dnsproxy_windows.go` | T1.2, T2.2 |
| `src/internal/dnsproxy/dnsproxy_darwin.go` | T1.2, T3.2 |
| `src/internal/tun/tun_common.go` | T1.3 |
| `src/internal/tun/tun_stub.go` | T1.3 |
| `src/internal/tun/tun_linux.go` | T1.3 |
| `src/internal/tun/tun_windows.go` | T1.3, T2.1 |
| `src/internal/tun/tun_darwin.go` | T1.3, T3.1 |
| `src/internal/bootstrap/client/tun.go` | T4.1, T4.3 |
| `src/internal/bootstrap/client/tun_linux.go` | T4.1 |
| `src/internal/bootstrap/client/tun_windows.go` | T4.2 |
| `src/internal/bootstrap/client/tun_darwin.go` | T4.2 |
| `src/internal/tun/tun_darwin_test.go` | T5.1 |
| `src/internal/tun/tun_windows_test.go` | T5.1 |
| `docs/ru/deployment.md`, `docs/en/deployment.md` | T5.3 |
| `.github/workflows/ci.yml` | T5.2 |

## Implementation Context

- Цель MVP: DNS-регистрация на TUN-интерфейсе Windows + macOS, рефакторинг dnsproxy, bootstrap интеграция
- Инварианты/семантика:
  - TunDevice интерфейс не breaking change — новый метод SetDNS([]string) error
  - dnsproxy.Server (ядро: Run, forwarding, Tracker) — платформенно-независимый, не трогать
  - Linux: DNS управляется через resolv.conf, SetDNS — no-op (return nil)
  - macOS: service name discovery через `-listallhardwareports`, fallback `utunX`
- Ошибки/коды:
  - networksetup не найден / нет прав → log warning, continue без DNS
  - Wintun LUID не существует → return error
- Контракты/протокол:
  - Windows DNS через winipcfg.LUID.SetDNS() и luid.GetDNS()
  - macOS DNS через `networksetup -setdnsservers/-getdnsservers`
- Границы scope: не делаем DNS override (full-tunnel), DoT/DoH, IPv6 DNS, SOCKS5/HTTP CONNECT DNS
- Proof signals: cross-compile (3 ОС), SetDNS устанавливает DNS на TUN, CleanupStaleDNS удаляет stale
- References: DEC-001 (dnsproxy split), DEC-002 (saveDNS/restoreDNS в bootstrap), DEC-003 (macOS hardwarePorts→utunX), DEC-004 (CleanupStaleDNS per platform)

## Фаза 1: Основа (рефакторинг dnsproxy + interface)

Цель: подготовить dnsproxy для кросс-платформенности и расширить TunDevice.

- [x] T1.1 Вынести Linux-специфику из dnsproxy.go в dnsproxy_linux.go. Исходный dnsproxy.go остаётся с платформенно-независимым ядром (Server struct, Run, forwarding, Tracker, RouteFunc). Функции `BackupResolvConf`, `OverrideResolvConf`, `CleanupStaleDNS`, `isSystemdResolved`, `resolvectlSet`, `resolvectlRevert` — в dnsproxy_linux.go с `//go:build linux`. Touches: src/internal/dnsproxy/dnsproxy.go, src/internal/dnsproxy/dnsproxy_linux.go
- [x] T1.2 Создать dnsproxy_windows.go и dnsproxy_darwin.go с заглушкой `CleanupStaleDNS` (no-op пока) и `//go:build windows` / `//go:build darwin`. Touches: src/internal/dnsproxy/dnsproxy_windows.go, src/internal/dnsproxy/dnsproxy_darwin.go
- [x] T1.3 Добавить `SetDNS(dnsServers []string) error` в TunDevice interface (tun_common.go). Реализовать no-op заглушки в tun_stub.go и tun_linux.go с `//go:build !linux,!windows,!darwin` / `//go:build linux`. Touches: src/internal/tun/tun_common.go, src/internal/tun/tun_stub.go, src/internal/tun/tun_linux.go

## Фаза 2: Windows SetDNS + saveDNS/restoreDNS

Цель: реализовать DNS-регистрацию на Wintun.

- [x] T2.1 Реализовать SetDNS в tun_windows.go через `luid.SetDNS()`. Сохранить оригинальный DNS через winipcfg (saveDNS). restoreDNS — luid.SetDNS(orig) в Close(). Touches: src/internal/tun/tun_windows.go
- [x] T2.2 Реализовать CleanupStaleDNS в dnsproxy_windows.go: проверить DNS на адаптере через winipcfg, если есть 127.0.0.54 — удалить. Touches: src/internal/dnsproxy/dnsproxy_windows.go

## Фаза 3: macOS SetDNS + saveDNS/restoreDNS

Цель: реализовать DNS-регистрацию на utun через networksetup.

- [x] T3.1 Реализовать SetDNS в tun_darwin.go: primary — `-listallhardwareports` парсинг Device→utun→Service, fallback — `-setdnsservers utunX 127.0.0.54`. saveDNS через `-getdnsservers`. restoreDNS в Close(). Touches: src/internal/tun/tun_darwin.go
- [x] T3.2 Реализовать CleanupStaleDNS в dnsproxy_darwin.go: проверить DNS на utun через `-getdnsservers`, если есть 127.0.0.54 — очистить. Touches: src/internal/dnsproxy/dnsproxy_darwin.go

## Фаза 4: Bootstrap интеграция

Цель: dispatch DNS setup по платформам в bootstrap.

- [x] T4.1 Вынести существующий Linux saveDNS/restoreDNS (BackupResolvConf/OverrideResolvConf/Restore) из tun.go в bootstrap/client/tun_linux.go как `saveDNS()`/`restoreDNS()`. Touches: src/internal/bootstrap/client/tun.go, src/internal/bootstrap/client/tun_linux.go
- [x] T4.2 Создать tun_windows.go и tun_darwin.go в bootstrap с saveDNS()/restoreDNS(), вызывающими tunDev.SetDNS() и platform-специфичные сохранения. Touches: src/internal/bootstrap/client/tun_windows.go, src/internal/bootstrap/client/tun_darwin.go
- [x] T4.3 Рефакторить DNS-секцию в tun.go (строки 313-436): dispatch по платформам, общая логика запуска dnsproxy остаётся. На Windows/macOS вместо OverrideResolvConf вызывается tunDev.SetDNS(dnsproxyListen). Touches: src/internal/bootstrap/client/tun.go

## Фаза 5: Проверка + docs

Цель: доказать, что фича работает на всех платформах.

- [x] T5.1 Unit-тесты: TestSetDNS для Windows/macOS (mock exec.Command / mock winipcfg через interface). Проверка парсинга hardwarePorts, fallback. Touches: src/internal/tun/tun_darwin_test.go, src/internal/tun/tun_windows_test.go
- [x] T5.2 Cross-compile verify: `GOOS=windows go build ./...`, `GOOS=darwin go build ./...`, `GOOS=linux go build ./...`. Touches: CI (github/workflows/ci.yml)
- [x] T5.3 Обновить docs: `docs/ru/deployment.md` и `docs/en/deployment.md` — DNS на Windows/macOS раздел (как работает, что требует root, macOS < Ventura limitation). Touches: docs/ru/deployment.md, docs/en/deployment.md

## Покрытие критериев приемки

- AC-001 -> T1.3, T2.1, T5.1
- AC-002 -> T1.3, T2.1, T5.1
- AC-003 -> T1.3, T3.1, T5.1
- AC-004 -> T1.3, T3.1, T5.1
- AC-005 -> T1.1, T5.2
- AC-006 -> T4.2, T4.3, T5.2
- AC-007 -> T4.2, T4.3, T5.2
- AC-008 -> T2.2, T5.1
- AC-009 -> T3.2, T5.1

## Заметки

- Фазы 2 и 3 независимы — можно параллелить
- Фаза 5.2 (cross-compile) — сквозная проверка, формально после всех фаз
- Linux stub для SetDNS (T1.3) — просто return nil (DNS на Linux управляется через resolv.conf, не через TunDevice)
- На macOS Close() уже вызывает CleanupExcludeRoutes — restoreDNS должен быть перед device.Close() или после, в зависимости от семантики networksetup
