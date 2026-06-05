# QUIC Obfuscation

## Scope Snapshot

- In scope: обфускация QUIC stream на уровне приложения — неотличимость от случайного трафика для DPI.
- Out of scope: MASQUE, полное шифрование фреймов, обфускация WS.

## Цель

DPI не может идентифицировать kvn по сигнатуре QUIC stream, даже зная quic-go фрейминг. Достигается минимальным overhead (8 байт nonce на стрим + XOR length prefix), без влияния на пропускную способность.

## Основной сценарий

1. Клиент с `obfuscation: true` в конфиге включает обфускацию.
2. При QUIC dial — клиент генерирует случайный 8-байт nonce и отправляет его серверу как первые 8 байт стрима.
3. Все последующие length prefix'ы QUIC-сообщений XOR'ятся nonce.
4. Сервер, получив nonce, расшифровывает length prefix'ы.
5. DPI видит случайные байты — не может определить ни протокол, ни границы сообщений.

## User Stories

- P1: Администратор включает `obfuscation: true` в конфиге и туннель продолжает работать с тем же throughput — DPI не может отфильтровать трафик.
- P2: Без `obfuscation` — поведение не меняется, обратная совместимость.

## MVP Slice

`ObfuscatedQUICConn` wrapper для `QUICConn` — 8-байт nonce + XOR length prefix. Config-флаг `obfuscation`.

## First Deployable Outcome

`--obfuscation true` на клиенте и сервере → туннель работает, tcpdump на wire показывает случайные байты в QUIC stream.

## Scope

- `src/internal/transport/quic/` — `obfuscated.go`: ObfuscatedQUICConn wrapper
- `src/internal/transport/quic/dial.go`, `listen.go` — опциональный obfuscation
- `src/internal/config/` — флаг `obfuscation` (client + server config)
- `src/internal/bootstrap/client/` — передача флага в dial
- `src/internal/bootstrap/server/` — передача флага в listener

## Контекст

- QUIC stream уже имеет length-prefix framing (4-байтовый BigEndian длина + payload).
- DPI может анализировать первые байты стрима после QUIC/TLS handshake и идентифицировать приложение.
- Nonce передаётся открыто (первые 8 байт стрима) — уровень обфускации, не шифрования.
- XOR — наносекундная операция, CPU overhead < 0.1%.
- Реальный security даёт TLS 1.3 session encryption (который уже есть).
- `SessionCipher` (app-layer encryption) — ортогонален, может комбинироваться.

## Требования

- RQ-001 Система ДОЛЖНА поддерживать `obfuscation: true/false` в конфиге (client + server), по умолчанию `false`.
- RQ-002 ObfuscatedQUICConn ДОЛЖЕН генерировать случайный 8-байт nonce при установке стрима и отправлять его первыми 8 байтами.
- RQ-003 Все length prefix'ы ДОЛЖНЫ быть XOR'ены nonce (client: Write → XOR, Read → XOR; server: наоборот).
- RQ-004 Nonce НЕ ДОЛЖЕН шифроваться — сервер должен получить его до начала обмена.
- RQ-005 Обратная совместимость: без `obfuscation` — поведение как сейчас.

## Вне scope

- Obfuscation для WS.
- Смена nonce в процессе сессии (nonce фиксирован на стрим).
- Полное шифрование payload (уже покрыто SessionCipher).

## Критерии приемки

### AC-001 Obfuscated handshake и data flow

- Почему: базовый сценарий — обфускация работает
- **Given** сервер и клиент с `obfuscation: true`
- **When** клиент подключается и передаёт данные
- **Then** туннель работает, tcpdump показывает первые 8 байт случайные, length prefix'ы нечитаемы
- Evidence: tcpdump, `go test`

### AC-002 Обратная совместимость

- Почему: без флага старые клиенты работают
- **Given** сервер с `obfuscation: true` и клиент без `obfuscation`
- **When** клиент подключается
- **Then** сервер не ждёт nonce — обычный QUICConn flow
- Evidence: handshake успешен, данные передаются

### AC-003 XOR не ломает длину сообщений

- Почему: не должно быть corruption
- **Given** ObfuscatedQUICConn
- **When** WriteMessage(X) → ReadMessage
- **Then** полученные данные идентичны X
- Evidence: unit test с random payload

### AC-004 Производительность

- Почему: overhead должен быть минимален
- **Given** ObfuscatedQUICConn и обычный QUICConn
- **When** iperf через туннель
- **Then** разница throughput < 1%
- Evidence: iperf comparison

## Допущения

- Nonce не шифруется — DPI видит случайные байты, но не может отличить nonce от данных без ключа.
- quic-go v0.50 не требует изменений — обфускация на уровне нашего QUICConn.
- 8 байт nonce достаточно — при смене nonce на каждой сессии.

## Критерии успеха

- SC-001 Throughput с обфускацией не ниже 99% от без обфускации (1 core).

## Краевые случаи

- Сервер без obfuscation, клиент с obfuscation → сервер читает nonce как length prefix → ошибка → fallback или отказ.
- Nonce — 0x0000000000000000 (маловероятно, но XOR с 0 не меняет данные) — корректно.

## Открытые вопросы

- Стоит ли добавить `obfuscation_key` для deterministic nonce (вместо CSPRNG)? Полезно для debug.
- Как быть с несовпадением obfuscation между клиентом и сервером — отказ или fallback?
- Нужна ли поддержка obfuscation для WS в будущем?
