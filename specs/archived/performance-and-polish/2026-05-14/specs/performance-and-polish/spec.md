# Performance & Polish — Оптимизация производительности WebSocket-туннеля

## Scope Snapshot

- In scope: Оптимизация производительности WebSocket-туннеля kvn-ws: buffer pooling, batch writes, TCP_NODELAY, MTU negotiation + PMTU strategy, payload compression, опциональное мультиплексирование, нагрузочное тестирование с gate criteria.
- Out of scope: Изменение протокола рукопожатия, архитектуры сессий, системы маршрутизации, алгоритмов шифрования, логики аутентификации.

## Цель

Разработчики и пользователи kvn-ws получают высокопроизводительный VPN-туннель через WebSocket с пропускной способностью ≥80% от raw TCP и накладными расходами latency ≤15% при 1000+ одновременных сессий. Успех подтверждается нагрузочным тестом.

## Основной сценарий

1. **Hot path**: encode/decode фреймов используют `sync.Pool` для переиспользования буферов, снижая GC pressure.
2. **Socket tuning**: WebSocket-соединения создаются с `TCP_NODELAY` + batch writes для минимизации latency.
3. **MTU awareness**: клиент и сервер согласовывают MTU, PMTU strategy предотвращает IP-фрагментацию.
4. **Compression**: опционально payload сжимается через permessage-deflate (WebSocket compression extension) для снижения bandwidth.
5. **Multiplex**: опционально несколько логических потоков поверх одного WS-соединения.
6. **Gate**: нагрузочный тест на 1000+ сессий подтверждает throughput ≥80% и latency overhead ≤15%.

## User Stories

- P1: Как оператор VPN-сервиса, я хочу чтобы туннель работал с производительностью, близкой к raw TCP, чтобы пользователи не замечали замедления.
- P2: Как разработчик, я хочу видеть benchmark-регрессию на hot path, чтобы не допустить деградации производительности.

## MVP Slice

Buffer pooling + TCP_NODELAY + batch writes + load testing gate. Закрывает AC-001, AC-002, AC-003, AC-008.

## First Deployable Outcome

Собранный бинарник с включёнными оптимизациями (sync.Pool, TCP_NODELAY, batch writes), проходящий gate test с throughput ≥80% и latency overhead ≤15%.

## Scope

- `sync.Pool` для буферов encode/decode фреймов
- `TCP_NODELAY` на WebSocket-соединениях (client + server)
- Batch writes через `WriteMessage` с coalescing
- MTU negotiation между client/server при handshake
- PMTU strategy (отказ от фрагментации, fallback на меньший MTU)
- Payload compression (permessage-deflate)
- Multiplex channels (опционально, feature-флаг в конфиге, через WebSocket subprotocol)
- Нагрузочное тестирование с 1000+ sessions и gate criteria
- Perf-бенчмарки для hot path (encode/decode, read/write)

## Контекст

- Репозиторий: `kvn-ws` (github.com/bzdvdn/kvn-ws), Go 1.22+, gorilla/websocket v1.5.3
- Текущее состояние: фреймы аллоцируют новые буферы на каждый encode/decode, TCP_NODELAY не выставлен, compression/MTU не реализованы
- Gate-критерии проверяются скриптом после имплементации
- Архитектура DDD + Clean Architecture — изменения не должны нарушать domain/infrastructure границы

## Требования

- RQ-001 Система ДОЛЖНА переиспользовать буферы encode/decode через `sync.Pool` для снижения количества аллокаций на hot path.
- RQ-002 Система ДОЛЖНА устанавливать `TCP_NODELAY` на всех WebSocket-соединениях при старте.
- RQ-003 Система ДОЛЖНА поддерживать batch writes — coalescing нескольких мелких фреймов в одно `WriteMessage`.
- RQ-004 Система ДОЛЖНА согласовывать MTU между клиентом и сервером в рамках handshake.
- RQ-005 Система ДОЛЖНА применять PMTU strategy — при превышении MTU разбивать фрейм или падать на меньший MTU без IP-фрагментации.
- RQ-006 Система ДОЛЖНА опционально сжимать payload через permessage-deflate (WebSocket compression extension) при включении в конфиге для совместимости с прокси.
- RQ-007 Система ДОЛЖНА опционально поддерживать мультиплексирование нескольких логических каналов поверх одного WS-соединения через WebSocket subprotocol.
- RQ-008 Система ДОЛЖНА проходить нагрузочный тест с 1000+ одновременных сессий: throughput ≥80% от raw TCP, latency overhead ≤15%.

## Вне scope

- Изменение протокола рукопожатия (Hello/ClientHello)
- Изменение архитектуры сессий и IP-пула
- Изменение системы маршрутизации (CIDR/DNS/ordered rules)
- Изменение алгоритмов шифрования и аутентификации
- Переход на другой транспорт (не WebSocket)
- Оптимизация TUN-устройства

