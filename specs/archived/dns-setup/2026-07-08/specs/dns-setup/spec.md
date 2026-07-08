# DNS Setup: Windows + macOS TUN Registration

## Scope Snapshot

- In scope: регистрация DNS-прокси на TUN-интерфейсе Windows и macOS, рефакторинг dnsproxy для кросс-платформенности, интеграция в bootstrap клиента
- Out of scope: DNS в SOCKS5/HTTP CONNECT proxy (клиент резолвит сам), transparent proxy на Windows/macOS (Linux-only), IPv6 DNS, DNS over TLS/HTTPS

## Цель

Пользователи Windows и macOS получают корректную DNS-маршрутизацию в TUN-режиме: DNS-запросы приложений попадают во встроенный dnsproxy (как на Linux), domain-based split-tunnel работает одинаково на всех платформах. Успех: после connect `nslookup` на Windows/macOS резолвит через dnsproxy, exclude-домены идут напрямую, include-домены — через туннель.

## Основной сценарий

1. Пользователь запускает kvn-client --mode tun на Windows или macOS
2. Клиент создаёт TUN-интерфейс, настраивает IP/MTU/маршруты
3. Клиент запускает dnsproxy на 127.0.0.54:53 (как на Linux)
4. Клиент регистрирует dnsproxy как DNS-сервер на TUN-интерфейсе:
   - Windows: `luid.SetDNS()` через winipcfg
   - macOS: `networksetup -setdnsservers <utun> 127.0.0.54`
5. Системные DNS-запросы приложений идут через dnsproxy
6. dnsproxy применяет domain-based routing (exclude/include)
7. При disconnect DNS регистрация откатывается к исходному состоянию

## User Stories

- P1: В TUN-режиме на Windows/macOS DNS резолвится через dnsproxy, split-tunnel по доменам работает
- P2: После остановки клиента DNS-настройки системы восстанавливаются

## MVP Slice

P1: `luid.SetDNS()` на Windows + `networksetup -setdnsservers` на macOS + вынос Linux-специфики из dnsproxy + bootstrap-интеграция. AC-001 — AC-009.

## First Deployable Outcome

На Windows: `kvn-client --mode tun` — `ipconfig /all` показывает DNS-сервер 127.0.0.54 на TUN-интерфейсе. На macOS: `scutil --dns` показывает dnsproxy как nameserver для utunX.

## Scope

- `TunDevice` interface: добавление `SetDNS(dnsServers []string) error`
- `tun_windows.go`: реализация `SetDNS()` через `luid.SetDNS()`
- `tun_darwin.go`: реализация `SetDNS()` через `exec.Command("networksetup", "-setdnsservers", ...)`
- `dnsproxy.go`: вынос `BackupResolvConf`, `OverrideResolvConf`, `CleanupStaleDNS`, `isSystemdResolved` в отдельный файл с build tag `linux`
- Создание `dnsproxy_darwin.go` и `dnsproxy_windows.go` для платформенных DNS-регистраций
- `bootstrap/client/tun.go`: разветвление DNS-секции (313-436) по платформам
- Сохранение/восстановление оригинальных DNS-настроек системы через `saveDNS()`/`restoreDNS()` в bootstrap

## Контекст

- Linux DNS registration работает через `/etc/resolv.conf` (или systemd-resolved `resolvectl`)
- Windows: winipcfg.LUID.SetDNS() — штатный API для задания DNS на интерфейсе
- macOS: `networksetup -setdnsservers <service> <ip>` — штатная команда, требует root
- macOS utun — сетевой сервис, может не появиться в networksetup списке; может потребоваться `# utunX` как service name
- dnsproxy.Server (ядро UDP listener + upstream forwarding + Tracker) — платформенно-независимый, не требует изменений
- CleanupStaleDNS() на Linux — специфичен для resolv.conf; на Windows/macOS нужна своя проверка stale-записей

## Зависимости

- `golang.org/x/sys/windows` — уже в go.mod
- `golang.zx2c4.com/wireguard/windows/tunnel/winipcfg` — уже в go.mod (win-tun)
- macOS `networksetup` — встроен в систему
- TunDevice интерфейс (`tun_common.go`) — будет расширен методом `SetDNS`

