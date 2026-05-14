# Tun Data Path — План

## Phase Contract

Inputs: spec.md, user answers (offset=4, single-buf, mock).
Outputs: plan, data model (no-change).
Stop if: нет — spec полная, решения приняты.

## Цель

Починить TUN I/O: Write offset, Read batch коллизия, unit-тесты без /dev/net/tun.

## MVP Slice

AC-001 (Write fix) + AC-002 (Read fix). После этого forwarding loop не падает с ошибкой, тест `go test ./src/internal/tun/` проходит.

## First Validation Path

```bash
go test -v -count=1 -run TestMockTun ./src/internal/tun/
```

## Scope

- `src/internal/tun/tun.go` — Read, Write, и возможно NewTunDevice/Open
- `src/internal/tun/tun_test.go` — новый файл с MockTunDevice
- `src/cmd/client/main.go` — tunToWS, wsToTun
- `src/cmd/server/main.go` — serverWSToTun

## Implementation Surfaces

| Surface | Тип | Почему |
|---------|-----|--------|
| `src/internal/tun/tun.go` | existing | Read/Write методы с багом |
| `src/internal/tun/tun_test.go` | new | MockTunDevice для тестов без TUN |
| `src/cmd/client/main.go` | existing | forwarding loop вызывает tun.Write с f.Payload |
| `src/cmd/server/main.go` | existing | forwarding loop вызывает dev.Write с f.Payload |

## Bootstrapping Surfaces

`tun_test.go` — создать с MockTunDevice до изменения tun.go, чтобы тестировать итеративно.

## Влияние на архитектуру

Локальное изменение в tunDevice — интерфейс TunDevice не меняется. MockTunDevice — только для тестов.

## Acceptance Approach

| AC | Подход | Поверхности | Наблюдение |
|----|--------|-------------|------------|
| AC-001 | Исправить Write offset=4 в tun.go | tun.go | TestMockTun Write возвращает без ошибки |
| AC-002 | Single-buf Read в tun.go | tun.go | TestMockTun Read возвращает пакет |
| AC-003 | Smoke test в Docker | docker compose | `docker exec client ping -c 1 10.10.0.1` |
| AC-004 | race test | tun_test.go | `go test -race ./src/internal/tun/` |

## Данные и контракты

Data model не меняется. Все изменения в tun.go и tun_test.go.

## Стратегия реализации

### DEC-001 offset=4 для tun.Device.Write()
- Why: Linux TUN ожидает 4-byte AF_INET header перед IP-пакетом. WireGuard TUN library использует offset для prepend.
- Tradeoff: Нужен буфер с headroom для каждого Write. Использовать pool/аллокацию за счёт caller.
- Affects: tun.go Write, client/main.go wsToTun, server/main.go serverWSToTun
- Validation: `dev.Write(testPacket)` не возвращает `invalid offset`

### DEC-002 Single-buf Read вместо batch
- Why: `tun.Device.Read()` с batch > 1 и одним shared buf на все слоты вызывает коллизию offset. Переходим на single-buf.
- Tradeoff: Потеря batch throughput, но для single-client это незначительно.
- Affects: tun.go Read
- Validation: `dev.Read(buf)` возвращает пакет без `invalid offset`

### DEC-003 MockTunDevice для unit-тестов
- Why: TUN требует `/dev/net/tun` + `--privileged`. Для CI и быстрой итерации нужен mock.
- Tradeoff: Не тестирует реальное TUN устройство, только логику forwarding.
- Affects: tun_test.go
- Validation: `TestMockTunReadWrite` — full cycle через mock

## Incremental Delivery

### MVP (T1.1 + T1.2)

1. tun_test.go + MockTunDevice
2. tun.go: Read single-buf + Write offset=4
3. `go test -v ./src/internal/tun/`
4. AC-001, AC-002

### Итеративное расширение

1. client/main.go, server/main.go — адаптация forwarding loops под новый буфер
2. Docker smoke test (требует docker-production spec)
3. race test

## Порядок реализации

1. T1.1: MockTunDevice + tun_test.go
2. T1.2: Исправить tun.go Read (single-buf)
3. T1.3: Исправить tun.go Write (offset=4)
4. T2.1: Адаптировать forwarding loops (client/server) для буферов с headroom
5. T2.2: Docker smoke test

## Риски

- **Риск 1:** offset=4 не является единственной причиной `invalid offset`. Возможна проблема с alignment буфера (mmap/virtio).
  Mitigation: Mock + логирование реального error от WireGuard TUN.
- **Риск 2:** Single-buf Read может не работать с пакетами > 1500 байт.
  Mitigation: Размер буфера 1600 (1500 MTU + 4 header + запас).
- **Риск 3:** `f.Payload` в forwarding может быть без headroom.
  Mitigation: prepend 4 байта в tun.Write или аллоцировать буфер с headroom в caller.

## Проверка

- `go test -v -race ./src/internal/tun/` — AC-001, AC-002, AC-004
- `go test -v -run TestDataFrameRoundTrip ./src/internal/transport/websocket/` — data path без TUN
- Docker smoke test (AC-003) — требует docker-production spec
