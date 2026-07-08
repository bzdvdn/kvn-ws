# DNS Setup: Windows + macOS TUN Registration — План

## Phase Contract

Inputs: spec, inspect (pass).
Outputs: plan, data-model (no-change), tasks.
Stop if: spec too vague for safe planning — spec is adequate.

## Цель

Реализовать DNS-регистрацию для TUN-режима на Windows и macOS, вынести Linux-специфику из dnsproxy, интегрировать DNS bootstrap на всех трёх платформах.

## MVP Slice

Windows `luid.SetDNS()` + macOS `networksetup -setdnsservers` + вынос Linux-специфики из dnsproxy + bootstrap dispatch. AC-001 — AC-005.

## First Validation Path

После реализации MVP: `GOOS=windows go build ./...` + `GOOS=darwin go build ./...` проходят. Windows: `go test ./src/internal/tun/` на Linux (сборка без реального Wintun). macOS: `go vet ./...`.

## Scope

- `tun_common.go` — добавление `SetDNS([]string) error` в TunDevice interface
- `tun_windows.go` — реализация SetDNS через `luid.SetDNS()`, saveDNS/restoreDNS
- `tun_darwin.go` — реализация SetDNS через `exec.Command("networksetup", "-setdnsservers", ...)`, saveDNS/restoreDNS
- `dnsproxy.go` — ядро Server (Run, forwarding, Tracker) остаётся общим
- `dnsproxy_linux.go` (новый) — `BackupResolvConf`, `OverrideResolvConf`, `CleanupStaleDNS`, `isSystemdResolved`, `resolvectlSet`, `resolvectlRevert` с `//go:build linux`
- `dnsproxy_windows.go` (новый) — `CleanupStaleDNS` для Windows
- `dnsproxy_darwin.go` (новый) — `CleanupStaleDNS` для macOS
- `bootstrap/client/tun.go` — рефакторинг DNS-секции для dispatch по платформам
- `bootstrap/client/tun_darwin.go` (новый) — `saveDNS()`/`restoreDNS()` macOS
- `bootstrap/client/tun_windows.go` (новый) — `saveDNS()`/`restoreDNS()` Windows
- `bootstrap/client/tun_linux.go` (новый) — вынос Linux saveDNS/restoreDNS из tun.go
- `docs/{ru,en}/deployment.md` — update DNS-секции

## Performance Budget

- none — DNS-регистрация не hot path (single call per connect/disconnect)

## Implementation Surfaces

| Surface | Тип | Почему участвует |
|---------|-----|-----------------|
| `src/internal/tun/tun_common.go` | existing, change | Добавить `SetDNS([]string) error` в интерфейс |
| `src/internal/tun/tun_windows.go` | existing, change | Реализовать SetDNS через winipcfg |
| `src/internal/tun/tun_darwin.go` | existing, change | Реализовать SetDNS через networksetup |
| `src/internal/dnsproxy/dnsproxy.go` | existing, change | Удалить Linux-специфичные функции |
| `src/internal/dnsproxy/dnsproxy_linux.go` | new | Перенести Linux-only код под build tag |
| `src/internal/dnsproxy/dnsproxy_windows.go` | new | CleanupStaleDNS для Windows |
| `src/internal/dnsproxy/dnsproxy_darwin.go` | new | CleanupStaleDNS для macOS |
| `src/internal/bootstrap/client/tun.go` | existing, change | Вынести saveDNS/restoreDNS, dispatch |
| `src/internal/bootstrap/client/tun_linux.go` | new | Linux saveDNS/restoreDNS |
| `src/internal/bootstrap/client/tun_windows.go` | new | Windows saveDNS/restoreDNS |
| `src/internal/bootstrap/client/tun_darwin.go` | new | macOS saveDNS/restoreDNS |

## Bootstrapping Surfaces

- `src/internal/tun/` — уже существует, SetDNS добавляется в существующий interface
- `src/internal/dnsproxy/` — уже существует, только split по build tags
- `src/internal/bootstrap/client/` — уже существует, добавляются platform-файлы

## Влияние на архитектуру

- TunDevice interface расширяется одним методом — все реализации (linux, windows, darwin, stub) должны его реализовать
- dnsproxy разделяется на 3 файла по build tags — ядро остаётся общим
- DNS bootstrap из одного монолитного блока в tun.go превращается в dispatch по платформам

## Acceptance Approach

- AC-001 (Windows SetDNS) → tun_windows.go: SetDNS via `luid.SetDNS()`; evidence: winipcfg query
- AC-002 (Windows restore) → tun_windows.go: saveDNS в Open/SetDNS, restore в Close
- AC-003 (macOS SetDNS) → tun_darwin.go: SetDNS via networksetup; evidence: `scutil --dns`
- AC-004 (macOS restore) → tun_darwin.go: saveDNS в Open/SetDNS, restore в Close
- AC-005 (dnsproxy refactoring) → dnsproxy_linux.go + build tags; evidence: `GOOS=windows go build ./src/internal/dnsproxy/...`
- AC-006 (Windows bootstrap) → bootstrap/client/tun_windows.go + tun.go dispatch
- AC-007 (macOS bootstrap) → bootstrap/client/tun_darwin.go + tun.go dispatch
- AC-008 (Windows CleanupStaleDNS) → dnsproxy_windows.go
- AC-009 (macOS CleanupStaleDNS) → dnsproxy_darwin.go

## Данные и контракты

- TunDevice interface: новый метод `SetDNS([]string) error`
- Никаких изменений в persisted entities, config structs, API contracts, event payloads
- `data-model.md` — no-change stub

## Стратегия реализации