## Требования

- RQ-001 Система ДОЛЖНА устанавливать DNS-сервер 127.0.0.54 на TUN-интерфейс Windows через `luid.SetDNS()`
- RQ-002 Система ДОЛЖНА устанавливать DNS-сервер 127.0.0.54 на TUN-интерфейс macOS через `networksetup -setdnsservers`
- RQ-003 Система ДОЛЖНА сохранять оригинальные DNS-настройки перед установкой своих и восстанавливать их при Close()
- RQ-004 `dnsproxy.go` НЕ ДОЛЖЕН содержать Linux-специфичных функций (resolv.conf, systemd-resolved) в общем файле — они ДОЛЖНЫ быть вынесены за build tag
- RQ-005 DNS bootstrap в `bootstrap/client/tun.go` ДОЛЖЕН работать на всех трёх платформах без копипаста Linux-only кода
- RQ-006 macOS: клиент ДОЛЖЕН сначала определить service name для utunX через `networksetup -listallhardwareports` (парсинг Device → utun → Service). Если service name не найден — fallback через `networksetup -setdnsservers utunX 127.0.0.54` напрямую
- RQ-007 Windows: `CleanupStaleDNS()` ДОЛЖЕН проверять DNS на TUN-интерфейсе через winipcfg и удалять stale-запись 127.0.0.54
- RQ-008 macOS: `CleanupStaleDNS()` ДОЛЖЕН проверять DNS на utunX через `networksetup -getdnsservers` и удалять stale-запись
- RQ-009 SOCKS5/HTTP CONNECT proxy mode НЕ ТРЕБУЕТ DNS-регистрации (приложение резолвит адрес самостоятельно)

## Вне scope

- DNS через SOCKS5/HTTP CONNECT proxy (клиентское приложение резолвит само)
- Transparent proxy DNS на Windows/macOS (режим отсутствует)
- DNS over TLS/HTTPS в dnsproxy
- IPv6 DNS-регистрация
- GUI для DNS-настроек
- Windows Service интеграция (влияет на сохранение/восстановление DNS через реестр для всех пользователей)

## Критерии приемки

### AC-001 Windows: SetDNS на TUN-интерфейс

- Почему это важно: без DNS на TUN интерфейсе DNS-запросы не доходят до dnsproxy
- **Given** Windows TUN-интерфейс активен (создан Wintun адаптер)
- **When** `tunDevice.SetDNS(["127.0.0.54"])` вызван
- **Then** `Get-DnsClientServerAddress -InterfaceIndex <idx>` показывает DNS-сервер 127.0.0.54
- Evidence: PowerShell команда подтверждает DNS на TUN-интерфейсе

### AC-002 Windows: сохранение и восстановление DNS

- Почему это важно: не оставлять DNS от dnsproxy после отключения
- **Given** на TUN-интерфейсе был установлен DNS 127.0.0.54
- **When** `tunDevice.Close()` вызван
- **Then** DNS на TUN-интерфейсе очищен (интерфейс удалён Wintun API, DNS уходит вместе с ним) ИЛИ восстановлен предыдущий DNS
- Evidence: после Close `Get-DnsClientServerAddress` не показывает 127.0.0.54 на бывшем TUN-интерфейсе

### AC-003 macOS: SetDNS на utun-интерфейс

- Почему это важно: без DNS на utun DNS-запросы не доходят до dnsproxy
- **Given** macOS utun-интерфейс активен
- **When** `tunDevice.SetDNS(["127.0.0.54"])` вызван
- **Then** `scutil --dns` показывает nameserver 127.0.0.54 для интерфейса utunX
- Evidence: `scutil --dns | grep -A2 utun` содержит "nameserver[127] 127.0.0.54"

### AC-004 macOS: сохранение и восстановление DNS

- Почему это важно: не оставить DNS на utun после отключения
- **Given** на utun установлен DNS 127.0.0.54
- **When** `tunDevice.Close()` вызван
- **Then** DNS для utunX очищен (или восстановлены оригинальные серверы)
- Evidence: `networksetup -getdnsservers <service>` не содержит 127.0.0.54

### AC-005 dnsproxy: Linux-специфика вынесена за build tag

