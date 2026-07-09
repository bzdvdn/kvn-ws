# Windows TUN Device Support

## Scope Snapshot

- **In scope:** реализация `TunDevice` для Windows через Wintun — создание TUN-интерфейса, чтение/запись IP-пакетов, назначение IP/MTU, управление маршрутами, конфигурация DNS, интеграция TUN-режима в kvn-web UI.
- **Out of scope:** Windows Service (svc), WFP kill-switch, Desktop UI (`src/cmd/desktop/`), установщик Windows, производительность (>1 Gbps).

## Цель

Пользователи Windows смогут запускать KVN-клиент в TUN-режиме (аналогично Linux). Клиент создаёт виртуальный сетевой интерфейс через Wintun, назначает IP/MTU, управляет маршрутами и DNS — так же, как сейчас на Linux через `wireguard/tun` + `netlink`/`ip`. Успех фичи измеряется прохождением трафика через KVN-туннель на Windows 10/11.

## Основной сценарий

1. Пользователь запускает `kvn-client` на Windows с TUN-конфигурацией.
2. Клиент загружает `wintun.dll` из директории приложения, создаёт Wintun-адаптер с детерминированным GUID.
3. Клиент открывает Wintun-сессию (ring buffer 8 MiB).
4. Клиент назначает IP-адрес и MTU на интерфейс через Win32 IP Helper API (winipcfg).
5. Клиент добавляет default route через TUN-интерфейс с низкой метрикой.
6. Клиент добавляет exclude-маршруты для server IP и указанных диапазонов на физический интерфейс.
7. Клиент конфигурирует DNS-серверы на TUN-интерфейсе.
8. TUN read/write цикл передаёт IP-пакеты между Wintun ring buffer и транспортом (WS/QUIC).
9. При завершении клиент удаляет маршруты, очищает DNS, закрывает Wintun-сессию и адаптер.

## User Stories

### P1 Story: Windows TUN create/read/write

Как Windows-пользователь, я хочу, чтобы KVN-клиент мог создать TUN-интерфейс и передавать через него IP-пакеты, чтобы установить VPN-соединение.

### P2 Story: Windows TUN routes + DNS

Как Windows-пользователь, я хочу, чтобы KVN-клиент корректно настраивал маршрутизацию и DNS на Windows (split-tunnel, exclude routes), чтобы трафик ходил по тем же правилам, что и на Linux.

## MVP Slice

P1 Story: создание Wintun-адаптера + read/write цикл + SetIP + SetMTU. Закрывает AC-001, AC-002.

## First Deployable Outcome

Бинарный файл `kvn-web.exe`, собранный под Windows (cross-compile с Linux). Пользователь открывает Web UI → видит в выпадающем списке режимов "TUN" → выбирает → коннектится. `ipconfig` показывает новый TUN-интерфейс с назначенным IP.

## Scope

- Новый файл `src/internal/tun/tun_windows.go` с build tag `windows`
- Модификация build tag в `src/internal/tun/tun_stub.go` на `!linux,!windows`
- Добавление зависимости `golang.zx2c4.com/wireguard/windows/tunnel/winipcfg`
- Имплементация на Windows методов `TunDevice`: Open, Close, Read, Write, SetIP, SetMTU, SetGateway, RemoveGateway, AddExcludeRoute, RemoveExcludeRoute, CleanupExcludeRoutes, DisableGSO, SaveDefaultRoute, CleanupStaleExcludeRoutes
- wintun.dll — загрузка из директории приложения (fully qualified path), не из system32
- Детерминированный GUID для Wintun-адаптера на основе хеша конфига
- `tun_windows_test.go` — unit тесты (mockable, без реального Wintun)
- Cross-compile скрипт (`scripts/build-windows.sh`) или модификация `scripts/build.sh`
- Web UI backend: `/api/platform` возвращает `tun_supported: true` на Windows, снятие блокировки TUN в `handler_connect.go`
- Web UI frontend: отображение TUN-режима в выпадающем списке только когда `tun_supported=true`

## Контекст

- Проект уже использует `golang.zx2c4.com/wireguard/tun` для Linux — та же библиотека работает и на Windows (через Wintun)
- `golang.zx2c4.com/wintun` уже в `go.mod` как indirect dependency
- `golang.org/x/sys` уже в `go.mod` (нужен для Windows API)
- Существующий `TunDevice` interface (tun_common.go) не требует изменений — Windows реализация будет соответствовать тому же контракту
- На Windows нет virtio-net-header — `Write()` не требует headroom (в отличие от Linux)
- Wintun требует Administrator rights при первом старте (driver install)
- Wintun — Layer 3 only (без Ethernet/TAP)

## Зависимости

- `golang.zx2c4.com/wireguard/windows/tunnel/winipcfg` — новая зависимость для управления IP/mаршрутами/DNS через IP Helper API
- `wintun.dll` — внешний DLL, не вендорится, распространяется с бинарём (скачивание с wintun.net или включение в установщик)

## Требования

