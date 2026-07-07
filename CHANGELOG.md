<!-- @sk-task docs-and-release#T1.2: CHANGELOG v1.0.0 (AC-007) -->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- **Config: `dns_cache` → `dns_routing`** — Go struct `DNSCacheCfg` переименован в `DNSRoutingCfg`, поле `RoutingCfg.DNSCache` → `RoutingCfg.DNSRouting`, теги `dns_cache` → `dns_routing`. Backward compat: старый ключ `dns_cache` читается из YAML (Viper) и JSON (`RoutingCfg.UnmarshalJSON`). Frontend: секция "DNS Cache" → "DNS Routing" с hint о связи с exclude/include domains. Примеры конфигов обновлены.
- **Examples: relay config** — deprecated `dns.upstream` заменён на `dns.upstreams` в `examples/relay-terminator/relay.yaml`.
- **Docker: frontend + kvn-web в образе** — Dockerfile: добавлен frontend build stage (node:20-alpine), kvn-web в Go build и runtime stage. Создан `.dockerignore` (исключены .git, node_modules, specs, android).
- **CI: автоматический Docker build/push на релизах** — новый job `docker` в `.github/workflows/ci.yml`: сборка мультиплатформенного образа (`linux/amd64`, `linux/arm64`), пуш в Docker Hub как `bzdvdn/kvn:<tag>` + `bzdvdn/kvn:latest`.

### Added

- **Android: in-app Log Viewer** — структурированный логгер `AppLogger` (уровни DEBUG/INFO/WARN/ERROR, кольцевой буфер 2000 записей, `SharedFlow` для live streaming). Полноэкранный `LogViewerScreen` как 4-й таб Bottom Navigation с живой лентой, auto-scroll, фильтрацией по уровню/тегу, текстовым поиском с highlight, pause/resume, copy, export и share. Старый AlertDialog "Routing Logs" удалён. Все потребители (FakeDnsResolver, KvnVpnService, QrScannerScreen) мигрированы на AppLogger.
- **Android: fakeDNS domain-based routing** — `routingDomainsEnabled` flag (default `false`), `routingExcludeDomains`, `routingIncludeDomains` suffix lists. DNS (UDP/53) перехватывается в tunReader: exclude-домены резолвятся через физическую сеть (bindNetwork) и доставляются напрямую (DirectDeliverer); include-домены получают fake IP из 198.18.0.0/15 с реврайтом dst IP + checksum. Suffix matching через endsWith + dot-barrier. FakeIpPool — bitmap-аллокатор (32768 адресов). TCP direct delivery с 2s connect timeout, tunnel fallback при ошибке. AAAA → пустой ответ.
- **Android: DNS Routing UI** — toggle switch, suffix fields, Routing Logs button в SettingsScreen.kt. LogBuffer — in-memory ring buffer (500 entries) для диагностики DNS/routing.
- **Android: LogBuffer** — in-memory ring buffer для диагностических логов DNS/routing, доступен через Routing Logs в UI.

### Fixed

- **Android: TCP data offset** — неверная формула dataOffset (`/4` вместо `ushr 4`) вычисляла TCP-заголовок в 4× длиннее реального. handleTcpData копировал payload со смещением +60 байт, теряя начало HTTP/TLS данных → 400 Bad Request, ERR_SSL_PROTOCOL_ERROR.
- **Android: TCP checksum range** — `tcpChecksum(..., 20, 20 + payload.size)` покрывал только payload без TCP-заголовка. SYN-ACK (payload=0) имел pseudo-header length=0, segment data не читался — все пакеты отбрасывались ядром. Исправлено: `end = 20 + tcpLen`.
- **Android: TCP checksum pseudo-header** — `tcpChecksum()` смешивал байты src и dst IP в одном 16-битном слове. Исправлено на 4 отдельных слова.
- **Android: TCP seq after SYN** — данные начинали seq с 10000 (как SYN-ACK), но SYN потребляет 1 байт seq space. Исправлен стартовый seq на 10001.
- **Android: TCP ports in response** — `buildTcpResponse` не менял порты местами для ответного пакета (src=app, dst=server). Исправлено: src=server, dst=app.
- **Android: bindSocket без protect fallback** — при `defaultNetwork != null` вызывался только `bindSocket`; если он бросал исключение, `protect()` не вызывался. Исправлено: оба всегда вызываются независимо.
- **Android: oversized TCP response packets** — reader job читал весь ответ сокета (до 65535 байт) в один IP-пакет, превышающий MTU. Исправлено: данные режутся на chunk-и по `mtu-40`.

## [0.5.4] — 2026-07-07

### Added

- **Server: nftables → iptables fallback** — `IPTablesManager` в `nat/iptables.go`. `NewManager()` автоопределяет: если `nft` не найден, создаёт NAT через `iptables`/`iptables-legacy`. Поле `natMgr` в `Server` изменено с `*NFTManager` на `Manager` (interface). Исправляет запуск сервера на Ubuntu 22.04 без `nftables`.

## [0.5.3] — 2026-07-06

### Added

- **Proxy mode: configurable parallel connections** — `proxy_connections` (default 10) в `ClientConfig`. Управляет числом параллельных WS-транспортных соединений в proxy mode, устраняя contention на едином `wmu`. Совместим с web UI — поле на вкладке Advanced.
- **kvn-web: exclude/include chip-списки** — Routing tab: `exclude_ranges`, `include_ranges`, `exclude_ips`, `include_ips`, `exclude_domains`, `include_domains` теперь отображаются как редактируемые chip-списки с add/delete, а не как placeholder-инпуты.
- **kvn-web: дедупликация конфигов** — строковые списки в routing автоматически дедуплицируются при сохранении через API и при merge в `mergeConfig`. Порядок первого вхождения сохраняется.
- **kvn-web: отображение дефолтов** — `SetClientDefaults` экспортирован и вызывается в API-хендлерах (`GET /api/config`, `GET /api/servers`), так что UI всегда показывает актуальные значения по умолчанию (auto_reconnect, proxy_connections, mtu и др.), а не zero values.
- **kvn-web: show/hide для поля Token** — кнопка 👁️/🙈 переключает видимость токена аутентификации.

