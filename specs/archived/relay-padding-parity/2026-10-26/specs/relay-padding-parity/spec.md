# Relay: паритет padding и unpadded-переговоры

## Scope Snapshot

- In scope: relay в режимах bridge и terminator применяет те же правила padding, что client/server, и участвует в `X-KVN-NoPad`-переговорах на обоих плечах.
- Out of scope: изменение протокола фрейминга, QUIC-обфускация и поведение client/server вне их переиспользования relay.

## Цель

Оператор relay получает одинаковое с прямым подключением поведение padding: конфиг трактуется без сюрпризов, а вторичный real-time UDP-канал может идти без padding на всём пути клиент → relay → upstream. Успех виден по подтверждённому `X-KVN-NoPad` на каждом плече и по отсутствию ошибок фрейминга в смешанных конфигурациях.

## Основной сценарий

1. Клиент с `obfuscation.padding.enabled: true` и `realtime_unpadded: true` открывает вторичный канал через relay (bridge или terminator).
2. Relay принимает запрос `X-KVN-NoPad`, проверяет собственную конфигурацию и, если upstream тоже подтверждает unpadded, снимает padding на обоих плечах.
3. Вторичный UDP-трафик проходит без 4-байтового padding-конверта и случайного хвоста; первичный канал продолжает работать в режиме, который выбрали стороны.
4. Если любая сторона не подтверждает unpadded, оба плеча остаются padded, соединение не рвётся и не возникает рассинхрона фрейминга.

## User Stories

- P1: как оператор relay, я хочу, чтобы relay не включал padding из-за одного `obfuscation.enabled`, иначе клиенты без padding ломаются на handshake.
- P2: как клиент с real-time UDP, я хочу получить unpadded-вторичный канал и через relay, чтобы не платить padding-оверхед на каждом пакете.
- P3: как оператор со смешанными версиями узлов, я хочу автоматический fallback в padded-режим без простоя.

## MVP Slice

- `AC-001` + `AC-002`: terminator перестаёт трактовать padding иначе, чем client/server, и подтверждает unpadded вторичному клиенту. Этого достаточно для independently demonstrable улучшения на плече клиент↔relay.

## First Deployable Outcome

- Terminator можно задеплоить отдельно: клиентские secondary-подключения получают подтверждённый no-pad, primary-трафик и старые клиенты работают как раньше.
- Bridge-часть поставляется тем же изменением, но проверяется отдельным integration-сценарием на оба плеча.

## Scope

- Вычисление `PaddingEnabled`/`PaddingSize` для relay в обоих режимах по тем же правилам, что у client/server.
- Client-facing WS accept в terminator: участие в `X-KVN-NoPad`.
- Upstream WS dial в terminator и bridge: участие в `X-KVN-NoPad`.
- Bridge: согласованное состояние padding на плече клиент↔relay и relay↔upstream в пределах одного проксируемого соединения.
- Логи результата переговоров на каждом плече.
- Документация relay (ru/en) о поведении padding и unpadded-переговоров.

## Контекст

- `transport/websocket` уже реализует `RequestNoPad`/`AllowNoPad` и заголовок `X-KVN-NoPad`; client и server уже используют его для вторичного канала.
- Terminator принимает клиентов сам и декодирует/перекодирует кадры, поэтому padding на клиентском и upstream-плечах независим.
- Terminator держит **одно общее** upstream-подключение; unpadded на нём действует на весь агрегированный трафик, включая primary.
- Bridge пересылает WS-сообщения как есть, поэтому при рассинхроне padding на плечах он не сможет исправить конверт на лету.
- Текущий relay включает padding при `obfuscation.enabled: true` без `padding.enabled`, что расходится с client/server.

## Зависимости

- `src/internal/transport/websocket` (`WSConfig.RequestNoPad`, `AllowNoPad`, `SetPadding`).
- `src/internal/config` (`ObfuscationCfg`, `PaddingCfg.RealtimeUnpaddedEnabled`).
- Совместимый upstream server, поддерживающий `X-KVN-NoPad`, для собственно unpadded-плеча relay↔upstream.

