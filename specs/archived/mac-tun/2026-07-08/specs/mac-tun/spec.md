# macOS TUN Device Support

## Scope Snapshot

- In scope: TUN-режим на macOS через utun + ifconfig/route + LaunchDaemon для root-операций
- Out of scope: Privileged Helper Tool, Network Extension, notarization, DNS через networksetup

## Цель

Пользователи macOS получают TUN-режим (VPN) аналогично Linux. kvn-client работает через LaunchDaemon от root (как systemd service), kvn-web — от пользователя (LaunchAgent). Web UI показывает TUN-опцию на macOS.

## Основной сценарий

1. Пользователь устанавливает kvn-ws на macOS через brew-формулу или вручную
2. LaunchDaemon `com.kvn.tun` стартует `kvn-client --mode tun` при загрузке (root)
3. LaunchAgent `com.kvn.web` стартует `kvn-web` при входе пользователя
4. В браузере открывается http://127.0.0.1:2311, TUN-режим доступен для выбора
5. После connect: utun-интерфейс создан, IP/MTU настроены, default route через utun

## User Stories

- P1: Пользователь macOS может запустить TUN-клиент через терминал (sudo kvn-client)
- P2: Пользователь macOS может управлять TUN через kvn-web (LaunchDaemon + user agent)

## MVP Slice

P1: TUN data path + IP/MTU + default route + exclude routes через exec.Command. Web UI флаг. Build target.
AC: AC-001, AC-002, AC-003, AC-004, AC-005, AC-009, AC-010

## First Deployable Outcome

`GOOS=darwin go build ./src/cmd/client` + `sudo ./kvn-client --config client.yaml` — работает TUN-соединение на macOS.

## Scope

- Файл `tun_darwin.go` с `//go:build darwin` — Open/Close/Read/Write через wireguard/tun
- SetIP/SetMTU через `exec.Command("ifconfig", ...)`
- SetGateway/RemoveGateway через `exec.Command("route", ...)`
- AddExcludeRoute/RemoveExcludeRoute через `exec.Command("route", ...)`
- SaveDefaultRoute через `route -n get default`
- SaveDefaultRoute на Linux уже существует, darwin-версия через парсинг `route -n get default`
- Web UI: `tun_supported` включает `runtime.GOOS == "darwin"`
- Build: `scripts/build.sh` — case `"darwin"` для `GOOS=darwin GOARCH=amd64,arm64`
- LaunchDaemon plist (`scripts/com.kvn.tun.plist`)
- LaunchAgent plist (`scripts/com.kvn.web.plist`)
- routeMeta и TunDevice interface — переиспользуются из `tun_common.go`
- Stub: `tun_stub.go` — build tag `!linux,!windows,!darwin`

## Контекст

- macOS не имеет нативного API для TUN кроме системных вызовов через `/dev/tun*`
- `golang.zx2c4.com/wireguard/tun` поддерживает macOS (utun) — используется как на Linux
- Управление IP/маршрутами — только через `exec.Command("ifconfig")` и `exec.Command("route")`
- Root требуется для utun + route. На macOS нет `CAP_NET_ADMIN` как на Linux
- LaunchDaemon — единственный штатный механизм root-демона без пароля
- kvn-web на macOS — user-space, без root
- Apple Silicon (arm64) и Intel (amd64) — обе архитектуры

## Зависимости

- `golang.zx2c4.com/wireguard/tun` — уже в go.mod, поддерживает macOS
- Зависимость от `exec.Command("ifconfig")` и `exec.Command("route")` — встроенные в macOS

## Требования

- RQ-001 Система ДОЛЖНА создавать utun-интерфейс на macOS и выполнять read/write IP-пакетов
- RQ-002 Система ДОЛЖНА устанавливать IP-адрес и MTU на utun-интерфейсе
- RQ-003 Система ДОЛЖНА устанавливать и удалять default route через utun
- RQ-004 Система ДОЛЖНА добавлять и удалять exclude-маршруты на физическом интерфейсе
- RQ-005 Система ДОЛЖНА сохранять текущий default route перед переключением
- RQ-006 Система ДОЛЖНА корректно чистить маршруты и интерфейс при disconnect
- RQ-007 Web UI ДОЛЖЕН показывать TUN-опцию на macOS
- RQ-008 Скрипт сборки ДОЛЖЕН поддерживать `GOOS=darwin GOARCH=amd64` и `arm64`
- RQ-009 LaunchDaemon plist ДОЛЖЕН запускать `kvn-client --mode tun` от root

## Вне scope

- SMJobBless / Privileged Helper Tool
- NetworkExtension / NEFilterDataProvider / NEPacketTunnelProvider
- Notarization и codesigning
- DNS через `networksetup` (post-MVP, аналогично DEC-005 Windows)
- GUI-установщик `.pkg`
- macOS System Settings интеграция

## Критерии приемки

### AC-001 TUN adapter creation

- Почему это важно: без адаптера нет data path
- **Given** macOS-система
- **When** `NewTunDevice().Open()` вызван
- **Then** создан utun-интерфейс, `Name()` возвращает непустое имя (например, `utunX`)
- Evidence: вызов `ifconfig -l` показывает новый utun-интерфейс