### RQ-001 Windows TUN создание и data path

Клиент ДОЛЖЕН создавать Wintun-адаптер и обеспечивать read/write цикл IP-пакетов через интерфейс `TunDevice`.

### RQ-002 IP и MTU конфигурация

Клиент ДОЛЖЕН назначать IPv4-адрес и MTU на Wintun-интерфейс через winipcfg.

### RQ-003 Маршрутизация

Клиент ДОЛЖЕН добавлять/удалять default route через TUN и exclude-маршруты на физический интерфейс.

### RQ-004 DNS конфигурация

Клиент ДОЛЖЕН устанавливать DNS-серверы на TUN-интерфейс через `luid.SetDNS()`.

### RQ-005 NLA стабильность

Клиент ДОЛЖЕН использовать детерминированный GUID при создании адаптера для предотвращения множественных NLA-профилей.

### RQ-006 wintun.dll sideloading

Клиент ДОЛЖЕН загружать `wintun.dll` из директории приложения (fully qualified path), а не из system32.

### RQ-007 Cross-compile

Сборка Windows-бинаря ДОЛЖНА работать с Linux через `GOOS=windows GOARCH=amd64 go build`.

### RQ-008 Deterministic GUID

Клиент ДОЛЖЕН генерировать детерминированный GUID для Wintun-адаптера на основе `serverURL` с использованием UUIDv5 (SHA-1, namespace DNS). GUID НЕ ДОЛЖЕН меняться между перезапусками для одного и того же serverURL.

### RQ-009 Web UI: TUN mode availability

Web UI ДОЛЖЕН показывать TUN-режим как доступный для выбора на Windows. Backend `/api/platform` ДОЛЖЕН возвращать `tun_supported: true` на Windows. Frontend ДОЛЖЕН использовать этот флаг для включения/отключения опции TUN в селекторе режима. Backend `handler_connect.go` НЕ ДОЛЖЕН блокировать TUN на Windows.

## Вне scope

- Windows Service (`golang.org/x/sys/windows/svc`) — будет отдельной фичей
- WFP kill-switch (Windows Filtering Platform firewall) — будет отдельной фичей
- Desktop UI (`src/cmd/desktop/`) на Windows — отдельная фича
- Desktop НЕ равно Web UI. Web UI (`src/internal/webui/`) — в scope фичи (селектор режима, `/api/platform`). Desktop UI — это native WebView обёртка, её код не меняется.
- GUI/Tray-интеграция
- Windows installer (.msi, .exe installer)
- IPv6 (поддержка по winipcfg заложена, но не тестируется в этой фиче)
- Производительность (batch read/write, spin-lock оптимизации)
- Поддержка Windows 7 (target: Windows 10 1803+)
- Поддержка ARM64 Windows

## Критерии приемки

### AC-001 Wintun adapter creation and IP assignment

- **Why:** без создания адаптера TUN-режим на Windows невозможен
- **Given** Windows-система с Administrator правами и `wintun.dll` в директории приложения
- **When** клиент вызывает `tunDevice.Open()` и затем `tunDevice.SetIP(ip, mask)`
- **Then** в системе появляется новый сетевой интерфейс, видимый через `ipconfig` / `Get-NetAdapter`, с назначенным IPv4-адресом
- **Evidence:** вывод `ipconfig` показывает интерфейс с именем "KVN" и указанным IP

### AC-002 TUN read/write packet loop

- **Why:** data path — core функция TUN
- **Given** TUN-интерфейс создан и открыт
- **When** IP-пакет записывается в TUN через `tunDevice.Write(buf)`, и читается через `tunDevice.Read(buf)`
- **Then** пакет корректно проходит через буфер ядра (write → kernel → userspace read)
- **Evidence:** данные, записанные в TUN, читаются из него (loopback test)

### AC-003 Default route configuration

- **Why:** без default route трафик не пойдёт через TUN
- **Given** TUN-интерфейс активен с IP-адресом
- **When** клиент вызывает `tunDevice.SetGateway(gateway)`
- **Then** в таблице маршрутизации Windows появляется default route с `InterfaceMetric=0`, указывающий на TUN-интерфейс
- **Evidence:** `route print -4` показывает `0.0.0.0/0` с gateway и interface TUN, metric 0

### AC-004 Exclude route management

- **Why:** split-tunnel требует bypass-маршрутов
- **Given** TUN-интерфейс активен, физический интерфейс известен
- **When** клиент вызывает `tunDevice.AddExcludeRoute("1.2.3.4/32", phyGateway, phyIface)`
- **Then** в таблице маршрутизации Windows появляется маршрут для `1.2.3.4/32` через физический интерфейс с низкой метрикой
- **And** вызов `tunDevice.RemoveExcludeRoute(...)` удаляет этот маршрут
- **And** вызов `tunDevice.CleanupExcludeRoutes()` удаляет все добавленные exclude-маршруты
- **Evidence:** `route print` показывает/скрывает указанный маршрут

### AC-005 MTU configuration