### Fixed

- **kvn-desktop: Windows EXE без иконки** — `.ico` файлы были только 16×16 (747 байт) — на высоких DPI выглядят как отсутствие иконки. Перегенерированы в мультирезолюционные (16–256px). CI команда `rsrc` не содержала `-ico` флаг, и `.syso` записывался в `winres/` вместо корня пакета — Go linker не подхватывал ни иконку, ни UAC-манифест.
- **CI: golangci-lint errcheck** — 2 unchecked type assertion в `src/internal/tunnel/session.go` (lines 351, 492). Исправлены через `_, _` pattern.
- **kvn-desktop: Windows proxy stream read error** — `kvn-web` больше не запускается как child-процесс `kvn-desktop`. Scheduled task `kvn-web` стартует `kvn-web.exe --no-browser --port 2311` при logon; `kvn-desktop` только управляет через `schtasks /Run` / `taskkill`. Полная изоляция от job object / console handles WebView2.

## [0.5.1] — 2026-07-05

### Added

- **kvn-web: redesigned UI** — новая архитектура фронтенда: React Context для состояния, `TabbedForm` с переиспользуемыми компонентами (`FormField`, `ServerCards`, `LogPanel`, `TrafficMeter`). Метрики трафика (RX/TX, скорость) через `MetricCollector` + `Sender`. Вывод логов в реальном времени через `LogPanel`. Переработан `App.tsx` (с -700 строк).
- **Android: per-server override UI** — `SettingsScreen` с переопределением параметров для каждого сервера (маршрутизация, DNS, proxy, шифрование). `TrafficScreen` с графиками. Рефакторинг `ConnectScreen` (-400 строк). Новые spec/plan/tasks.
- **kvn-desktop: brand icon across platforms** — единая иконка (щит KVN из `favicon.svg`) на всех платформах: встроена в `.exe` через `.syso` (Windows), установка в `~/.local/share/icons/` (Linux), в `Contents/Resources` `.app` bundle (macOS). `.desktop` / `Info.plist` / `.lnk` ссылаются на кастомную иконку.
- **DNS upstreams: список upstream'ов вместо одного** — `config.DNSProxyCfg.Upstreams []string`, `config.RelayDNSCfg.Upstreams []string`, `config.ServerConfig.DNSUpstreams`. Backward compat для старого поля `upstream` (custom `UnmarshalYAML`/`UnmarshalJSON`). `dnsproxy.New` variadic (`listen string, upstreams ...string`). Fallback-цикл в `dnsproxy.forward` — при ошибке/таймауте первого upstream перебор следующих. Web UI: динамический список input'ов с кнопками Add/Remove. `mergeConfig` в `handler_connect.go` проверяет `Upstreams` вместо `Upstream`. Default: `["1.1.1.1:53", "8.8.8.8:53"]`.

### Fixed

- **Web UI: DNS proxy секция не отображалась в Global Settings** — Go-бинарник через `//go:embed all:frontend/dist` отдавал старую сборку `dist/` без фикса upstreams. Пересобраны фронтенд (`npm run build`) и Go-бинарник.
- **Windows: closing kvn-desktop больше не убивает kvn-web** — webui-сервер больше не встроен в desktop-процесс; при закрытии окна завершается только `kvn-desktop.exe`, `kvn-web.exe` продолжает работать как внешний процесс. `ServiceManager.Start/Stop` запускают/останавливают `kvn-web.exe` с `--no-browser --port 2311`.
- **macOS: .desktop shortcut заменён на .app bundle** — `shortcut_darwin.go` создаёт `~/Applications/KVN Desktop.app/Contents/Info.plist` с `CFBundleName`, `CFBundleIconFile`, symlink бинарника. Приложение отображается в Launchpad.
- **Windows desktop build: совместимость с golang.org/x/sys v0.44.0** — `windows.SyscallN`/`windows.CoCreateInstance` заменены на `syscall.Syscall` (stdlib) и прямой вызов `ole32.CoCreateInstance` через `NewLazySystemDLL`. Исправлена type mismatch в `single_windows.go`.

### Removed

- **kvn-desktop: system tray** — удалён мёртвый код tray (GtkStatusIcon, Shell_NotifyIconW, NSStatusBar stub, `--no-tray` flag, `noTrayMode`). Tray был написан, но никогда не интегрирован в lifecycle приложения. Все сопутствующие иконки (`connected.png`, `disconnected.png`, `kvn.ico`) удалены; заменены на единый brand icon.

### Changed

- **kvn-desktop: Windows lifecycle** — `app_windows.go` переписан по модели Linux/macOS: подключение к внешнему `kvn-web.exe` вместо встроенного сервера. Убран `SetServerRestart`/`SetServerStop`. `service_windows.go` теперь реально запускает/останавливает процесс.
- **kvn-desktop: macOS shortcut** — `shortcut_unix.go` ограничен тегом `linux` (только `.desktop`). Добавлен `shortcut_darwin.go` с генерацией `.app` bundle.

## [0.5.0] — 2026-07-04

### Added

