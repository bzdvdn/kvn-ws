# Context — src/internal/dnsproxy

## Purpose

Локальный UDP DNS-прокси на loopback: принимает запросы приложения, решает — отправить их через туннель или разрешить напрямую через исходные/системные резолверы, и ведёт учёт IP↔домен для domain-based маршрутизации.

## Responsibilities

- Владеет: UDP-сервером (`Server`), маршрутизацией DNS-запросов (route-by-domain), кэшем ожидающих ответов по stream ID, трекингом доменов через `dns.Tracker`, подменой/восстановлением `resolv.conf` на Linux.
- Не владеет: самим туннелем и его кадрами (`tunnel`, `framing`), правилами маршрутизации (`routing`), TTL-кэшем DNS (`dns`).

## How it works

- `New(listenAddr, upstreams...)` — variadic upstreams с дефолтом `1.1.1.1:53`, `8.8.8.8:53`.
- `Run(ctx)`: `ListenUDP`, цикл с read-deadline 1с (проверка ctx), каждый запрос копируется и уходит в отдельную горутину `forward`.
- `forward`: снимок конфигурации под `configMu.RLock`; если `routeDirect(qname)` истинно — `resolveDirect` (UDP к `origResolves`, 3с на попытку, перебор всех); иначе при установленном `stream` — `forwardViaTunnel`; иначе fallback — прямой TCP к upstreams (2-байтовый length-prefix, ответ ≤1500).
- `forwardViaTunnel`: `atomic.AddUint32` даёт stream ID, регистрирует канал в `pending`, шлёт кадр `FrameTypeDNS` (payload = `uint32 id` + raw query), ждёт ответ до 10с или ctx; `HandleDNSResponse` копирует ответ и доставляет его в канал.
- `SetStream`/`ClearStream` переключают режим «через туннель» (используется прозрачным прокси); `SetRouteFunc` задаёт предикат «напрямую», `SetOrigResolvers` — исходные системные резолверы.
- `SetTracker` + `TrackResponse` наполняют `dns.Tracker` (IP→домен), `SetDirectRouteFunc` получает IP из прямых ответов, чтобы клиент мог добавить kernel-exclude маршруты.
- Linux: `BackupResolvConf` сохраняет оригинал и nameserver'ы, `OverrideResolvConf` либо `resolvectl dns lo`, если `resolv.conf` — симлинк systemd-resolved, либо переписывает файл (nameserver первой строкой); `Restore` возвращает как было.
- `CleanupStaleDNS(proxyListen)` удаляет nameserver, указывающий на адрес прокси, оставшийся от убитой сессии.
- `Shutdown` закрывает UDP-сокет; `Run` завершается по ctx или на не-timeout ошибке чтения.

## Key files

- `src/internal/dnsproxy/dnsproxy.go` — `Server`, `Run`/`forward`/`forwardViaTunnel`, `HandleDNSResponse`, парсинг qname.
- `src/internal/dnsproxy/dnsproxy_linux.go` — `ResolvConfBackup`, `BackupResolvConf`, `OverrideResolvConf`, `Restore`, `CleanupStaleDNS` (Linux).
- `src/internal/dnsproxy/dnsproxy_darwin.go` / `dnsproxy_windows.go` — `CleanupStaleDNS`-заглушки/платформенные версии.
- `src/internal/dns/dns.go` — `Tracker`/`ParseDNSResponse` (используются здесь, но живут в другом пакете).

## Public contracts

- `New(listenAddr string, upstreams ...string) *Server`, `(*Server).Run(ctx) error`, `Shutdown() error`.
- Сеттеры: `SetStream`, `ClearStream`, `SetRouteFunc(func(domain) bool)`, `SetOrigResolvers([]string)`, `SetTracker(*dns.Tracker)`, `SetDirectRouteFunc(func([]netip.Addr))`.
- `HandleDNSResponse(streamID uint32, resp []byte)` — вызывается tunnel-слоем на кадр `FrameTypeDNS`.
- `StreamConn` — локальный структурный интерфейс (совместим с `transport.StreamConn`, но объявлен здесь независимо).
- Linux: `BackupResolvConf()`, `(*ResolvConfBackup).Restore/Nameservers`, `OverrideResolvConf(addr)`, `CleanupStaleDNS(addr)`.

## Dependencies

- `src/internal/dns` (`Tracker`, `ParseDNSResponse`), `src/internal/transport/framing` (`FrameTypeDNS`, `Frame.Encode`).

## Consumed by

- `src/internal/bootstrap/client` — TUN- и proxy-режимы, Linux DNS-override, очистка stale DNS.
- `src/internal/webui/server.go` — `CleanupStaleDNS` при старте Web UI.

## Gotchas

- `forwardViaTunnel` использует переданный `stream` без собственной сериализации; несколько горутин пишут в один `StreamConn` одновременно — корректность зависит от реализации транспорта (`WriteMessage` должен быть потокобезопасен).
- Ожидание ответа жёстко ограничено 10с; `pending` очищается и по таймауту, и после доставки — «поздний» `HandleDNSResponse` просто игнорируется.
- `extractDNSDomain` не поддерживает compression-указатели и берёт только первый QNAME; на нестандартных/сжатых вопросах доменное решение не сработает.
- Ответы из прямого TCP-fallback ограничены 1500 байтами — крупные DNS-ответы обрезаются (молча пропускаются при `respLen > 1500`).
- `resolveDirect` идёт только по UDP и возвращает первый успешный ответ; при пустом списке резолверов запрос просто теряется (клиент не получит ответ).
- `OverrideResolvConf` пишет `resolv.conf` с правами `0644` и **не** делает бэкап сам — порядок «Backup → Override → Restore» обязателен на стороне вызывающего.
- При systemd-resolved (`resolv.conf` — симлинк) правки идут через `resolvectl dns lo`, а восстановление — `resolvectl revert lo`; прямое редактирование файла там бессмысленно.
- `Run` не идемпотентен: повторный вызов перезапишет `s.conn` и утечёт предыдущий сокет.
- `HandleDNSResponse` копирует буфер намеренно: вызывающий может вернуть pooled-буфер сразу после вызова.