## Критерии приемки

### AC-001 sync.Pool для буферов encode/decode

- Почему это важно: горячий путь фрейминга создаёт GC pressure, снижая throughput.
- **Given** настроенный `sync.Pool` с pre-allocated буферами
- **When** вызывается `Encode()` или `Decode()` на фрейме
- **Then** буфер берётся из pool и возвращается после использования
- Evidence: `go test -bench=. -benchmem` показывает снижение аллокаций на бенчмарке encode/decode

### AC-002 TCP_NODELAY на WS-соединениях

- Почему это важно: Nagle's algorithm добавляет задержку 40-200ms на каждый пакет.
- **Given** WebSocket-соединение установлено (dial или accept)
- **When** соединение переходит в active state
- **Then** `TCP_NODELAY` установлен на underlying `*net.TCPConn`
- Evidence: тест проверяет `tcpConn.NoDelay()` после upgrade

### AC-003 Batch writes

- Почему это важно: каждый вызов `WriteMessage` — syscall; coalescing снижает количество syscall.
- **Given** накоплено несколько мелких фреймов (меньше MTU)
- **When** Writer готов к отправке
- **Then** фреймы coalesced в одно `WriteMessage`
- Evidence: тест с mock conn проверяет количество вызовов WriteMessage до/после

### AC-004 MTU negotiation

- Почему это важно: фреймы больше MTU сети вызывают IP-фрагментацию или потерю пакетов.
- **Given** клиент и сервер устанавливают соединение
- **When** handshake завершён
- **Then** стороны обменялись MTU и выбрали min(клиент_MTU, сервер_MTU, путь_MTU)
- Evidence: тест handshake показывает согласованный MTU в обоих концах

### AC-005 PMTU strategy

- Почему это важно: без PMTU фреймы могут фрагментироваться на уровне IP.
- **Given** фрейм с payload превышает согласованный MTU
- **When** фрейм ставится в очередь отправки
- **Then** он разбивается на несколько MTU-сегментов или отбрасывается с fallback на меньший MTU
- Evidence: тест отправляет фрейм >MTU и проверяет количество отправленных сегментов

### AC-006 Payload compression

- Почему это важно: сжатие уменьшает bandwidth, критично для лимитированных каналов.
- **Given** компрессия включена в конфиге (client и server)
- **When** фрейм с payload отправляется/получается
- **Then** payload сжимается на отправке и разжимается на получении
- Evidence: тест отправляет сжимаемые данные и проверяет, что compressed size < original size

### AC-007 Multiplex channels (опционально)

- Почему это важно: позволяет нескольким логическим потокам (TUN, control, metrics) делить одно WS-соединение.
- **Given** multiplex включён в конфиге
- **When** создаётся второй логический канал через существующее WS-соединение
- **Then** фреймы маршратизируются по channel ID и не интерферируют
- Evidence: интеграционный тест с двумя каналами проверяет независимую отправку/приём

### AC-008 Load testing gate (1000+ sessions)

- Почему это важно: гарантирует, что оптимизации работают под реальной нагрузкой.
- **Given** нагрузочный стенд с 1000+ WS-сессий
- **When** прогоняется throughput и latency тест
- **Then** throughput ≥80% от raw TCP и latency overhead ≤15%
- Evidence: CI-шаг или скрипт выводит pass/fail и метрики

## Допущения

- Gorilla/websocket v1.5.3 поддерживает установку TCP_NODELAY через `NetDial` и `UnderlyingConn()`
- WebSocket compression extension (permessage-deflate) доступен в gorilla/websocket v1.5.3 через `EnableCompression` и `SetCompressionLevel`
- MTU не меньше 576 (минимальный IPv4 MTU)
- Нагрузочное тестирование проводится на выделенных серверах без конкуренции за ресурсы

## Критерии успеха

- SC-001 Throughput ≥80% от raw TCP при 1000+ сессий (измеряется скриптом load testing)
- SC-002 Latency overhead ≤15% относительно raw TCP (измеряется скриптом load testing)
- SC-003 Количество аллокаций на encode/decode снижено на ≥80% (измеряется benchstat)

## Краевые случаи

- MTU mismatch между клиентом и сервером — берётся наименьший
- PMTU discovery недоступен (например, блокировка ICMP) — fallback на 1500
- Компрессия несжимаемых данных (binary уже сжатые) — небольшой overhead допустим
- Multiplex с мёртвым каналом — канал закрывается без влияния на другие
- Load testing при лимитированных ресурсах стенда — тест предупреждает о недостаточных ресурсах

## Открытые вопросы

1. RESOLVED: Компрессия через permessage-deflate (WebSocket compression extension) — совместимо с прокси.
2. RESOLVED: Multiplex через WebSocket subprotocol.
3. RESOLVED: Отдельный конфигурационный файл для load testing стенда.