- **kvn-desktop: нативное десктоп-приложение** — кросс-платформенный GUI-клиент на базе webview (CGo + libgtk-3/webkit2gtk на Linux, WebKit на macOS, Edge WebView2 на Windows):
  - Единое окно с Web UI (SPA), без необходимости открывать браузер.
  - CI-сборка для linux/amd64, darwin/arm64, windows/amd64.
  - Windows: UAC-манифест, `-H windowsgui` (без консольного окна).
- **Android: кнопка show/hide для поля Token** — глазок в `OutlinedTextField` для переключения `PasswordVisualTransformation` / `VisualTransformation.None`, удобно править один символ.
- **Android: NetworkCallback для reconnect при смене сети** — `ConnectivityManager.NetworkCallback.onLost()` дёргает `transportClient?.disconnect()`, триггеря reconnect через новую сеть без ожидания таймаута OkHttp.
- **DNS Response Tracker: TUN DNS proxy hardening** — production-ready TUN DNS proxy для exclude_domains:
  - `SetRouteFunc` для TUN mode (was missing — все DNS шли TCP upstream).
  - `SetDirectRouteFunc` hook — добавляет `/32` kernel exclude route для resolved IP excluded домена непосредственно при DNS-ответе (до `WriteToUDP`), устраняя race condition.
  - `CleanupExcludeRoutes()` в `tunDevice` — удаление всех kernel exclude routes при disconnect (ранее не чистились, ломали openfortivpn после stop KVN).
  - Loopback resolver filter (`127.0.0.53`) — systemd-resolved не вызывает DNS loop; fallback на upstream DNS.
  - Private IP resolver filter — corporate DNS (10.x.x.x) за openfortivpn не получает exclude route; пакеты идут через ppp0.
  - `resolveDirect` multi-resolver — пробует все резолверы последовательно (был exit после первого).
  - `directRouteFn` private/loopback skip — `/32` exclude route не добавляется для private/loopback resolved IP (corporate IP за ppp0 не ломается).
- **Android: app picker with search, system apps filter** — `AppPickerScreen` full-screen Compose list with icon, name, search, checkboxes, "Show system apps" toggle (default hidden). Icons loaded lazily via `ImageView` for reliable rendering on all API levels.
- **Android: loading indicator** — "Loading apps..." text centered while app list loads asynchronously in `produceState`.

### Fixed

- **Android: EOFException из-за race на wmu** — удалён OkHttp `pingInterval(30s)`. Сервер уже отправляет PING каждые 25с, OkHttp отвечает PONG автоматически. Собственный PING OkHttp создавал race на `wmu.Lock()` в серверном `SetPingHandler` → ответный PONG не отправлялся → OkHttp генерировал `EOFException`.
- **Android: бесконечный цикл реконнекта** — `ReconnectManager.stop()` + `start()` при каждом `DISCONNECTED` сбрасывал `retryCount` в 0. `MAX_RETRIES=10` никогда не достигалcя. Убран stop/start, ReconnectManager теперь сам управляет циклом с бэкоффом.
- **TUN DNS proxy: stale exclude routes после disconnect** — все exclude routes (exclude_ranges, resolver IPs, resolved IPs) теперь удаляются при `CleanupExcludeRoutes()` в defer `runSession`. Без фикса после `systemctl stop kvn` маршруты через phy оставались, ломая openfortivpn и базовый интернет-доступ (только ребут чинил).
- **TUN DNS proxy: resolveDirect пробовал только первый resolver** — если он не отвечал (NXDOMAIN/timeout), fallback на второй не происходил. С корпоративным DNS (10.x.x.x) для .ru доменов это приводило к failure. Исправлено: resolveDirect перебирает все резолверы, останавливается на первом успешном ответе.
- **DNS loop с systemd-resolved** — `resolveDirect` коннектился к `127.0.0.53`, который (из-за `resolvectl dns lo <proxy>`) форвардил запрос обратно в proxy → loop → 5s timeout. Исправлено: loopback фильтр + fallback на upstream DNS.
- **Android: ANR when opening app picker** — `PackageManager` calls moved to `Dispatchers.Default` via `produceState`.
- **Android: per-app filtering non-functional** — `VpnService.Builder` now uses XOR mode (`addAllowedApplication` XOR `addDisallowedApplication`, never both). Previously both were called → `establish()` threw `IllegalArgumentException` (silently caught) → no filtering applied.
- **Android: stale app mode data on connect** — opposite list (allow/block) cleared when saving, preventing stale entries from overriding user's current mode.
- **Android: app settings race condition** — `connect()` now accepts optional app settings parameters, avoiding stale read from DataStore while `saveAppSettings()` coroutine is still writing.

### Changed

- **Android: per-app filtering** — split-tunnel (TCP/UDP proxy routing, DNS intercept) replaced with OS-native per-app filtering via `VpnService.Builder.addAllowedApplication()` / `addDisallowedApplication()`. Allowlist and blocklist modes mutually exclusive (XOR).
- **Android: ConnectionConfig** — удалены `routingIncludeDomains`, `routingExcludeDomains` (IP-based routing поля сохранены).
- **Android: app-level DNS servers** — moved to `AppConfig` (persist across server switches, not per-connection).
- **Android: ConnectScreen** — apps section with `FilterChip` (Allowed/Blocked mode) + "Select apps" button + column display with app names (resolved via `PackageManager`). DNS section with servers text field.
- **Android: connect flow** — app settings passed directly to `KvnVpnService.start()` parameters, bypassing DataStore race condition.
- **Android: AppPickerScreen layout** — `Box` with `fillMaxSize()` overlay pattern for spinner, avoiding `if/else` subtree swap.