- Почему это важно: не компилировать Linux-код на Windows/macOS, чистая кодовая база
- **Given** сборка на Linux
- **When** `go build ./...`
- **Then** resolv.conf/systemd-resolved код компилируется
- **And** на Windows/macOS этот код не компилируется (build tag linux)
- Evidence: `GOOS=windows go build ./src/internal/dnsproxy/...` проходит без ошибок

### AC-006 DNS bootstrap работает на Windows

- Почему это важно: DNS+domain-routing должен работать из коробки
- **Given** Windows-система, конфиг с `dns_routing.enabled=true` и suffix-доменами
- **When** `kvn-client --mode tun` запущен
- **Then** dnsproxy запущен на 127.0.0.54:53, DNS на TUN-интерфейсе установлен на 127.0.0.54
- Evidence: лог клиента показывает "dns proxy started", `Get-DnsClientServerAddress` подтверждает DNS

### AC-007 DNS bootstrap работает на macOS

- Почему это важно: DNS+domain-routing должен работать из коробки
- **Given** macOS-система, конфиг с `dns_routing.enabled=true` и suffix-доменами
- **When** `sudo kvn-client --mode tun` запущен
- **Then** dnsproxy запущен на 127.0.0.54:53, DNS на utun установлен на 127.0.0.54
- Evidence: лог клиента показывает "dns proxy started", `scutil --dns` подтверждает DNS

### AC-008 CleanupStaleDNS на Windows

- Почему это важно: после убитого процесса DNS на TUN не должен оставаться
- **Given** предыдущий запуск был убит без Close (crash)
- **When** `CleanupStaleDNS("127.0.0.54:53")` вызван
- **Then** DNS-сервер 127.0.0.54 удалён из конфигурации Wintun-адаптера
- Evidence: `Get-DnsClientServerAddress` не содержит 127.0.0.54 на TUN-интерфейсе

### AC-009 CleanupStaleDNS на macOS

- Почему это важно: после убитого процесса DNS на utun не должен оставаться
- **Given** предыдущий запуск был убит без Close (crash)
- **When** `CleanupStaleDNS("127.0.0.54:53")` вызван
- **Then** DNS-сервер 127.0.0.54 удалён из конфигурации utunX
- Evidence: `networksetup -getdnsservers <service>` не содержит 127.0.0.54

## Допущения

- Wintun GUID адаптера детерминирован — DNS конфигурация сохраняется между перезапусками (win-tun AC-006)
- macOS utunX может не появиться в networksetup списке сервисов — используется fallback `-setdnsservers utunX <ip>` без сервисного имени
- Для macOS требуется root для networksetup (utun уже root-owned)
- Если на macOS SIP блокирует networksetup — DNS работать не будет, падать с логом (как ifconfig/route)
- DNS-override (SetDNSOverride) в full-tunnel режиме остаётся как опция для отдельной фичи
- В SOCKS5/HTTP CONNECT proxy mode DNS-маршрутизация не требуется — клиентское приложение само выбирает DNS

## Критерии успеха

- SC-001: `go vet ./...` на Linux без ошибок, `GOOS=windows` и `GOOS=darwin` cross-compile без ошибок
- SC-002: Существующие Linux тесты не сломаны (регрессия 0)
- SC-003: `nslookup google.com` в TUN-режиме на Windows/macOS резолвит через dnsproxy (проверяется по логам: "direct resolve" для exclude, "tunnel forward" для include)

## Краевые случаи

- DNS routing отключён (`dns_routing.enabled=false`) — dnsproxy не запускается, системный DNS не меняется
- Нет suffix-доменов — dnsproxy не запускается (как на Linux сейчас)
- Несколько DNS-серверов на Windows TUN — `luid.SetDNS()` принимает слайс
- macOS не нашёл utunX в networksetup при restore — fallback игнорировать ошибку
- Windows: адаптер не существует при CleanupStaleDNS (уже удалён) — silent skip
- macOS: `networksetup -setdnsservers` с пустым списком — сбрасывает DNS на DHCP

## Открытые вопросы

- На macOS — протестировать fallback `-setdnsservers utunX <ip>` на macOS < Ventura. Если не работает — потребуется cgo/SystemConfiguration для полной поддержки старых версий.
