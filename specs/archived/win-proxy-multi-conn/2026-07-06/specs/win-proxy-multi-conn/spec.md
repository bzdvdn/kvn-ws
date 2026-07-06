# Win Proxy — Multi-Connection Architecture

## Scope Snapshot

- In scope: N параллельных WebSocket transport соединений (по умолчанию 10, конфигурируется через `proxy_connections`) в proxy mode вместо одного, устранение contention на `wmu` (sync.Mutex в WSConn.WriteMessage).
- Out of scope: изменения TUN mode, QUIC transport, server-side обработки.

## Цель

Пользователи Windows (и других платформ) в proxy mode получают стабильно низкую latency при множественных параллельных TCP-соединениях. Раньше все proxy-streams одного клиента конкурировали за `wmu` единственного WS-соединения — при N одновременно пишущих горутин каждая блокировалась на мьютексе, снижая throughput. Фича устраняет contention: каждый из N slot'ов владеет отдельным WS-соединением со своим `wmu`, распределение stream'ов — round-robin.

## Основной сценарий

1. Клиент запускается в proxy mode (не TUN).
2. `runProxyMode` читает `proxy_connections` из конфига (умолчание 10) и вызывает `dialProxySlots(N)`, которая создаёт N параллельных WebSocket-соединений к серверу, каждое проходит полный handshake.
3. `runProxySessionMulti` запускает SOCKS5/HTTP CONNECT listener и N read-loop горутин (по одной на слот).
4. При входящем прокси-соединении listener выбирает слот через round-robin (`nextSlot.Add(1) % numSlots`).
5. Потоки данных внутри одного слота мультиплексируются через `proxy.Manager` и `framing.FrameTypeProxy`.
6. При падении любого соединения весь набор закрывается, `runProxyMode` ждёт backoff и переподключает все N слотов заново.
7. TUN mode работает как раньше — одно соединение, никаких изменений.

## User Stories

- P1: Как пользователь proxy mode, я хочу чтобы параллельные TCP-запросы (например, веб-браузер с 6 вкладками) не создавали очереди на запись через единый мьютекс.
- P2: Как оператор, я хочу чтобы при обрыве одного WS-соединения клиент не терял другие (в будущем), но MVP — полный переподключатель.

## MVP Slice

N WS-соединений (по умолчанию 10) с round-robin распределением, полный reconnect всех слотов при ошибке любого. Закрывает AC-001, AC-002, AC-003 первым pass.

## First Deployable Outcome

Pass `scripts/test-proxy.sh` с N параллельными соединениями + ручная проверка logs на отсутствие `wmu` contention.

## Scope

- `src/internal/config/client.go`: поле `ProxyConnections` в `ClientConfig`, дефолт 10
- `src/internal/bootstrap/client/proxy.go`: `dialProxySlots(N)`, `runProxySessionMulti`, round-robin селектор
- `src/internal/bootstrap/client/dial.go`: `dialStream` — общая функция создания WS/QUIC соединения (уже есть, используется)
- Каждый slot — отдельный `proxy.Manager` со своим `transport.StreamConn`
- `src/internal/proxy/stream.go`: `Manager.stream` — один на слот (не меняется)
- Read-loop (`proxyReadLoop`) — по одной горутине на слот
- TUN mode — не затрагивается

## Контекст

- `WSConn.WriteMessage()` (websocket.go:162) захватывает `c.wmu.Lock()`. При одном соединении все пишущие горутины соревнуются за этот мьютекс.
- N по умолчанию равно 10, конфигурируется через `proxy_connections` в YAML/JSON.
- Решение использует горутины на слот — N read-loop + N Manager — каждый с собственным мьютексом.
- Round-robin не гарантирует равномерной загрузки (долгоживущий stream занимает слот), но устраняет главный источник contention.

## Зависимости

- `src/internal/transport/websocket.WSConn` — wmu contention target (не меняется)
- `src/internal/transport.StreamConn` — интерфейс транспорта (не меняется)
- none

## Требования

