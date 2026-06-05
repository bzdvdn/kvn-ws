# QUIC Obfuscation — План

## Phase Contract

Inputs: `specs/active/quic-obfuscation/spec.md` + текущее состояние QUIC кода.
Outputs: план с задачами, реализация в `feature/quic-obfuscation`.
Stop if: нет — spec однозначна.

## Цель

Добавить `ObfuscatedQUICConn` wrapper для QUIC stream — 8-байт nonce + XOR length prefix. DPI не может идентифицировать приложение по первым байтам QUIC stream.

## MVP Slice

`ObfuscatedQUICConn` в `quic` package + флаг `obfuscation` в конфиге клиента и сервера. Закрывает AC-001, AC-002, AC-003, AC-004.

## First Validation Path

1. `go build ./...` — компиляция с ObfuscatedQUICConn
2. `go test ./src/internal/transport/quic/...` — unit-тесты XOR roundtrip
3. Сервер с `obfuscation: true` → клиент --transport quic --obfuscation → curl OK
4. Клиент без `obfuscation` → сервер с `obfuscation` → отказ (ожидаемо, AC-002)

## Scope

- `src/internal/transport/quic/obfuscated.go` — новый файл: ObfuscatedQUICConn wrapper
- `src/internal/config/client.go` — поле `Obfuscation bool`
- `src/internal/config/server.go` — поле `Obfuscation bool`
- `src/internal/bootstrap/client/tun.go` — wrap после Dial если obfuscation
- `src/internal/bootstrap/client/proxy.go` — wrap после Dial если obfuscation
- `src/internal/bootstrap/server/server.go` — wrap после Accept если obfuscation

Явная граница: WS не трогаем, handler.go не меняется.

## Implementation Surfaces

| Surface | Тип | Зачем меняется |
|---------|-----|----------------|
| `transport/quic/obfuscated.go` | new | ObfuscatedQUICConn: nonce + XOR length prefix |
| `config/client.go:ClientConfig` | existing | + `Obfuscation bool` |
| `config/server.go:ServerConfig` | existing | + `Obfuscation bool` |
| `client/tun.go` | existing | wrap QUICConn если Obfuscation |
| `client/proxy.go` | existing | wrap QUICConn если Obfuscation |
| `server/server.go` | existing | wrap QUICConn после Accept если Obfuscation |

## Bootstrapping Surfaces

`none` — все изменения локальны, нового пакета не требуется.

## Влияние на архитектуру

- `ObfuscatedQUICConn` embed'ит `*QUICConn` — реализует `tunnel.StreamConn` автоматически
- Обратная совместимость: `obfuscation: false` (default) — ничто не меняется
- Нового пакета/зависимости не добавляется (crypto/rand уже стандартный)

## Acceptance Approach

- **AC-001**: `ObfuscatedQUICConn` с nonce на wire → tcpdump показывает случайные байты → unit test + ручной тест
- **AC-002**: Сервер с obfuscation, клиент без → nonce читается как длина → отказ (ожидаемо)
- **AC-003**: XOR roundtrip → write random data → read → data совпадает (unit test)
- **AC-004**: iperf сравнение throughput Obfuscated vs plain < 1% diff (manual)

## Данные и контракты

8-байт nonce на старте стрима (client→server), все 4-байт length prefix XOR'ятся с nonce.

## Стратегия реализации

### DEC-001 Obfuscation на уровне QUICConn, не ниже

- Why: минимальные изменения — wrapper над существующим `QUICConn`, не трогаем quic-go
- Tradeoff: nonce передаётся открыто (не шифрование), но spec это разрешает
- Affects: `quic/obfuscated.go`, callers в bootstrap
- Validation: `go build ./...`, unit test

### DEC-002 Флаг в конфиге, не `--obfuscation` аргумент

- Why: единообразие с `compression`, `multiplex`
- Validation: yaml `obfuscation: true` в конфиге

### DEC-003 Server-side: wrap после Accept, не в Listen

- Why: Listen возвращает `*Listener`, Accept возвращает `*QUICConn` — wrapper применяется к конкретному соединению
- Validation: `server.go` проверяет `s.cfg.Obfuscation` и вызывает `quic.NewObfuscatedQUICConn`

## Incremental Delivery

### MVP

1. `quic/obfuscated.go` — ObfuscatedQUICConn
2. `config/client.go`, `config/server.go` — Obfuscation bool
3. `client/tun.go`, `client/proxy.go` — wrap в dial
4. `server/server.go` — wrap в accept loop
5. Unit test в `quic/obfuscated_test.go`
6. `go build ./...` + `go test ./...`

## Порядок реализации

1. `quic/obfuscated.go` + тест
2. config structs
3. bootstrap clients (tun, proxy) + server
4. `go build ./...` + `go test ./...`

## Риски

- **nonce совпадение с length prefix obfuscation?** — нет, nonce всегда 8 байт, XOR биективен
- **Производительность?** — XOR 4 байт на сообщение, CPU overhead < 0.01%
- **Server без obfuscation, client с obfuscation** — сервер прочитает nonce как length prefix → мусор → fallback/отказ. Принимаем как ожидаемое поведение.
- **crypto/rand блокировка?** — только 8 байт при старте, не критично

## Проверка

- `go build ./...` — компиляция
- `go test ./src/internal/transport/quic/... -v` — unit тест XOR roundtrip
- `go vet ./...` — без ошибок

## Соответствие конституции

нет конфликтов. Новый код только в пакете `transport/quic`, без глобального состояния.
