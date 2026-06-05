<!-- @sk-task docs-and-release#T1.2: CHANGELOG v1.0.0 (AC-007) -->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.3.0] — 2026-06-05

### Changed

- **cmd/client/main.go** — сокращён с ~787 до 25 строк: вся логика вынесена
  в `internal/bootstrap/client/` (client.go, tun.go, proxy.go, reconnect.go, killswitch.go).
- **cmd/server/main.go** — сокращён с ~632 до 32 строк: вся логика вынесена
  в `internal/bootstrap/server/` (server.go, handler.go).
- **internal/bootstrap/client/** — новый пакет: Client struct + New() + Run(),
  TUN reconnectLoop с экспоненциальным backoff, runProxyMode с reconnect,
  nftables kill-switch (apply/removeKillSwitch), computeGateway.
- **internal/bootstrap/server/** — новый пакет: Server struct + New() + Run(),
  buildMux(), startSighupHandler(), handleTunnel(), health endpoints.
- **internal/tunnel/session.go** — извлечён из cmd/client; Session переиспользуется
  и клиентом и сервером; nil-guards для sm/collectors/proxyStreams;
  SetTunRouter, SetInterruptibleRead, tunReadInterruptible.
- **internal/ratelimit/** — извлечён rate-limiter (IPRateLimiter, SessionPacketLimiter).
- **internal/proxy/stream.go** — SessionStreams.M сделан private `m`,
  добавлен конструктор NewSessionStreams().
- **proxySem глобальная → поле Session** — убрана глобальная переменная,
  proxySem инициализируется в NewSession().
- **SIGHUP reload fix** — startSighupHandler теперь использует сохранённый
  путь конфига вместо сломанного type assertion.
- **Proxy goroutine nil-guard** — проверка proxyStreams в FrameTypeProxy handler.

### Removed

- **pkg/api/** — мёртвый пакет удалён.
- **cmd/client: tunToWS, wsToTun, tunReadInterruptible** — дублированный
  data-path (-114 строк) заменён на вызов tunnel.NewSession(...).Run(ctx).
- **cmd/server: дублированная логика** — ~600 строк бутстрапа перенесены
  в internal/bootstrap/server.

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
