# Context — src/internal/tunnel

## Purpose

Ядро пересылки одной сессии: двунаправленный мост между транспортом (`StreamConn`) и TUN-устройством, включая split-tunnel маршрутизацию, proxy-стримы, DNS-запросы, шифрование payload, вторичный канал (UDP) и QUIC-датаграммы.

## Responsibilities

- Владеет: `Session` (read/write циклы, обработчики кадров), `TunDemux` (один reader на общий TUN), классификацией UDP/TCP, per-stream очередями proxy, batched writer'ом secondary, жизненным циклом горутин и буферными пулами.
- Не владеет: транспортом (`transport/*`), форматом кадра (`framing`), правилами (`routing`), прокси-менеджером (`proxy`), сессиями/IP-пулом (`session`), шифрованием (`crypto`).

## How it works

- `NewSession(...)` принимает TUN, stream, `SessionManager`, лимитеры, коллекторы метрик, шифр, `proxy.SessionStreams`, таймауты; дефолты: `tunnelTimeout=30s`, `proxyConcurrency=1000`, DNS upstreams `1.1.1.1:53, 8.8.8.8:53`.
- `Run(ctx)`: стартует TUN-reader (напрямую или через `TunDemux`), опциональный datagram-reader, запускает `errgroup` из `wsToTun` и `tunToWS`, при наличии secondary — отдельный secondary-loop; паники в любой горутине перехватываются и превращаются в ошибку; `done` закрывается ровно один раз.
- `wsToTun`: обновляет активность сессии, применяет packet rate limit, читает сообщение с `SetReadDeadline(tunnelTimeout)`, декодирует `framing.Frame`, расшифровывает `FrameTypeData`, диспетчеризует по типу (`DATA`→TUN, `CLOSE`→выход, `PROXY`→proxy-стримы, `DNS`→горутина).
- `tunToWS`: берёт пакет из TUN-канала; при наличии `TunRouter` отдаёт пакет ему (router сам решает direct/tunnel), иначе применяет bandwidth limiter, пробует датаграмму для UDP, выбирает secondary для UDP, шифрует и отправляет `FrameTypeData` с write-deadline.
- `sendViaTunnel` (вызывается роутером) дублирует логику отправки: datagrams → secondary/batch → primary.
- Proxy-кадры: payload = `stream_id(4)` + `dst_len(2)` + `dst` + `data`. `dst_len=0` — закрытие стрима, payload < 6 — ACK. Новый стрим диалится асинхронно (`net.Dialer{Timeout:10s}`), начальные данные копятся в очереди на 2048, ёмкость ограничена `proxySem`; на каждый стрим — один упорядоченный writer.
- DNS-кадры: payload = `stream_id(4)` + raw query; ответ уходит тем же кадром; запросы форвардятся по UDP с таймаутом 5с на upstream, ответ ≤1500.
- Secondary-канал: отдельный writer-горутина, батчинг до 4096 байт (`secondaryCh` 2048); при переполнении/ошибке UDP молча дропается, primary не блокируется; при потере канала вызывается `onSecondaryLost` и владелец переподключает.
- `TunDemux`: один goroutine читает TUN и раскладывает пакеты по `chan tunReadResult` сессий по destination IP (v4/v6); при остановке рассылает ошибку всем; буферы из пула возвращаются, если адресат не найден.

## Key files

- `src/internal/tunnel/session.go` — `Session`, `Run`, ws/tun-циклы, frame-хендлеры, secondary/datagram.
- `src/internal/tunnel/demux.go` — `TunDemux`, `parseDestIP`, `parseIPProto`.
- `src/internal/tunnel/stream.go` — алиас `StreamConn = transport.StreamConn`.

## Public contracts

- `NewSession(...) *Session` (порядок аргументов стабилен — его используют client/server/relay).
- `(*Session).Run(ctx) error`, `Done() <-chan struct{}`, `SetTunRouter`, `SetSecondary`, `SetBatchMode`, `BatchEnabled`, `SetDatagrams`, `SetOnSecondaryLost`, `SetDemux`, `SetOutgoingInterceptor`.
- `OutgoingInterceptor func(payload []byte) (handled bool, err error)` — перехват исходящих в TUN пакетов (fake-DNS и т.п.).
- `NewTunDemux(tunDev, logger)`, `Register(ip4, ip6, ch)`, `Unregister(ip4, ip6)`; тип `tunReadResult` — внутренний.
- Форматы payload'ов `FrameTypeProxy`/`FrameTypeDNS` (см. выше) — контракт совместимости с клиентской стороной.

## Dependencies

- `src/internal/{transport,framing,protocol/handshake,proxy,routing,session,ratelimit,crypto,metrics,tun,logger,transport/quic}`; `golang.org/x/sync/errgroup`, `go.uber.org/zap`.

## Consumed by

- `src/internal/bootstrap/{client,server,relay}` — клиентские и серверные сессии.
- `src/integration/` — интеграционные тесты туннеля.

## Gotchas

- `Run` внутренне подменяет ctx через `errgroup.WithContext`; внешний ctx всё равно должен отменяться — иначе `tunToWS` будет ждать пакет вечно (TUN-read не отменяемый, его закрывает клиент через `StopTun`).
- Таймаут чтения primary — **фатален**: gorilla после read-timeout помечает соединение испорченным, поэтому сессия завершается, а не продолжает читать.
- При установленном `TunRouter` bandwidth limiter в `tunToWS` **не применяется** (router отправляет пакеты сам через `sendViaTunnel`).
- Шифруется только payload `FrameTypeData`; данные внутри `FrameTypeProxy` и DNS-запросы идут без app-layer шифрования.
- `proxySem` ограничивает число одновременных proxy-стримов; при переполнении стрим закрывается `writeProxyCloseFrame`.
- Очереди `dialStream`s (2048) при переполнении дропают начальные данные без уведомления — медленный стек может потерять первые байты стрима.
- Secondary-writer при ошибке записи останавливается и дропает UDP до перепривязки канала; primary при этом не страдает.
- `TunDemux` — на один TUN-девайс; регистрация идёт по IP, поэтому две сессии с одинаковым IP перетирают друг друга.
- Буферы `tunReadBufPool`/`proxyBufPool` надо возвращать (`putTunReadBuf`, `framing.ReturnBuffer`), иначе пул вырождается в аллокации.
- `FrameTypeDNS` обрабатывается в отдельной горутине с копией payload — `f.Release()` обязателен до запуска обработки.
