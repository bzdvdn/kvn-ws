<!-- @sk-task docs-and-release#T1.2: CHANGELOG v1.0.0 (AC-007) -->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