### Removed

- **Android: split-tunnel modules** — removed `RoutingManager`, `TcpProxy`, `UdpProxy`, `ExcludedIpSet`, `DnsInterceptor`, `DnsResolver`, `DnsTracker` (and their tests). Replaced by OS-native per-app filtering.

## [0.4.3] — 2026-06-19

### Changed

- **Windows: install-web.ps1** — удалена установка Windows Service (был `New-Service`, потом `sc.exe create`,
  ошибка 1639). Теперь скрипт только копирует бинарник и создаёт .lnk-ярлыки в Пуск и на рабочий стол.
  Пользователь запускает `kvn-web.exe` через ярлык, закрывает окно — `defer Restore()` чистит прокси.
  Опциональный `-Startup` для автозапуска при входе в систему.
- **Windows: system proxy** — теперь пишется в реестр активного пользователя (`HKEY_USERS\<SID>\...`)
  через `WTSGetActiveConsoleSessionId` + `WTSQueryUserToken`, а не в `HKEY_CURRENT_USER` (`LocalSystem`).

### Fixed

- **Windows: proxy_windows.go** — `Set()` возвращал `nil` вместо ошибки при неудаче открытия реестра.
- **Windows: crash recovery** — при запуске проверяется, не висит ли в реестре мёртвый прокси
  от предыдущего краша (`ProxyEnable=1` + `ProxyServer=127.0.0.1:2310`). Если обнаружен —
  очищается (`ProxyEnable=0`), чтобы после disconnect интернет работал напрямую.

### Added

- **GeoIP/GeoSite/External Sources для роутинга** — поддержка динамических источников правил роутинга:
  GeoIP, GeoSite, CIDR и кастомные URL-списки, которые резолвятся в плоские CIDR/домены на старте.
  - `SourceRule` тип с полями `geoip`, `geosite`, `cidr`, `url` в `RoutingCfg`.
  - `include_sources` / `exclude_sources` для client config.
  - Модуль `routing/resolver.go` — скачивание/кеширование geoip.dat/geosite.dat (v2fly protobuf),
    парсинг, раскрытие в CIDR/домены, merge + dedup с плоскими списками.
  - `geoip: "private"` — built-in alias для RFC 1918 + CGNAT + ULA.
  - Graceful degradation: битый URL/файл → warning, работа со статикой.
  - Refresh button в Web UI (POST `/api/config/refresh-sources`) с атомарной подменой `RuleSet`.
  - Proto-генерация для geoip.dat/geosite.dat в `src/internal/routing/geoip/`.
  - Web UI: карточки sources в секции Routing с выбором типа и кнопка Refresh.
  - Android: кнопка Refresh Sources на экране настроек.
- **Relay: direct_sources** — поддержка динамических источников (`direct_sources`) в relay terminator.
  - `RelayRoutingCfg`: новые поля `DirectSources`, `GeoIPPath`, `GeoSitePath`, `GeoIPURL`, `GeoSiteURL`, `SourceTTL`.
  - `Resolver.ResolveSources()` — публичный метод для резолва произвольного списка источников.
  - `resolveDirectSources()` — резолв источников и мерж в `DirectRanges`/`DirectDomains` в relay bootstrap.

## [0.4.0] — 2026-06-18

### Added

- **Relay mode** — новый тип узла `relay`, работающий как транзитный сервер между client ↔ relay ↔ server.
  Поддерживает TUN-режим (relay-terminator) и proxy-режим. Управление upstream через `RelayConfig.Upstreams`.
  - `src/cmd/relay/main.go` — точка входа relay-узла.
  - `src/internal/bootstrap/relay/` — bootstrap: сервер, handler, bridge, NAT, upstream selection.
  - `RelayRouter` — маршрутизация по `label` из `RelayConfig.Routes`, клиент указывает `relay_label`.
  - `RelayBridge` — проброс трафика между downstream и upstream через `transport.StreamConn`.
  - **QUIC relay mode** — relay-транспорт поверх QUIC с переключением по `config.Transport`.
  - **Relay terminator** — relay может завершать TUN-туннель (режим terminator) или проксировать.
  - Документация: `docs/en/relay.md`, `docs/ru/relay.md`, примеры в `examples/relay/` и `examples/relay-terminator/`.
  - CI: сборка relay-бинарника в `scripts/build.sh`.
- **kvn-web: multi-server** — Web UI теперь поддерживает несколько серверов.
  - `WebUIConfig` с `ActiveServer` + `Servers: []ServerEntry` и автоматической миграцией.
  - CRUD через admin API: create, update, delete, select active.
  - `loadWebUIConfig()` / `saveWebUIConfig()` — загрузка и сохранение списка серверов.
  - `mergeConfig()` / handler_config.go: обновление `ActiveServer` + `Servers` при create/update.
  - Frontend: селектор серверов, кнопки Add/Copy/Rename/Delete в `App.tsx`.
  - DDD spec + plan + data-model в `specs/archived/multi-server/`.
- **Android client: multi-server** — полноценное управление несколькими серверами.
  - `AppConfig` (activeServer + servers[]), `ServerEntry` (name + config), `AppConfigStore` с автоМиграцией.
  - `MainViewModel`: CRUD методы, dirty-флаг, сортировка (активный сверху, остальные A→Z).
  - `ConnectScreen`: `ExposedDropdownMenuBox` селектор, кнопки Add/Copy/Rename/Delete с иконками и семантическими цветами, dirty-диалог.
  - QR: импорт добавляет новый сервер (`Imported <timestamp>`), экспорт — активный сервер.
  - Data model: `data-model.md`, spec, plan, tasks, verify.
