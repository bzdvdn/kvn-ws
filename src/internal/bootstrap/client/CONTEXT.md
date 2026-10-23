# Context — src/internal/bootstrap/client

## Purpose

Клиентский bootstrap: запускает туннель KVN в одном из двух режимов — `tun` (перехват всего IP-трафика через TUN-устройство) и `proxy` (локальный SOCKS5/HTTP-прокси поверх N мультиплексированных соединений). Отвечает за dial, handshake, переподключение с backoff, split-tunnel, DNS-прокси, kill switch и корректный откат системных изменений.

## Responsibilities

- Владеет: циклом подключения/переподключения, TUN-конфигурацией (IP/MTU/routes), локальным proxy-listener'ом, multi-channel (secondary WS) и cleanup'ом системного состояния (routes, resolv.conf, system/transparent proxy, nftables).
- Не владеет: типами и загрузкой конфига (`src/internal/config`), реализацией транспортов (`src/internal/transport`), протоколом кадров/handshake (`src/internal/protocol`, `src/internal/transport/framing`), логикой пересылки внутри сессии (`src/internal/tunnel`), матчерами/правилами (`src/internal/routing`).

## How it works

- Два конструктора: `NewFromConfig(cfg)` — для встраивания (Web UI, desktop), `New()` — CLI с `--config`; оба нормализуют DNS-дефолты, парсят master-key и резолвят `routing.*_sources` через `routing.Resolver`.
- `Run(ctx)` — единственная точка входа: ставит `signal.NotifyContext(SIGINT/SIGTERM)`, в режиме `proxy` опционально включает system proxy и прозрачный прокси (только root), иначе открывает TUN и уходит в `reconnectLoop`.
- `reconnectLoop`: чистит протухшие DNS/маршруты → `dialStream` (QUIC с автоматическим fallback на WS) → при ошибке `applyKillSwitch`/backoff → при успехе `removeKillSwitch`, сброс backoff → `runSession` (сессия живёт до ошибки, затем цикл повторяется).
- `runSession`: handshake (`handshakeSession`) → `configureTun` (IP/MTU/GSO/cipher) → `addBypassRoutes` (exclude-CIDR сервера и правил + default gateway) → `buildTunRouter` → `setupDNSProxy` → `tunnel.NewSession(...).Run()`; всё снимается через `defer` в обратном порядке.
- Proxy-режим: `runProxyMode` диалит `proxy_connections` (по умолчанию 10) потоков последовательно, каждый с полным handshake (all-or-nothing), затем `runProxySessionMulti` — по одному `proxy.Manager` на слот, round-robin распределение, DNS и QUIC-keepalive только на slot 0.
- `bindSecondaryChannel`: второй WS-поток с тем же `SessionId` для UDP/батчинга; при потере переподключается до 5 раз с удвоением задержки.
- Метрики: после успешного dial поток оборачивается в `transport.CountingStreamConn`, счётчики пишутся в `metricclient.Collector`.

## Key files

- `client.go` — `Client`, `NewFromConfig`/`New`, `Run`, `StopTun`, режимы и lifecycle.
- `tun.go` — `reconnectLoop`, `runSession`, handshake, TUN/DNS/routes setup, secondary channel.
- `proxy.go` — proxy-режим: слоты, listener, routing, direct-pipe, keepalive.
- `dial.go` — `dialStream`/`dialStreamWith`, `transport.FactoryConfig`, QUIC→WS fallback.
- `killswitch.go` + `reconnect.go` — nftables `kvn-kill` и exponential backoff с jitter.
- `tun_linux.go` / `tun_darwin.go` / `tun_windows.go` — платформенные `setupDNS`/`applyDNS`/`restoreDNS`.
- `helpers.go` — TLS-конфиг клиента и парсинг backoff.

## Public contracts

- `client.New() (*Client, error)`, `client.NewFromConfig(*config.ClientConfig) (*Client, error)`.
- Методы: `(*Client).Run(ctx) error`, `SetLogger(*zap.Logger)`, `SetMetricCollector(*metricclient.Collector)`, `StopTun()` (идемпотентный force-close TUN).
- Побочный контракт: `Run` блокирующий и сам подписывается на SIGTERM/SIGINT.

## Dependencies

- `src/internal/config`, `src/internal/tun`, `src/internal/tunnel`, `src/internal/transport` (+`websocket`/`quic` через blank import), `src/internal/transport/{tls,framing}`, `src/internal/protocol/{handshake,control}`, `src/internal/routing`, `src/internal/dns`, `src/internal/dnsproxy`, `src/internal/proxy`, `src/internal/systemproxy`, `src/internal/transparent`, `src/internal/crypto`, `src/internal/metrics/client`, `src/internal/logger`.

## Consumed by

- `src/cmd/client/main.go` — CLI-бинарь.
- `src/internal/webui/handler_connect.go` — `NewFromConfig` + `Run`/`StopTun` для Web UI.
- `src/internal/webui/state.go` — хранит `*client.Client`.
- Косвенно `src/cmd/desktop` — через сервис `kvn-web`.

## Gotchas

- `NewFromConfig` **мутирует** переданный `*config.ClientConfig` (DNS-дефолты, `DNSRouting`, resolved sources) — не переиспользуйте один конфиг для двух клиентов.
- Кэш источников: `New()` пишет в `<dir(config)>/.source-cache`, `NewFromConfig` — в `./.source-cache` (текущая директория).
- `tunDev.Read()` не принимает context, поэтому `Run` держит горутину `<-ctx.Done() → StopTun()`; без force-close сессия не разблокируется.
- TUN-режим требует root: `tun.NewTunDevice().Open()` вернёт ошибку; прозрачный прокси дополнительно проверяет `os.Geteuid()==0`.
- `dialStream` для `quic` всегда оборачивается `NewFallbackFactory(quic, ws)` — недоступность QUIC не фатальна.
- Kill switch применяется только после неудачного dial и снимается после успешного; правила называются `kvn-kill` (ip и, при `ipv6`, ip6).
- Эффективный MTU = `min(cfg.MTU, serverHello.Mtu)`, `0` означает «не задавать».
- DNS-прокси поднимается только если есть `routing`, `DNSRouting.Enabled` и хотя бы одно domain-правило; на Linux он подменяет `resolv.conf` и обязан восстановить его на выходе.
- Прокси-режим: при ошибке любого из N dial'ов закрываются уже открытые и весь цикл повторяется; повторный backoff растёт и после успешной сессии.
- Secondary channel обязан вернуть тот же `SessionId`, иначе поток закрывается; `batchOK` включается только по подтверждению сервера.