## Требования

- RQ-001 Relay включает padding только при `obfuscation.padding.enabled: true`; `obfuscation.enabled` сам по себе padding не включает. `padding.size` по умолчанию равен 512.
- RQ-002 Terminator подтверждает `X-KVN-NoPad` клиенту только когда padding включён и `realtime_unpadded` не выключен; иначе клиент остаётся в padded-режиме.
- RQ-003 Terminator запрашивает unpadded у upstream согласно своей конфигурации; решение принимает ответ upstream, отказ сохраняет padding без разрыва обслуживания.
- RQ-004 Bridge согласует padding на обоих плечах одного соединения: клиент получает подтверждение no-pad только если upstream тоже подтвердил; смешанное состояние не допускается.
- RQ-005 Любая комбинация «старый клиент / старый upstream / `realtime_unpadded: false` / padding выключен» остаётся работоспособной: handshake проходит, данные доставляются, ошибок вида `padding frame too short` нет.
- RQ-006 Relay логирует результат no-pad-переговоров на каждом плече (подтверждён/отклонён, сторона, peer) без раскрытия токенов.

## Вне scope

- Изменение wire-формата padding или заголовка `X-KVN-NoPad`.
- Padding для QUIC (в QUIC используется обфускация, не padding).
- Новые конфигурационные ключи или изменение значения по умолчанию `realtime_unpadded`.
- Паритет прочих transport-настроек relay (multiplex, batching, MTU, uTLS) — только padding.
- Изменение поведения client/server, кроме переиспользования их правил relay-кодом.

## Критерии приемки

### AC-001 Конфиг padding совпадает с client/server

- Почему это важно: неверная трактовка конфига ломает handshake у клиентов с другим padding-режимом.
- **Given** relay-конфиг содержит `obfuscation: {enabled: true}` без блока `padding`, а клиент настроен без padding
- **When** клиент подключается через relay в любом режиме
- **Then** WS-upgrade и обмен данными проходят, relay работает в unpadded-режиме, и в логах нет ошибки `padding frame too short`
- Evidence: integration-тест с таким конфигом и клиентом без padding, доставка данных.

### AC-002 Terminator подтверждает unpadded вторичному клиенту

- Почему это важно: именно это убирает padding-оверхед с real-time UDP через terminator.
- **Given** relay terminator с `padding.enabled: true`, `realtime_unpadded: true` и клиент с теми же настройками
- **When** клиент открывает secondary-канал с заголовком `X-KVN-NoPad: 1`
- **Then** upgrade-ответ содержит `X-KVN-NoPad: 1`, обе стороны фиксируют `no-pad framing negotiated`, secondary UDP-трафик доставляется без padding-конверта
- Evidence: заголовок ответа, лог-строки client и terminator, прохождение UDP-пакетов в integration-тесте.

### AC-003 Terminator согласует unpadded с upstream

- Почему это важно: без второго плеча padding остаётся в relay↔upstream и обесценивает выигрыш.
- **Given** terminator с `realtime_unpadded: true`; upstream server поддерживает `AllowNoPad`
- **When** terminator устанавливает upstream WS-соединение
- **Then** upstream подтверждает `X-KVN-NoPad`, соединение relay↔upstream работает без padding, и агрегированный трафик доставляется
- Evidence: лог upstream-сервера о подтверждении, лог terminator о результате, успешная передача данных.

### AC-004 Отказ upstream сохраняет padded-режим terminator

- Почему это важно: отказ не должен ломать обслуживание клиентов.
- **Given** terminator с `realtime_unpadded: true`, а upstream server не подтверждает no-pad
- **When** устанавливается upstream-соединение и клиенты шлют трафик
- **Then** relay↔upstream остаётся padded, ошибок фрейминга нет, upstream-трафик продолжает доставляться
- Evidence: отсутствие подтверждающего заголовка, лог «denied/kept padded», успешный обмен данными.