- **Android client: dark theme** — Material 3 darkColorScheme в цветах kvn-web.
  - `Color.kt`: палитра #161616/#222/#1a5a9e/#2e7d32/#b71c1c/#ff9800.
  - `MainActivity.kt`: обёртка в `MaterialTheme(colorScheme = DarkKvnWebColorScheme)`.

### Changed

- **Android: CRUD кнопки** — заменены с `OutlinedButton` + текст на `Button` + иконки (`Add`, `ContentCopy`, `Edit`, `Delete`) с семантическими цветами (зелёный/синий/оранжевый/красный).
- **Android: зависимость `material-icons-extended`** — добавлена для иконки `ContentCopy`.
- **Android: QR парсинг** — `WebConfig.auto_reconnect` изменён с `Boolean` на `Boolean?` для совместимости с web JSON.
- **CI: сборка relay** — в `scripts/build.sh` добавлена сборка relay-бинарника.

### Fixed

- **Android: QR не парсился из kvn-web** — `auto_reconnect: null` в web JSON вызывал ошибку десериализации. Исправлено: `Boolean → Boolean?`, fallback `?: true`.
- **Gosec G304 (CWE-22) в `config/webui.go`** — `os.ReadFile(path)` заменён на `os.DirFS(dir) + fs.ReadFile(root, name)` для scoped file access (Go 1.24+).

### Removed