### DEC-001 Разделение dnsproxy по build tags

Why: dnsproxy.Server (UDP listener, forwarding, Tracker) — платформенно-независимый. `resolv.conf`/`systemd-resolved` — только Linux. Разделение предотвращает ошибки компиляции на Windows/macOS и делает границу явной.
Tradeoff: 3 файла вместо 1; дублирование Import в каждом файле.
Affects: `src/internal/dnsproxy/`
Validation: `GOOS=windows go build ./src/internal/dnsproxy/...` и `GOOS=darwin` проходят, `GOOS=linux` — все функции доступны

### DEC-002 saveDNS/restoreDNS в bootstrap, не в TunDevice

Why: сохранение/восстановление DNS — забота bootstrap'а, не TUN-устройства. TunDevice.SetDNS только устанавливает DNS на интерфейс. saveDNS() запоминает настройки до изменений, restoreDNS() восстанавливает — это логика жизненного цикла клиента.
Tradeoff: bootstrap получает 3 platform-файла вместо одного универсального interface.
Affects: `src/internal/bootstrap/client/{tun,tun_linux,tun_windows,tun_darwin}.go`
Validation: на Windows saveDNS() → luid.GetDNS() (через winipcfg), restoreDNS() → luid.SetDNS(orig)

### DEC-003 macOS: hardwarePorts → utunX fallback

Why: `networksetup -listallhardwareports` может не найти utun (не hardware port). Fallback на `utunX` напрямую работает на macOS Ventura+.
Tradeoff: macOS < Ventura может не поддержать fallback → DNS не авто-настроится; документировано.
Affects: `tun_darwin.go` (SetDNS), `tun_darwin.go` (saveDNS — `-getdnsservers <service>` с тем же discovery)
Validation: `-listallhardwareports` парсинг Device → utun → Service → `-setdnsservers <service> 127.0.0.54`. Если service не найден: `-setdnsservers utunX 127.0.0.54`

### DEC-004 CleanupStaleDNS платформенно-специфичен

Why: Linux читает `/etc/resolv.conf`. Windows проверяет через winipcfg DNS на адаптере. macOS проверяет через `-getdnsservers`. Каждый сам знает свои stale-индикаторы.
Tradeoff: 3 отдельных реализации, без shared logic.
Affects: `dnsproxy_linux.go`, `dnsproxy_windows.go`, `dnsproxy_darwin.go`
Validation: после имитации "kill без Close" — CleanupStaleDNS удаляет 127.0.0.54 из конфигурации

## Incremental Delivery

### MVP (AC-001 — AC-005)

Рефакторинг dnsproxy + TunDevice.SetDNS на обеих платформах + saveDNS/restoreDNS в bootstrap.

### Итеративное расширение

- AC-006, AC-007: bootstrap dispatch — интеграция DNS в полный connect/disconnect lifecycle
- AC-008, AC-009: CleanupStaleDNS для защиты после crash

## Порядок реализации

1. **Фаза 1 (Основа):** Рефакторинг dnsproxy — вынести Linux-специфику в `dnsproxy_linux.go`. Добавить `SetDNS` в TunDevice interface и stub-реализацию для Linux. Подготовить platform-заглушки для dnsproxy_windows.go и dnsproxy_darwin.go.
2. **Фаза 2 (Windows SetDNS):** Реализовать SetDNS в tun_windows.go. saveDNS()/restoreDNS() для Windows. CleanupStaleDNS для Windows.
3. **Фаза 3 (macOS SetDNS):** Реализовать SetDNS в tun_darwin.go. saveDNS()/restoreDNS() для macOS. CleanupStaleDNS для macOS.
4. **Фаза 4 (Bootstrap):** Вынести Linux saveDNS/restoreDNS в tun_linux.go. Разветвление DNS-секции в tun.go по платформам.
5. **Фаза 5 (Проверка):** Тесты, cross-compile verify, docs.

Фазы 2 и 3 можно параллелить (платформы независимы).

## Риски

- **macOS < Ventura:** `-setdnsservers utunX <ip>` может не работать. Mitigation: если hardwarePorts не нашёл И fallback упал — log warning, continue без DNS. Документировано.
- **Wintun GUID и DNS persistence:** Если GUID адаптера детерминирован (win-tun AC-006), DNS настройки переживают перезапуск. После crash без Close — CleanupStaleDNS чистит. Mitigation: CleanupStaleDNS вызывается в начале reconnectLoop (как сейчас для Linux).
- **Сломанный cross-compile после рефакторинга:** dnsproxy.go теряет Linux-функции, может не скомпилироваться на Linux. Mitigation: CI build matrix проверяет все три ОС.

## Rollout и compatibility

- Никаких feature flags — DNS регистрация включается автоматически при `dns_routing.enabled=true` (существующий флаг)
- Для пользователей без suffix-доменов поведение не меняется (dnsproxy не стартует)
- CleanupStaleDNS на Windows/macOS — новый вызов, но silent skip если адаптер не существует
- Специальных rollout-действий не требуется

## Проверка

- `go vet ./...` на Linux
- `GOOS=windows go build ./...`, `GOOS=darwin go build ./...`
- `go test ./src/internal/tun/...` (Linux stub + build tags)
- `go test ./src/internal/dnsproxy/...` (Linux, с существующими тестами)
- `go test ./src/internal/bootstrap/client/...` (существующие тесты не сломаны)

## Соответствие конституции

- нет конфликтов
- traceability: `@sk-task` и `@sk-test` обязательны на всех новых функциях/методах/типах
- docs: обновить `docs/ru/deployment.md` и `docs/en/deployment.md` с описанием DNS на Windows/macOS
