# Context — src/internal/session

## Purpose

Учёт серверных сессий: пул IP-адресов (v4/v6), жизненный цикл сессий с idle/TTL-экспирацией и лимитом на токен, персистентность аллокаций в BoltDB и per-token rate limiting.

## Responsibilities

- Владеет: `IPPool` (выдача/освобождение/резолв адресов), `SessionManager` (создание/удаление/список/активность/экспирация), `BoltStore` (bucket `allocations`/`allocations_v6`), `TokenBandwidthManager`.
- Не владеет: аутентификацией и выбором токена (`protocol/auth`), сетевой пересылкой (`tunnel`), конфигом сервера (`config`).

## How it works

- `NewIPPool(cfg, logger)`: парсит subnet/gateway/range; по умолчанию диапазон — от `subnet+1` до `broadcast-1`; gateway исключается при выдаче. `NewIPPool6` — то же для IPv6, но `start` сдвигается на случайное смещение внутри подсети (избегать одинаковых первых адресов между запусками).
- `Allocate(sessionID)` идемпотентен: повторный вызов с тем же ID возвращает тот же IP. Перебор линейный с `nextIP`, освобождение — `Release(sessionID)`.
- `LoadFromBolt`/`SetBoltStore`: при старте пул догружает сохранённые аллокации; каждые `Allocate`/`Release` вызывают `SaveAllocations` (warn при ошибке, без падения).
- `SessionManager.Create`: идемпотентно по `sessionID`; проверяет `maxSessions` на `tokenName`; выделяет v4 и, при `ipv6=true` и наличии `pool6`, v6 (при ошибке v6 освобождает v4); пишет `ConnectedAt`/`LastActivity`.
- `Start(idleTimeout, sessionTTL, interval)` запускает `reclaimLoop`: по тикеру `expireIdle` (по `LastActivity`) и `expireTTL` (по `ConnectedAt`); экспирация идёт в две фазы — удаление под `mu`, затем `cancelSession` под отдельным `cancelFuncsMu`.
- `Remove` декрементирует счётчик токена, освобождает IP и отменяет контекст сессии; `UpdateActivity` продлевает idle-таймаут; `List` отдаёт срез без копирования полей.
- `BoltStore`: bbolt с таймаутом открытия 1с, права `0600`; значения — строковые IP.
- `TokenBandwidthManager`: на токен создаётся `rate.Limiter` с `burst = bps`; токен без лимита (нет в конфиге или `<=0`) — без ограничений; `Reserve` возвращает задержку или `false`.

## Key files

- `src/internal/session/session.go` — `IPPool`, `Session`, `SessionManager`, экспирация, `MaxSessions`.
- `src/internal/session/bolt.go` — `BoltStore` и персистентность аллокаций.
- `src/internal/session/bandwidth.go` — `TokenBandwidthManager`.

## Public contracts

- `PoolCfg{Subnet, Gateway, RangeStart, RangeEnd}`; `NewIPPool`, `NewIPPool6`, `(*IPPool).Allocate/Release/Resolve/SetBoltStore/LoadFromBolt`.
- `Session` (ID, TokenName, AssignedIp, AssignedIpv6, RemoteAddr, ConnectedAt, LastActivity).
- `NewSessionManager(pool, logger)`, `Start`, `Stop`, `Create`, `Remove`, `Get`, `List`, `UpdateActivity`, `SetPool6`, `SetCancel`.
- `SessionManager.Create` возвращает `(*Session, ip, ip6, error)`; `MaxSessions(*net.IPNet) int`.
- `NewBoltStore`/`NewBoltStore6`, `SaveAllocations`, `LoadAllocations`, `Close`.
- `NewTokenBandwidthManager(map[string]int)`, `Allow(token, bytes)`, `Reserve(token, bytes) (time.Duration, bool)`.

## Dependencies

- `go.etcd.io/bbolt`, `golang.org/x/time/rate`, `go.uber.org/zap`.

## Consumed by

- `src/internal/bootstrap/server` и `src/internal/bootstrap/relay` — создание менеджеров/пулов.
- `src/internal/admin` — листинг/удаление сессий.
- `src/internal/tunnel/session.go` — обновление активности и резолв адресов.

## Gotchas

- `SaveAllocations` только делает `Put` для текущих аллокаций и **никогда не удаляет** ключи: освобождённые IP остаются в BoltDB и после рестарта `LoadFromBolt` снова пометит их занятыми.
- `expireIdle`/`expireTTL` не декрементируют `sessionCnt` — лимит `max_sessions` на токен «утекает» после экспираций (в отличие от `Remove`).
- `Remove` освобождает только v4 (`pool.Release`); v6-адрес в `pool6` не освобождается.
- `Start` с `interval <= 0` приведёт к панике `time.NewTicker`; `Stop` идемпотентен только благодаря `sync.Once`.
- Аллокация линейная и не пропускает границы: если `RangeStart > RangeEnd`, цикл не завершится по `ip.Equal(end)` (переполнение адреса) — конфиг должен быть валидным.
- `MaxSessions` вычитает только сетевой и broadcast адреса, gateway не учитывает — фактическая ёмкость на 1 меньше.
- `LoadFromBolt` не очищает уже существующие `allocated`, а `usedIPs` пересоздаёт — повторный вызов может оставить «фантомные» занятые адреса.
- `TokenBandwidthManager` держит мьютекс во время `ReserveN` (намеренно, ради атомарности) — это точка сериализации на горячем пути.
- `BoltStore` открывается с таймаутом 1с: второй процесс, держащий файл, получит ошибку открытия, а не будет ждать.