- **examples/relay/** — старые примеры заменены на `examples/relay-terminator/`. Архив: `specs/archived/relay-terminator/`.

## [0.3.1] — 2026-06-12

### Fixed

- **Android: reconnect loop** — исправлен бесконечный цикл переподключения при потере соединения.

## [0.3.0] — 2026-06-09

### Added

- **MaxMessageSize limit** (`ClientConfig`) — новый параметр для защиты от OOM в QUIC-транспорте.
  По умолчанию 10 MB. При превышении возвращается `ErrMessageTooLarge`, соединение закрывается.
  Доступен в Web UI как поле "Max Message Size (bytes)".
- **TunnelTimeout** (`ClientConfig`) — таймаут бездействия туннеля в секундах (default: 30).
- **ProxyMaxConcurrency** (`ClientConfig`) — максимальное количество одновременных прокси-соединений
  (default: 1000, только для mode=proxy).
- **QUIC proxy mode** — QUIC-транспорт теперь поддерживается не только в TUN-, но и в proxy-режиме.
  `proxy.StreamConn` унифицирован под `transport.StreamConn`. При недоступности QUIC — fallback на TCP.
- **QUIC obfuscation** — XOR-обфускация QUIC-трафика для обхода DPI: 8-байтовый nonce,
  полный XOR полезной нагрузки. Nonce выводится через TLS `ExportKeyingMaterial`.
  Конфигурация: `obfuscation.enabled`.
- **DNS-роутинг** — маршрутизация DNS-запросов по суффиксу домена. Новый `DomainMatcher.MatchDomain()`,
  `RuleSet.MatchDomain()`, `RouteNone` sentinel. Парсер DNS-вопросов (`ParseDNSQuestion`).
  В proxy-режиме — прямой TCP-диал для `RouteDirect` доменов до резолва IP.
- **uTLS (TLS fingerprint obfuscation)** — `DialWithUTLS()` с Chrome fingerprint `HelloChrome_Auto`,
  опциональный fallback на `crypto/tls`. Конфигурация: `obfuscation.utls`.
- **WebSocket padding** — дополнение WS-сообщений случайным padding-фреймом (4 байта длины + паддинг).
  Конфигурация: `obfuscation.padding`.
- **SNI rotation** — случайный SNI из списка `tls.sni` при каждом подключении (`SelectSNI`).
- **WSPath whitelist** — конфигурируемый список разрешённых WS-путей на сервере (`ws_paths`, по умолчанию `["/tunnel"]`).
- **netlink migration** (partial) — `SetIP`/`SetMTU`/`DisableGSO` переведены на
  `github.com/vishvananda/netlink`. Управление маршрутами (`addDefaultRoute`,
  `removeDefaultRoute`, `AddExcludeRoute`, `RemoveExcludeRoute`, `SaveDefaultRoute`)
  оставлено на `exec.Command("ip")` — netlink.RouteDel с частичным совпадением
  может удалить физический default route вместо TUN-маршрута.
- **Import/Export QR UI** — экспорт конфига в JSON (буфер обмена), импорт через текстовое поле,
  генерация QR-кода. Система toast-уведомлений.
- **Web UI: uiLogCore** — `zapcore.Core`-обёртка для структурированной отправки логов в SSE:
  извлечение `action`, `ip`, `ts` из полей лога. Фильтр по уровню, поиск по строке.
  Автоматический скролл логов.
- **Proxy semaphore** — ограничение одновременных прокси-соединений через `sem chan struct{}` (default: 1000).
- **Permanent TUN reader** — выделенная goroutine с channel-based результатом (`tunReaderCh`),
  устранены per-read goroutines.
- **Sync.Pool для proxy-буферов** — `proxyBufPool` для 4KB буферов, устранено per-call выделение.
- **BoltDB timeout** — `bbolt.Open` с `Timeout: 1s` для предотвращения зависания при заблокированной БД.
- **Session manager: lock ordering** — отдельный `cancelFuncsMu` для карты cancel-функций;
  двухфазное удаление (сбор ID под `mu`, отмена после unlock) — устранён potential deadlock.
- **Admin server: логирование ошибок** — `listSessions`/`deleteSession` логируют ошибки `json.Encode`.
- **kvn-web на Windows** — `internal/tun` разделён на linux/заглушку, `GET /api/platform`,
  фронтенд скрывает TUN-mode на не-Linux. CI собирает kvn-web для всех платформ.
- **System proxy** — новый пакет `internal/systemproxy/`: автоматическая установка/восстановление
  `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` при старте/стопе proxy-режима. Linux env vars + systemd
  drop-in override (best-effort). macOS/Windows — stub (итеративное расширение).
  UI чекбокс "Use as system proxy" в блоке Proxy настроек.
  macOS: `networksetup` webproxy/securewebproxy с восстановлением оригиналов.
  Windows: реестр `HKCU\…\Internet Settings` с сохранением состояния.
  Recovery при краше: детект orphaned прокси при старте, лог, очистка.
- **Transparent proxy: domain-based DNS routing** — DNS proxy проверяет домен из DNS-запроса через `RouteSet.MatchDomain()`. Для excluded доменов запрос резолвится локально через оригинальные nameserver-ы (из backup `/etc/resolv.conf`), а не через туннель. Это позволяет exclude_domains работать в transparent mode.
  - `dnsproxy.Server`: `SetRouteFunc()`, `SetOrigResolvers()`, `resolveDirect()`, `extractDNSDomain()`.
  - `ResolvConfBackup.Nameservers()` — сохранение оригинальных DNS-серверов.
  - `proxy.go`: routeSet создаётся до DNS proxy, `RouteFunc` и `OrigResolvers` передаются в DNS proxy.
- **Debug logging для transparent-соединений** — `proxy.Listener.SetLogFn()` + логи в `handleTransparent`.
- **TunDemux** (`src/internal/tunnel/demux.go`) — single-reader TUN demultiplexer:
  `Register(ip4, ip6, chan)` / `Unregister(ip4, ip6)`, `parseDestIP()`.

### Changed

- **cmd/client/main.go** — сокращён с ~787 до 25 строк: вся логика вынесена
  в `internal/bootstrap/client/` (client.go, tun.go, proxy.go, reconnect.go, killswitch.go).
- **cmd/server/main.go** — сокращён с ~632 до 32 строк: вся логика вынесена
  в `internal/bootstrap/server/` (server.go, handler.go).
- **internal/bootstrap/** — новые пакеты client (Client struct + New() + Run(),
  TUN reconnectLoop с backoff, runProxyMode, kill-switch, computeGateway)
  и server (Server struct + New() + Run(), buildMux(), startSighupHandler(),
  handleTunnel(), health endpoints).
- **internal/transport/transport.go** — единый `StreamConn` interface; дублирующие объявления
  удалены из tunnel/ и proxy/.
- **internal/bootstrap/client/dial.go** — общая функция `dialStream` для tun- и proxy-режимов,
  устранено дублирование.
- **internal/tunnel/session.go** — wsToTun декомпозирован на handler-методы
  (`handleDataFrame`, `handleCloseFrame`, `handleProxyFrame`); Session переиспользуется
  клиентом и сервером; параметры `tunnelTimeout` и `proxyConcurrency` из конфига.
- **internal/tunnel/session.go** — `matchClientIP` удалён (demux handles per-session dispatch).
  Server-side `startTunReader` регистрируется в `TunDemux` вместо прямого `tunDev.Read`.
  Client-side (TUN mode) не изменился.
- **internal/bootstrap/server/** — создаёт `TunDemux` в `New()`, передаёт в
  `handleStream` через `SetDemux`.
- **Магические числа** — заменены на конфигурируемые поля, именованные константы
  (`wsReadLimit`, `CIDRMaskV4Bits`, `CIDRMaskV6Bits`).
- **Obfuscation config** — `bool → *ObfuscationCfg` (struct с `Enabled`, `UTLS`, `Padding`);
  обратная совместимость: `obfuscation: true` → `{enabled: true}`.
- **QUIC obfuscation rewritten** — nonce через TLS Exporter вместо random; полный XOR payload
  (не только длины); убран параметр `isClient`.
- **Web UI: default transport** — изменён с `"tcp"` на `"quic"`, obfuscation по умолчанию включён.
- **Server: tunnel handler** — перемещён с `/tunnel` на `/`; `allowedWSPath()` возвращает 404.
- **QUICConn** — `WriteMessage` защищён mutex'ом; `Close()` закрывает и connection и stream.
- **Proxy: ForwardToWS → ForwardToStream** — обобщено под `transport.StreamConn`.
- **Proxy listener: NewListener** — опциональный variadic `proxyConcurrency`.
- **Proxy listener: `Listener.logFn`** — добавлено поле `logFn` и методы `SetLogFn`/`logf` для отладки transparent-соединений.
- **Proxy goroutine nil-guard** — проверка proxyStreams в FrameTypeProxy handler.
- **TUN: SetInterruptibleRead** — удалён (больше не нужен с permanent reader).
- **DNS: DomainMatcher.SetCtx** — контекст для обновления DNS-кэша.
- **Reconnect: sleepWithContext** — `time.NewTimer` + `defer timer.Stop()` вместо `time.After`.
- **Config: secretFromEnv/warnSecretInFile** — принимают `prefix` вместо глобальной переменной.
- **internal/ratelimit/** — извлечён rate-limiter (IPRateLimiter, SessionPacketLimiter).
- **internal/proxy/stream.go** — SessionStreams.M сделан private `m`,
  добавлен конструктор NewSessionStreams().
- **proxySem глобальная → поле Session** — убрана глобальная переменная,
  proxySem инициализируется в NewSession().

### Fixed

- **TUN mode packet loss (33–66%) при нескольких сессиях** — все сессии
  (TUN + proxy) на сервере конкурировали за чтение из одного TUN-устройства.
  Пакет для TUN-клиента мог прочитать proxy-сессия (75% шанс) и дропнуть.
  Исправлено: добавлен `TunDemux` — одна горутина читает из TUN и диспатчит
  пакет в сессию по destination IP. Non-blocking send.
- **Critical: QUIC WriteMessage сбрасывал write deadline** — `SetWriteDeadline(time.Time{})` внутри `WriteMessage` отменял deadline, установленный caller'ом (TUN/proxy). TUN-записи могли зависать навсегда. Исправлено: deadline больше не сбрасывается.
- **QUIC keepalive не триггерил reconnect** — ошибка записи keepalive только логировалась (`continue`), соединение висело мёртвым. Исправлено: keepalive возвращает ошибку в errgroup → сессия закрывается → reconnect с backoff.
- **WebSocket keepalive goroutine утекала** — горутина пингов жила вечно после `Close()`, продолжая писать в закрытый сокет и плодить `"ping error"` каждые 25с. Исправлено: добавлен `stopCh`, горутина останавливается при `Close()`.
- **Windows: transparent proxy в UI** — чекбокс "Transparent proxy" показывался на всех платформах, хотя функция Linux-only. Исправлено: `GET /api/platform` возвращает `transparent_supported`, UI скрывает опцию на Windows/macOS.
- **Windows cross-compile** — `getOriginalDst()` использует Linux-only `SYS_GETSOCKOPT`. Исправлено: код вынесен в `listener_linux.go` с build tag, добавлена заглушка `listener_other.go` для !linux.
- **SIGHUP reload fix** — startSighupHandler использует сохранённый путь конфига вместо type assertion.
- **Proxy frame: deadline exceeded** — `runProxySession` не завершается при `Timeout() == true`.
- **Proxy frame: zero-length close** — `HandleIncomingFrame` закрывает стрим при пустом payload.
- **Tunnel: close frame on dial failure** — `wsToTun` отправляет close-фрейм при неудачном proxy-диале.
- **UI logs stream** — `zap.WrapCore` вместо `zap.Hooks` для корректной передачи полей лога.
- **WebUI broadcast lifecycle** — `broadcastLogs`/`broadcastStatus` стартуют в `Serve()` с серверным context.
- **Critical: config prefix race** — устранена глобальная переменная prefix в конфиге.
- **QUIC dial background ctx** — `Dial` принимает `ctx context.Context`, таймаут через `WithTimeout`.
- **Critical: SO_ORIGINAL_DST errno 0** — `getOriginalDst()` всегда падала с `errno 0` из-за классической Go ошибки: `syscall.Errno(0)` при присваивании в `error`-интерфейс становится non-nil. Исправлено: `var opErr error` → `var errno syscall.Errno`, проверка `errno != 0`.

### Removed

- **pkg/api/** — мёртвый пакет удалён.
- **cmd/client: tunToWS, wsToTun, tunReadInterruptible** — дублированный
  data-path (-114 строк) заменён на вызов tunnel.NewSession(...).Run(ctx).
- **cmd/server: дублированная логика** — ~600 строк бутстрапа перенесены в internal/bootstrap/server.
- **exec.Command("ip")** из tun.go — все манипуляции с TUN через netlink API.
- **Зависимость от os/exec, strconv, strings** в tun.go.
- **Дублирующиеся StreamConn interface** — единый `transport.StreamConn`, type alias в tunnel/ и proxy/.
- **Глобальная prefix в config** — заменена на параметр функции.
- **SetInterruptibleRead** из TUN reconnect.

## [0.2.0] — 2026-06-04

### Fixed

- **Клиентские падения "too many segments"** — при смене default route на TUN
  интерфейс клиента падал с ошибкой `tun read: too many segments`. Причина: GSO/GRO
  super-пакеты от ядра. Исправлено: отключение TUN offloading через `TUNSETOFFLOAD`
  ioctl=0 на TUN-устройстве (клиент и сервер).
- **Серверные падения "too many segments"** — та же проблема, исправлена
  отключением GSO на стороне сервера.
- **IPv6 exclude routes** — добавлено корректное пропускание IPv6 CIDR
  (`::1/128`, `ff00::/8`, `fe80::/10`) вместо попытки добавить их с IPv4-gateway.
- **RTNETLINK: File exists** на exclude routes — заменён `ip route add` на
  `ip route replace` для всех exclude-маршрутов.
- **Спам rate limit логов** — повторяющиеся `"packet rate limited"` теперь
  логируются не чаще раза в секунду на сессию.

### Added

- **DisableGSO()** — новый метод в `TunDevice` интерфейс, вызывается
  на клиенте и сервере после настройки TUN.
- **scripts/install-client.sh** — скрипт установки клиента из GitHub release
  с поддержкой `--mode proxy`, `--service` (systemd), SHA256-верификацией.
- **SHA256 checksums** в CI — для каждого архива релиза генерируется
  `.sha256`, таблица с хэшами в release notes.
- **scripts/build.sh** — аргумент `client`/`server`/`both` для сборки
  только нужного бинарника.

### Changed

- **scripts/build.sh** — добавлены ldflags `-s -w`.
- **install-server.sh** — использует SHA256-верификацию из CI.
- **Rate limit logging** — подавление повторяющихся warn-логов.

## [1.0.0] — 2026-05-14

### Added

- VPN-туннель через WebSocket Binary Frames поверх TLS 1.3
- TUN-интерфейс на стороне клиента
- IP-пул с динамическим выделением (IPv4 + IPv6)
- Сессионный менеджмент с BoltDB-персистентностью
- Гибкая маршрутизация: server/direct, CIDR, DNS-имена, отдельные IP
- Ordered rules для конфликтующих маршрутов
- DNS-резолвер с in-memory TTL-кэшем
- Аутентификация: token-based, JWT, basic
- Keepalive (PING/PONG) и контроль сессий
- nftables MASQUERADE для server-side NAT
- Prometheus-метрики (active_sessions, throughput, errors)
- App-layer encryption (AES-256-GCM) для Data-фреймов, per-session key derivation через HMAC-SHA256
- SOCKS5 + HTTP CONNECT proxy listener
- Docker multi-stage build
- docker-compose оркестрация
- Документация на английском и русском

## [0.4.5] — 2026-06-20

### Fixed

- **kvn-web: race condition disconnect→connect** (`handler_connect.go`, `server.go`, `state.go`).
  `handleDisconnect` дёргал `cancel()` и через 500ms чистил state, не дожидаясь полной остановки
  старого клиента. Если нажать "Connect" сразу после "Disconnect" до завершения cleanup
  старого `cl.Run()`:
  - Новый клиент ставил iptables, system proxy, DNS proxy
  - Старый через `defer` сносил только что поставленные правила
  - Трафик тормозился/не работал
  Исправлено:
  - `Server.connectMu` (`sync.Mutex`) — serializes connect/disconnect
  - `AppState.doneCh` — канал, закрываемый после полного выхода `cl.Run()`
  - `handleDisconnect` ждёт `doneCh` (до 3с) вместо хардкодного `Sleep(500ms)`
- **DNS proxy: silent ошибка восстановления resolv.conf** (`proxy.go:160-174`).
  `_ = resolvBackup.Restore()` проглатывал ошибку. Если `resolvectl revert` или
  `WriteFile` падал (нет прав, read-only symlink), resolv.conf оставался на мёртвый
  `127.0.0.54:53` → все DNS-запросы таймаутят → трафик тормозится.
  Исправлено: логируем ошибку через `c.logger.Warn(...)`.
- **Killswitch не чистился при graceful disconnect (TUN mode)** (`tun.go:41-42`).
  `reconnectLoop` при `ctx.Done()` выходил по `return` без `removeKillSwitch()`.
  Если до отключения была неудачная попытка reconnect (killswitch включён),
  он оставался в nftables → весь трафик заблокирован.
  Исправлено: `removeKillSwitch(c.cfg, c.logger)` перед `return` при `ctx.Done()`.

## [0.4.4] — 2026-06-20

### Fixed

- **Critical: HTTP/2 `bad_record_mac` через proxy-туннель** (сервер, `session.go:328-335`).
  TLS-записи от клиента приходили в неправильном порядке: `handleProxyFrame` для существующего
  стрима запускал `conn.Write()` в отдельной горутине (`go func`). При быстрой отправке нескольких
  фрагментов (HTTP/2 Magic + SETTINGS + HEADERS) горутины гонялись за write-lock сокета,
  и YouTube получал TLS-записи не по порядку → `bad_record_mac alert`.
  Исправлено: запись синхронная, без горутины. HTTP/1.1 не страдал, потому что отправлял
  всё одним фрагментом.
- **Transparent proxy: 1-байтовый TLS-фрагмент** (`listener.go:333-361`).
  `MultiReader(bytes.NewReader(firstByte), client)` в первом `Read()` возвращал
  только 1 байт (content type TLS-записи), а остаток ClientHello — во втором.
  На сервере YouTube получал 1 байт, потом остаток ← работает, но неоптимально.
  Исправлено: `prependConn.Read()` копирует `pending` + немедленно читает данные из сокета
  за один вызов.
- **`isSystemdResolved()` не детектился при относительном symlink** (`dnsproxy.go:356-373`).
  `/etc/resolv.conf → ../run/systemd/resolve/stub-resolv.conf` не распознавался как
  systemd-resolved, потому что `os.Readlink` возвращал относительный путь, а сравнение
  шло с абсолютным. Исправлено: `filepath.EvalSymlinks` для канонического пути.
  Без этого фикса `OverrideResolvConf` через `os.WriteFile` перезаписывал сам
  stub-файл systemd-resolved, ломая DNS.
- **Use-after-free в `handleProxyFrame`** (`session.go:328-333`).
  `data` — sub-slice `f.Payload`, который освобождался `defer f.Release()`
  до завершения горутины с `conn.Write(data)`. Исправлено: `make([]byte, len(data)); copy(...)`.
- **DNS proxy запускался до хендшейка** (`proxy.go:126-158`).
  Раньше `OverrideResolvConf` вызывался сразу, до подтверждения туннеля.
  Если хендшейк падал, resolv.conf уже был изменён. Исправлено: DNS proxy стартует
  только после успешного `FrameTypeHello` от сервера.
- **iptables MSS clamping** (`iptables_linux.go:43`).
  Добавлен `--clamp-mss-to-pmtu` в transparent-правила.

### Added

- **System proxy: авто-исключение сервера из NO_PROXY / ProxyOverride** (`client.go:177-251`).
  IP и домен сервера (из `config.server`) автоматически добавляются в `NO_PROXY` и
  Windows `ProxyOverride` при `system_proxy: true`. Предотвращает loop (попытку
  подключиться к серверу через самого себя).
  - Для iptables (transparent) — CIDR `ip/32`.
  - Для `NO_PROXY` — `ip,domain` (и IP, и домен, на случай изменения DNS).
- **Debug-логи для proxy write** (`session.go`): hex первых байт данных,
  записываемых в YouTube, при `log.level: debug`.