### AC-002 Read/Write data path

- Почему это важно: основные операции TUN
- **Given** открытый utun-интерфейс
- **When** IP-пакет записан через `Write()` и прочитан через `Read()`
- **Then** данные корректно передаются через интерфейс
- Evidence: loopback write/read тест (интеграционный, при наличии root); unit-тесты для не-root функций (парсинг вывода ifconfig/route, форматирование аргументов) без root

### AC-003 IP and MTU configuration

- Почему это важно: без IP/MTU трафик не ходит
- **Given** открытый utun-интерфейс
- **When** `SetIP()` и `SetMTU()` вызваны
- **Then** `ifconfig utunX` показывает корректный IP и MTU
- Evidence: `ifconfig utunX | grep inet` и `ifconfig utunX | grep mtu`

### AC-004 Default route management

- Почему это важно: трафик должен идти через VPN
- **Given** открытый utun-интерфейс с IP
- **When** `SetGateway()` вызван с gateway IP
- **Then** `netstat -rn -f inet` показывает default route через utunX
- Evidence: `netstat -rn -f inet | grep default` показывает gateway на utunX

### AC-005 Exclude routes

- Почему это важно: split-tunnel для локальной сети
- **Given** физический интерфейс известен
- **When** `AddExcludeRoute("10.0.0.0/8", gateway, iface)` вызван
- **Then** `netstat -rn -f inet` показывает /8 маршрут через физический интерфейс
- Evidence: `netstat -rn -f inet | grep 10.0.0.0`

### AC-006 Cleanup on disconnect

- Почему это важно: не оставлять маршруты после остановки
- **Given** установлены default route + exclude routes
- **When** `Close()` вызван
- **Then** default route удалён, exclude routes удалены, utun-интерфейс отсутствует
- Evidence: `netstat -rn -f inet` не показывает маршрутов через utun; `ifconfig -l` не показывает интерфейс

### AC-007 LaunchDaemon integration

- Почему это важно: TUN должен работать без ручного sudo
- **Given** установлен `com.kvn.tun.plist` в `/Library/LaunchDaemons/`
- **When** `sudo launchctl load /Library/LaunchDaemons/com.kvn.tun.plist`
- **Then** `kvn-client --mode tun` запущен от root, TUN-соединение активно
- Evidence: `sudo launchctl list com.kvn.tun` показывает PID; `ifconfig -l` показывает utunX

### AC-008 Cross-compile for Darwin

- Почему это важно: сборка с Linux хоста
- **Given** Linux хост
- **When** `GOOS=darwin GOARCH=amd64 go build ./...`
- **Then** бинарник собран без ошибок
- Evidence: `file kvn-client` показывает `Mach-O 64-bit executable x86_64`

### AC-009 Stub on non-macOS

- Почему это важно: не ломать сборку на других OS
- **Given** Linux или Windows хост
- **When** `go build ./...`
- **Then** `tun_darwin.go` не компилируется; `tun_stub.go` с тегом `!darwin` работает
- Evidence: `go build` проходит без ошибок

### AC-010 SaveDefaultRoute

- Почему это важно: сохранить оригинальный gateway для восстановления
- **Given** активный default route
- **When** `SaveDefaultRoute()` вызван
- **Then** возвращены gateway IP и имя физического интерфейса
- Evidence: `route -n get default` — парсинг stdout возвращает корректные значения

### AC-011 Web UI tun_supported

- Почему это важно: пользователь видит TUN в UI
- **Given** kvn-web запущен на macOS
- **When** `GET /api/platform`
- **Then** `{"tun_supported": true}`
- Evidence: curl-запрос к /api/platform

## Допущения

- `wireguard/tun` на macOS создаёт utun-интерфейсы, поведение аналогично Linux
- `ifconfig` и `route` — стабильные команды, присутствуют во всех версиях macOS
- LaunchDaemon требует `sudo` для загрузки; kvn-web не должен управлять LaunchDaemon (user-space)
- Пароль sudo не требуется после `launchctl load` — демон стартует автоматически при загрузке
- Пользователь имеет admin-доступ для установки LaunchDaemon

## Критерии успеха

- SC-001 TUN-соединение на macOS устанавливается за <3s после запуска kvn-client
- SC-002 Пропускная способность TUN не более чем на 10% ниже нативной скорости интерфейса

## Краевые случаи

- SIP (System Integrity Protection) может блокировать `ifconfig`/`route` даже от root — требуется отключение SIP
- Несколько utun-интерфейсов — ищем первый свободный (wireguard/tun делает это автоматически)
- Sleep/Wake — macOS может сбросить маршруты при wake; reconnectLoop пересоздаст их
- Нет default route на старте (офлайн) — SaveDefaultRoute возвращает ошибку
- Физический интерфейс не указан — exclude routes не добавляются
- Apple Silicon (arm64) требует отдельную сборку — добавлено в build.sh

## Открытые вопросы

- Стоит ли bundle `com.kvn.tun.plist` в zip-архив релиза или генерировать скриптом установки?
- Нужен ли `kvn-client --install-daemon` / `--uninstall-daemon` флаг?
- Как обрабатывать Sleep/Wake — launchd KeepAlive сам перезапустит?