- RQ-001 Система ДОЛЖНА создавать `proxy_connections` (по умолчанию 10) параллельных WebSocket-соединений при старте proxy mode.
- RQ-002 Каждое соединение ДОЛЖНО пройти полный ClientHello↔ServerHello handshake.
- RQ-003 При обрыве любого из N соединений система ДОЛЖНА закрыть все N и переподключить полный набор.
- RQ-004 Новые прокси-стримы ДОЛЖНЫ распределяться по слотам через round-robin счётчик.
- RQ-005 TUN mode НЕ ДОЛЖЕН использовать множественные соединения — одно WS-соединение как раньше.
- RQ-006 DNS proxy ДОЛЖЕН привязываться к слоту 0 (первое соединение) для отправки DNS-ответов.
- RQ-007 QUIC keepalive ДОЛЖЕН работать на слоте 0 (как сейчас).

## Вне scope

- Динамическое изменение количества соединений (реконфигурация на лету).
- Graceful per-connection reconnect (без потери stream'ов других слотов).
- Server-side изменения — сервер обрабатывает каждое WS-соединение независимо, изменений не требует.
- Балансировка нагрузки между слотами сложнее round-robin (least-connections, weighted).
- TUN mode multi-connection.

## Критерии приемки

### AC-001 N параллельных WS-соединений в proxy mode (конфигурируемое N, default 10)

- Почему это важно: устраняет wmu contention, повышает throughput при множественных параллельных стримах
- **Given** клиент запущен в proxy mode
- **When** `runProxyMode` завершает установку соединений
- **Then** в логе присутствуют N записей "handshake complete" (где N = `proxy_connections`)
- Evidence: `grep -c "handshake complete" <client-log>` возвращает N

### AC-002 Round-robin распределение

- Почему это важно: гарантирует равномерное использование всех соединений
- **Given** N WS-соединений установлены
- **When** M последовательных прокси-запросов обработаны
- **Then** каждый слот получил приблизительно M/N запросов
- Evidence: метрики/логи показывают распределение ~равномерно

### AC-003 Полный reconnect при ошибке любого соединения

- Почему это важно: консистентность состояния — если одно соединение упало, данные остальных могут быть повреждены
- **Given** N WS-соединений активны
- **When** одно из соединений закрывается (например, обрыв сети)
- **Then** все N соединений закрываются, клиент входит в цикл reconnect с backoff
- Evidence: лог содержит "closing" для всех N, затем "connecting" с attempt++

### AC-004 TUN mode не затронут

- Почему это важно: TUN mode должен оставаться стабильным, multi-conn ему не нужен
- **Given** клиент запущен в TUN mode
- **When** `tun.Run` активен
- **Then** открыто ровно 1 WS-соединение
- Evidence: в логе одна запись handshake complete

## Допущения

- Сервер не требует изменений — каждое WS-соединение сервер обрабатывает как независимую сессию (идентифицируется по токену/сессионному ID).
- 10 соединений — достаточное количество для устранения wmu contention на типичных нагрузках (браузер, ~50 одновременных TCP-соединений).
- Количество можно менять через конфиг `proxy_connections`.
- Round-robin без состояния даёт достаточную равномерность для MVP.
- Все слоты подключаются к одному серверу с одинаковыми параметрами.

## Критерии успеха

- SC-001 Throughput в proxy mode при 100 параллельных TCP-стримах вырос не менее чем в 2× по сравнению с single-connection.
- SC-002 P99 latency write-очереди < 10ms (против >100ms на single-connection при 50+ стримах).

## Краевые случаи

- Ошибка handshake на одном из N слотов — все закрываются, reconnect.
- Сервер отклоняет одно соединение (например, лимит сессий) — reconnect всех.
- Клиент в TUN mode — multi-conn не создаётся.
- Все N слотов успешно соединились, но DNS-proxy не смог стартовать — TearDown всех слотов, reconnect.
- QUIC transport: при keepalive ошибке на слоте 0 — reconnect всех.

## Открытые вопросы

- Нужно ли добавить метрику `proxy_active_slots` для observability?
- Должен ли reconnect пытаться восстановить только упавший слот (graceful partial reconnect) в будущем?
