# Primary TCP backpressure и переиспользование TUN-буферов

## Scope Snapshot

- In scope: primary TCP снова работает через backpressure без drop-очереди; TUN read/write не аллоцируют буфер на каждый пакет.
- Out of scope: secondary UDP-очередь, routing KTalk, padding/no-pad, QUIC и Android.

## Цель

Пользователь TUN-режима получает предсказуемую задержку для обычного TCP: при медленном канале отправитель притормаживается, а не теряет пакеты из user-space очереди. Джиттер снижается за счёт отказа от per-packet аллокаций в TUN. Успех виден по отсутствию дропов primary, сохранению порядка кадров и нулевым аллокациям на горячем пути.

## Основной сценарий

1. Клиент пишет TCP-трафик в TUN; `tunToWS` классифицирует пакет.
2. UDP при наличии secondary уходит в прежнюю bounded-очередь; TCP пишется в primary синхронно.
3. Если WS-канал медленный, запись блокируется и естественно тормозит источник; кадры не теряются и не переставляются.
4. При ошибке записи сессия завершается штатно; secondary-поведение не меняется.

## User Stories

- P1: как пользователь обычного TCP через VPN, я хочу, чтобы при заторе канала не было скрытых дропов и retransmit-штормов.
- P2: как пользователь звонков, я хочу, чтобы secondary UDP не зависел от изменений primary.
- P3: как оператор, я хочу видеть меньше GC-джиттера на высоком pps.

## MVP Slice

- `AC-001` + `AC-002`: primary на backpressure, secondary-очередь без изменений.
- `AC-003` подключается тем же изменением, но проверяется отдельным alloc-тестом.

## First Deployable Outcome

- Сборка клиента/сервера без primary-очереди; существующие тесты secondary остаются зелёными; TCP-тест подтверждает доставку всех кадров при медленном writer'е.

## Scope

- Удаление primary drop-очереди из `tunnel.Session` и запись primary из dispatch-пути с backpressure.
- Сохранение secondary bounded-очереди, batching, max-age и no-pad без изменений.
- Переиспользование буферов в TUN `Read`/`Write` (stack sizes, sync.Pool с headroom).
- Тесты на backpressure, ошибку записи, аллокации и регресс secondary.

## Контекст

- v1.4.1 добавила `primaryWriter` с `drop-on-full` (`src/internal/tunnel/writer.go`, `session.go`), чтобы затор primary не блокировал UDP.
- Для TCP это даёт bufferbloat и потери; до v1.4.1 primary писался синхронно.
- Secondary-очередь решает другую задачу (real-time UDP) и должна остаться.
- `src/internal/tun/tun.go` аллоцирует `sizes` и `padded` на каждый пакет.

## Зависимости

- `src/internal/tunnel` (dispatch/writer), `src/internal/tun` (Linux реализация), существующие `transport.StreamConn` контракты.

## Требования

- RQ-001 Primary Write ДОЛЖЕН применять backpressure: при заполненном/медленном канале вызывающий ждёт, а не теряет кадр.
- RQ-002 Порядок primary-кадров ДОЛЖЕН сохраняться.
- RQ-003 Secondary-очередь, batching, stale-drop и no-pad НЕ ДОЛЖНЫ меняться.
- RQ-004 Ошибка primary-записи ДОЛЖНА завершать сессию без зависших горутин.
- RQ-005 TUN `Read`/`Write` НЕ ДОЛЖНЫ аллоцировать новый буфер на каждый пакет после прогрева пула.

## Вне scope

- Маршрутизация KTalk/`.host` и exclude-правила.
- Primary no-pad / отключение padding в data-фазе.
- QUIC datagrams, Android, relay-специфика.
- Приоритизация TCP-потоков и fair queueing.

## Критерии приемки

### AC-001 Primary TCP не теряет кадры при медленном writer'е

- Почему это важно: скрытые дропы TCP вызывают retransmit и лаги.
- **Given** сессия с медленным primary `StreamConn` и N TCP-кадров
- **When** dispatch пишет все N кадров
- **Then** writer получает все N кадров в исходном порядке, дропов primary нет
- Evidence: unit-тест с медленным fake `StreamConn` и счётчиком кадров.

### AC-002 Secondary UDP-поведение не изменилось

- Почему это важно: фикс TCP не должен ломать real-time UDP.
- **Given** заполненная secondary-очередь и stale-кадры
- **When** идёт enqueue/чтение
- **Then** сохраняются bounded drop/stale-семантика, batching и no-pad
- Evidence: существующие тесты `TestQueueWriterDropsOnFull`, `TestSecondaryWriterDropsStaleAge`, `TestSecondaryRebindClearsQueue` зелёные.

### AC-003 TUN read/write без per-packet аллокаций

- Почему это важно: высокий pps создаёт GC-джиттер на всём трафике.
- **Given** прогретый пул буферов и fake TUN device
- **When** выполняется серия `Read`/`Write`
- **Then** `AllocsPerRun` не растёт с числом пакетов (0–1 alloc/op)
- Evidence: unit/benchmark-тест в `src/internal/tun`.

### AC-004 Ошибка primary-записи завершает сессию

- Почему это важно: иначе сессия зависает с мёртвым каналом.
- **Given** primary `StreamConn` возвращает ошибку записи
- **When** dispatch пытается отправить TCP-кадр
- **Then** сессия завершается с ошибкой, secondary-writer останавливается по контексту
- Evidence: unit-тест на завершение `tunToWS`/`Run`.

### AC-005 Прочие режимы не регрессируют

- Почему это важно: TCP-фикс не должен менять padding/no-pad/relay.
- **Given** padded primary, no-pad secondary и relay-тесты
- **When** прогоняется существующий набор
- **Then** все тесты проходят
- Evidence: `go test -race ./src/internal/tunnel/ ./src/internal/tun/ ./src/internal/transport/websocket/`.

## Допущения

- Синхронный primary может задержать постановку UDP при длительном заторе primary; это принятый компромисс ради корректного TCP, secondary-очередь остаётся.
- TUN-реализация Linux — основная цель оптимизации; другие платформы не ломаются.
- Сервер и клиент используют общий `tunnel.Session`, поэтому фикс действует на обеих сторонах.

## Критерии успеха

- SC-001 При медленном writer'е все TCP-кадры доставлены, `primary_dropped_full` не растёт.
- SC-002 На серии из 10k TUN read/write после прогрева — ≤1 alloc/op.

## Краевые случаи

- Пустой/нулевой пакет из TUN — backpressure-путь не паникует.
- Отмена контекста во время блокирующей записи — сессия завершается, буфер возвращается.
- Ошибка записи на первом кадре — сессия завершается без паники.
- Secondary не привязан — UDP идёт по primary, как раньше.

## Открытые вопросы

- none