- **Why:** MTU直接影响фрагментация
- **Given** TUN-интерфейссоздан
- **When** клиент вызывает `tunDevice.SetMTU(1400)`
- **Then** MTU интерфейса устанавливается в 1400
- **Evidence:** `netsh interface ipv4 show interfaces` показывает MTU=1400 для TUN-интерфейса

### AC-006 Deterministic GUID (NLA stability)

- **Why:** без этого каждый перезапуск создаёт новый NLA-профиль, лимит 15
- **Given** конфиг клиента имеет определённый fingerprint
- **When** адаптер создаётся через `tun.CreateTUNWithRequestedGUID`
- **Then** GUID адаптера детерминирован и повторяем между перезапусками
- **Evidence:** GUID в `Get-NetAdapter` одинаковый после перезапуска клиента

### AC-007 Graceful shutdown

- **Why:** чистое удаление интерфейса и маршрутов
- **Given** TUN-интерфейс активен с маршрутами
- **When** клиент вызывает `tunDevice.Close()`
- **Then** интерфейс исчезает из системы, маршруты удалены
- **Evidence:** после Close интерфейс не виден в `ipconfig`, маршруты не висят

### AC-008 Cross-compile from Linux

- **Why:** разработка на Linux, сборка для Windows
- **Given** Linux-система с Go toolchain
- **When** выполняется `GOOS=windows GOARCH=amd64 go build ./src/cmd/client`
- **Then** создаётся `client.exe` без ошибок сборки
- **Evidence:** `file client.exe` показывает PE32+ executable for Windows

### AC-009 Error on non-Windows

- **Why:** защита от некорректного запуска
- **Given** система не Windows
- **When** клиент пытается создать TUN
- **Then** возвращается ошибка "TUN is not supported on this platform"
- **Evidence:** `tun_stub.go` с build tag `!linux,!windows` выбрасывает ошибку

### AC-010 SaveDefaultRoute on Windows

- **Why:** нужно для exclude-маршрутов (физический интерфейс)
- **Given** Windows-система с активным сетевым подключением
- **When** вызывается `tun.SaveDefaultRoute()`
- **Then** возвращается gateway IP и имя интерфейса текущего default route
- **Evidence:** результат содержит IP шлюза и name физического интерфейса

### AC-011 Web UI: TUN mode selectable on Windows

- **Why:** без UI пользователь не сможет выбрать TUN-режим в kvn-web
- **Given** Windows-система, kvn-web запущен
- **When** пользователь открывает Web UI и смотрит селектор режимов
- **Then** в выпадающем списке присутствует опция "TUN"
- **And** при выборе TUN и нажатии Connect соединение устанавливается (не падает с "not supported")
- **Evidence:** селектор показывает "TUN", после Connect в логе нет ошибки "TUN mode is not supported"

## Допущения

- Windows 10 1803+ или Windows 11 (требование Wintun)
- У пользователя есть Administrator права (необходимо для Wintun driver install)
- `wintun.dll` (amd64) распространяется вместе с бинарём или скачивается
- Для IPv6 достаточно фреймворка winipcfg — отдельная маршрутизация IPv6 не требуется в MVP
- Существующий `TunDevice` interface не меняется — только добавляется Windows-реализация
- **DNS стратегия на Windows:** встроенный dnsproxy работает как на Linux (прослушивает 127.0.0.1:<port>), но вместо перезаписи `/etc/resolv.conf` DNS на TUN-интерфейс конфигурируется через `luid.SetDNS()`, указывая на dnsproxy. Для domain-based routing сам dnsproxy не меняется.
- **Формат phyIface на Windows:** `SaveDefaultRoute()` возвращает LUID физического интерфейса как строку (результат `LUID.String()`). `AddExcludeRoute`/`RemoveExcludeRoute` принимают эту строку как `phyIface` и внутри конвертируют в `winipcfg.LUID` через `luid.FromString()`.

## Критерии успеха

- SC-001: Go test suite проходит на Linux без изменений (регрессии нет)
- SC-002: `go vet ./src/internal/tun/...` не выдаёт ошибок
- SC-003: Cross-compile Windows binary из Linux успешен

## Краевые случаи

- Wintun adapter creation при отсутствии `wintun.dll` — понятная ошибка
- Повторный Open без Close — возврат ошибки или graceful reset
- Закрытие TUN во время активного Read/Write — корректное завершение без паники
- Default route не найден (нет сети) — `SaveDefaultRoute` возвращает ошибку, exclude-маршруты не добавляются
- Очень большое количество exclude-маршрутов (>100) — корректное добавление/удаление
- Пересоздание адаптера с тем же GUID после падения — не должно дублировать NLA профили

## Открытые вопросы

- Падать ли, если `wintun.dll` не найден, или пытаться скачать? Пока: падать с понятной ошибкой.
- Нужен ли отдельный `scripts/build-windows.sh` или достаточно модифицировать `scripts/build.sh`? Предварительно: модифицировать build.sh.
