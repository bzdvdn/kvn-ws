# Performance & Polish — Задачи

## Phase Contract

Inputs: plan.md, plan.digest.md, spec.md
Outputs: 4 фазы, 12 задач, покрытие всех 8 AC
Stop if: —

## Implementation Context

- **Цель MVP:** sync.Pool + TCP_NODELAY + batch writes + load testing gate (AC-001, AC-002, AC-003, AC-008)
- **Инварианты:** все изменения в infrastructure/transport/config; domain не затронут; feature-флаги для compression/multiplex
- **Контракты/протокол:** handshake — опциональное поле MTU (uint16, default 1500); permessage-deflate через `EnableCompression`/`SetCompressionLevel` gorilla/websocket; multiplex через WebSocket subprotocol
- **Границы scope:** не трогаем session, routing, crypto, auth, nat, dns, tun
- **Proof signals:** benchstat (allocations ≥80%), unit tests, gatetest (throughput ≥80%, latency ≤15%)
- **References:** DEC-001—DEC-008, AC-001—AC-008

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/transport/framing/framing.go | T2.1, T3.2 |
| src/internal/transport/websocket/websocket.go | T1.1, T2.2, T2.3, T3.3, T3.4 |
| src/internal/protocol/handshake/ | T3.1 |
| src/internal/config/client.go | T1.1 |
| src/internal/config/server.go | T1.1 |
| src/cmd/client/main.go | T1.1 |
| src/cmd/server/main.go | T1.1 |
| src/cmd/gatetest/main.go | T2.4 |
| configs/loadtest.yaml | T1.2, T2.4 |

## Фаза 1: Основа

Цель: подготовить конфиги и bootstrapping surfaces.

- [x] T1.1 Добавить поля `Compression`, `Multiplex`, `MTU` в `config.ClientConfig` и `config.ServerConfig`, пробросить через конструкторы в `cmd/client/main.go` и `cmd/server/main.go`. Все поля опциональны с zero-defaults. Touches: src/internal/config/client.go, src/internal/config/server.go, src/cmd/client/main.go, src/cmd/server/main.go, src/internal/transport/websocket/websocket.go
- [x] T1.2 Создать `configs/loadtest.yaml` — шаблон конфига для gatetest: session_count, duration, target_host, throughput_threshold, latency_threshold. Touches: configs/loadtest.yaml

## Фаза 2: MVP Slice

Цель: sync.Pool + TCP_NODELAY + batch writes + load testing gate.

- [x] T2.1 Реализовать `sync.Pool` для буферов в `Encode()`/`Decode()`: `pool.Get()` на encode, `pool.Put()` после `WriteMessage`; `pool.Get()` на decode, `pool.Put()` после обработки payload. Доказательство: `go test -bench=. -benchmem` — снижение аллокаций ≥80%. Touches: src/internal/transport/framing/framing.go, src/internal/transport/framing/framing_test.go
- [x] T2.2 Реализовать `TCP_NODELAY` после WS upgrade: `tcpConn, ok := ws.UnderlyingConn().(*net.TCPConn)` → `tcpConn.SetNoDelay(true)`. Для client — через `Dialer.NetDial`; для server — после `Upgrader.Upgrade`. Доказательство: юнит-тест проверяет `NoDelay() == true`. Touches: src/internal/transport/websocket/websocket.go, src/internal/transport/websocket/websocket_test.go
- [x] T2.3 Реализовать batch writes: `BatchWriter` с внутренним буфером + `flush()` по timer (max 5ms) или по заполнению (MTU-size). Фреймы coalesce в одно `WriteMessage`. Доказательство: интеграционный тест проверяет coalescing. Touches: src/internal/transport/websocket/websocket.go, src/internal/transport/websocket/websocket_test.go
- [x] T2.4 Реализовать gatetest: бинарник, который открывает 1000+ WS-сессий, гоняет трафик, замеряет throughput и latency, сравнивает с порогами из `configs/loadtest.yaml`. Доказательство: `go run ./src/cmd/gatetest/ --mode loadtest --config configs/loadtest.yaml` выводит pass/fail. Touches: src/cmd/gatetest/main.go, configs/loadtest.yaml

## Фаза 3: Основная реализация

Цель: MTU/PMTU, compression, multiplex.

- [x] T3.1 Реализовать MTU negotiation в handshake: добавить поле MTU в ClientHello и ServerHello (uint16, default 1500). После handshake — `min(client, server)`. Доказательство: тест handshake проверяет согласованный MTU. Touches: src/internal/protocol/handshake/, src/internal/config/client.go, src/internal/config/server.go
- [x] T3.2 Реализовать PMTU strategy: при payload > MTU разбивать на сегменты (MTU-sized chunks) в `Encode()`. На получателе — сборка по sequence в `Decode()`. Доказательство: тест отправляет >MTU и проверяет количество сегментов. Touches: src/internal/transport/framing/framing.go, src/internal/transport/framing/framing_test.go
- [x] T3.3 Реализовать permessage-deflate compression: `Dialer.EnableCompression = true` / `Upgrader.EnableCompression = true`; `conn.SetCompressionLevel(flate.DefaultCompression)` если `Config.Compression`. Доказательство: тест сжимаемых данных проверяет compressed < original. Touches: src/internal/transport/websocket/websocket.go, src/internal/config/client.go, src/internal/config/server.go, src/internal/transport/websocket/websocket_test.go
- [x] T3.4 Реализовать multiplex через WebSocket subprotocol: добавить subprotocol name (напр. "kvn-ws-mux"), routing по channel ID внутри фреймов. Feature-флаг `Config.Multiplex`. Доказательство: интеграционный тест с двумя независимыми каналами. Touches: src/internal/transport/websocket/websocket.go, src/internal/config/client.go, src/internal/config/server.go, src/internal/transport/websocket/websocket_test.go

## Фаза 4: Проверка

Цель: доказать, что все AC закрыты, замерить регрессию.

- [x] T4.1 Добавить бенчмарки для hot path: `BenchmarkEncode`, `BenchmarkDecode`, `BenchmarkEncodeDecode` (с и без sync.Pool). Доказательство: `go test -bench=. -benchmem -count=5 | benchstat` показывает снижение аллокаций ≥80%. Touches: src/internal/transport/framing/framing_test.go
- [x] T4.2 Интегрировать load testing gate в CI: добавить шаг в `.github/workflows/ci.yml` для запуска gatetest с `configs/loadtest.yaml`. Доказательство: CI pass с метриками throughput/latency. Touches: .github/workflows/ci.yml, configs/loadtest.yaml
- [x] T4.3 Прогнать `go test -race ./...` и `golangci-lint run ./...`, исправить найденные проблемы. Touches: все затронутые файлы

## Покрытие критериев приемки

- AC-001 -> T2.1, T4.1
- AC-002 -> T2.2
- AC-003 -> T2.3
- AC-004 -> T3.1
- AC-005 -> T3.2
- AC-006 -> T3.3
- AC-007 -> T3.4
- AC-008 -> T2.4, T4.2
- Race/lint -> T4.3

## Заметки

- T1.1/T1.2 — можно параллелить
- T2.1/T2.2/T2.3 — можно параллелить (разные surfaces)
- T2.4 — зависит от configs/loadtest.yaml (T1.2)
- T3.1 — зависит от config MTU (T1.1)
- T3.2 — зависит от MTU negotiation (T3.1)
- T3.3 — зависит от config Compression (T1.1)
- T3.4 — зависит от config Multiplex (T1.1)
- T4.1/T4.2 — можно параллелить после T2.x и T3.x
- T4.3 — финальная, после всех задач
