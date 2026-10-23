# Context — src/internal/bootstrap/server

## Purpose

Серверный bootstrap: поднимает TLS-листенер (WS/WSS) и опционально QUIC на том же адресе, выполняет handshake/аутентификацию, выдаёт IP из пула, создаёт `tunnel.Session` и обслуживает health/metrics/admin.

## Responsibilities

- Владеет: конфигом сервера, пулами IPv4/IPv6 и их Bolt-персистентностью, `SessionManager`, ACL/rate-limit, NAT, TUN-устройством и `TunDemux`, TLS-конфигом, реестром активных сессий для secondary-каналов, HTTP-маршрутами.
- Не владеет: пересылкой пакетов внутри сессии (`tunnel`), протоколом handshake (`protocol/handshake`), административным API (`admin`), правилами маршрутизации relay (`bootstrap/relay`).

## How it works

- `New(configPath)`: грузит `ServerConfig` → logger → CIDR-ACL → `IPPool` (v4, опц. v6) с `BoltStore` → `SessionManager` (idle 300с, TTL 24ч, reclaim 10с по умолчанию) → метрики → TUN (`Open`, MTU, IP шлюза, `DisableGSO`) → `TunDemux` → NAT (`Setup`, опц. `Setup6`) → TLS → origin-checker → per-token bandwidth → master-key → IP/session rate limiters → `ServeMux`.
- Маршруты: `"/"` — tunnel-handler (проверка WS-запроса, allowlist `ws_paths`, IP rate limit, padding/no-pad), `/livez`, `/readyz`, `/health`, `/metrics` (за `admin.token` и отдельным лимитером 100/100).
- `Run(ctx)`: `signal.NotifyContext(SIGTERM, SIGINT)`, SIGHUP-обработчик, `tls.Listen` + `httpServer.Serve`, при `transport: quic` — параллельный `quictp.Listen` с `AcceptWithBackoff` (и обёрткой `ObfuscatedQUICConn` при `obfuscation.enabled`); graceful shutdown 5с; в defer'ах — NAT teardown, `sm.Stop`, закрытие TUN/Bolt.
- `handleTunnel`: `websocket.Accept` + keepalive (`control.DefaultPingInterval`, `pong_timeout` из конфига) → `handleStream`.
- `handleStream`: читает ClientHello с дедлайном 30с, декодирует, ищет токен (`auth.FindToken`); при ошибке шлёт `AuthError` и закрывает; при `Channel=="secondary"` уходит в `handleSecondaryStream`.
- Основной поток: генерирует 16-байтовый `session_id`, `sm.Create(...)` (учитывает `MaxSessions` и IPv6), генерирует crypto-salt, клэмпит MTU, определяет datagram-возможность, шлёт ServerHello (IP, MTU, gateway, transport-маркер), создаёт `crypto.SessionCipher`, `tunnel.NewSession` c `TunDemux`, регистрирует сессию в `tunnelSessRefs` до `Run`, затем по завершении — `CloseAll`, `sm.Remove`, закрытие потока.
- `handleSecondaryStream`: находит сессию по `SessionId`, сверяет `TokenName` и наличие активного `*tunnel.Session`; при `BatchSupport` включает batch-режим; отвечает ServerHello и биндит поток через `SetSecondary`; блокируется до `Done()`/ctx.
- SIGHUP: перечитывает конфиг и переключает уровень логирования; новый конфиг кладётся в `cfgPtr`.

## Key files

- `src/internal/bootstrap/server/server.go` — конструктор, TUN/пулы/NAT/TLS, `Run`, маршруты, SIGHUP, admin API.
- `src/internal/bootstrap/server/handler.go` — handshake, авторизация, создание сессии, secondary-канал.

## Public contracts

- `New(configPath string) (*Server, error)`, `(*Server).Run(ctx) error`.
- HTTP: `GET /livez`, `GET /readyz`, `GET /health`, `GET /metrics` (Bearer/token admin), WS/QUIC handshake на `/` с allowlist путей из `ws_paths`.
- Протокол: сначала `FrameTypeHello` (`ClientHello`), ответ `ServerHello` или `FrameTypeAuth` (`AuthError`); secondary — тот же hello с `Channel="secondary"` и `SessionId`.
- Конфиг читается из `config.ServerConfig`; hot-reload через SIGHUP.

## Dependencies

- `src/internal/{config,session,tunnel,transport/*,protocol/auth,protocol/control,protocol/handshake,acl,admin,nat,ratelimit,metrics,crypto,logger,tun}`; `quic-go`, `prometheus/client_golang`, `errgroup`.

## Consumed by

- `src/cmd/server/main.go` — единственный entrypoint (создаёт и запускает сервер).

## Gotchas

- `New` открывает TUN и делает NAT setup **до** старта листенеров: без root/`CAP_NET_ADMIN` сервер не поднимется (TUN), а ошибки NAT лишь логируются.
- SIGHUP фактически меняет только уровень логирования и `cfgPtr`; все рабочие пути читают `s.cfg`, поэтому токены, пулы, TLS и лимиты после reload **не обновляются**.
- `/readyz` всегда отвечает `"tun":"ok","nat":"ok"` — это не реальная проверка, а фиксированные значения.
- Tunnel-handler не проверяет путь `"/"` как обычный HTTP: любой запрос без `Upgrade: websocket` получает 404.
- При `tunnelSessRefs` secondary обязан прийти после основного hello; реконнект secondary к уже завершившейся сессии получит `"session not active"`.
- Datagram-режим включается только если и сервер (`udp_datagrams`), и клиент (маркер `quic-dgram`) его запросили.
- Шифрование включается лишь при непустом master-key; salt генерируется на каждую сессию (без salt клиент предупредит об отсутствии шифрования).
- `shutdownTimeout` фиксирован (5с); незавершённые сессии принудительно закрываются вместе с процессом.
- Rate limit `"/"` — по IP на авторизацию; `/metrics` имеет собственный лимитер, создаваемый на каждый `buildMux`.