### AC-005 Bridge не создаёт смешанное состояние

- Почему это важно: bridge пересылает сообщения как есть, поэтому рассинхрон конверта необратим.
- **Given** bridge с `padding.enabled: true` и `realtime_unpadded: true`; клиент запрашивает no-pad
- **When** upstream подтверждает `X-KVN-NoPad`
- **Then** и клиент↔bridge, и bridge↔upstream работают unpadded, клиент видит подтверждение, данные проходят
- **When** upstream не подтверждает
- **Then** клиент **не** получает подтверждение, оба плеча остаются padded, данные проходят
- Evidence: проверка заголовка на клиенте и на upstream, логи двух плеч, сквозной обмен в обоих сценариях.

### AC-006 Клиент без запроса no-pad остаётся padded

- Почему это важно: primary-канал не должен терять маскировку из-за настройки, предназначенной вторичному каналу.
- **Given** relay bridge/terminator с включённым padding и `realtime_unpadded: true`
- **When** клиент с padding подключается без заголовка `X-KVN-NoPad`
- **Then** клиентское плечо не переходит в unpadded само по себе, padding остаётся, данные проходят
- Evidence: отсутствие `no-pad framing negotiated` для этого соединения и успешный обмен.

### AC-007 `padding.size` применяется на relay-плечах

- Почему это важно: размер выравнивания должен совпадать у сторон, иначе длины не сходятся.
- **Given** relay с `padding.size: 1024`, padding включён, no-pad не согласован
- **When** через relay идёт WS-трафик
- **Then** padding-сообщения на клиентском и upstream-плечах выровнены до 1024; при отсутствии `size` используется 512
- Evidence: размеры WS-сообщений, снятые в integration-тесте на обоих плечах.

### AC-008 Результат переговоров наблюдаем

- Почему это важно: без логов невозможно отличить «no-pad не поддержан» от «no-pad не запрошен».
- **Given** relay обрабатывает WS-соединение с любым исходом no-pad-переговоров
- **When** переговоры завершаются
- **Then** в логе есть запись с исходом, стороной (client-facing/upstream) и peer; токены и ключи не логируются
- Evidence: пример лог-строки для подтверждённого и отклонённого исхода.

## Допущения

- `X-KVN-NoPad` и `SetPadding` в `transport/websocket` считаются стабильным контрактом; менять их не требуется.
- `realtime_unpadded` в relay-конфиге относится ко всем WS-плечам relay, включая общее upstream-соединение terminator.
- Оператор согласует `obfuscation.padding` между клиентом, relay и upstream server; рассинхрон конфигов остаётся ошибкой конфигурации, как и в прямом подключении.
- Padding применяется только к WS; QUIC-транспорт не участвует в этой фиче.
- Один upstream terminator общий: unpadded на нём — осознанный компромисс, зафиксированный этим требованием.

## Критерии успеха

- SC-001 Матрица integration-сценариев (padded, клиент без padding, no-pad подтверждён, no-pad отклонён, `realtime_unpadded: false`) проходит без ошибок фрейминга.
- SC-002 В подтверждённом no-pad-режиме размер WS-сообщения вторичного канала равен размеру кадра без 4-байтового конверта и padding-хвоста.

## Краевые случаи

- Клиент с padding подключается к relay с выключенным padding: handshake по-прежнему завершается ошибкой, как и при прямом подключении; фича не маскирует ошибку конфигурации.
- Upstream недоступен в bridge: соединение клиента отклоняется как сейчас; повторный dial заново согласует padding.
- Upstream переподключается в terminator: новый dial заново проходит no-pad-переговоры; уже принятые клиентские соединения не меняют свой клиентский padding.
- `realtime_unpadded: false` или отсутствие `padding.enabled`: заголовок не отправляется и не подтверждается.
- Старый клиент без `X-KVN-NoPad` через новый relay: padding сохраняется, ошибок нет.

## Открытые вопросы

- none
