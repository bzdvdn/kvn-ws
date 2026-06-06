# Архитектурный рефакторинг kvn-ws — План

## Phase Contract

Inputs: spec, inspect (pass), minimal repo-контекст (transport/quic, tunnel, proxy, bootstrap/client, tun, config, webui).
Outputs: plan.md, data-model.md.
Stop if: spec слишком расплывчата — нет, spec детализирована.

## Цель

Реализовать 6 независимых блоков рефакторинга на ветке `feature/arch-refactoring`. Каждый блок обратно совместим на уровне протокола. Валидация: `go test -race ./...` + `golangci-lint run ./...` до и после.

## MVP Slice

QUIC OOM fix (RQ-001) + консолидация StreamConn (RQ-002). Покрывает AC-001, AC-002, AC-003. Время реализации ~2-3 задачи.

## First Validation Path

1. `go build ./...` — компиляция без ошибок.
2. `go test -race ./internal/transport/quic/...` — тесты на лимит сообщений.
3. `grep -r "type StreamConn interface" internal/ | wc -l` == 1.

## Scope

- QUIC ReadMessage / ObfuscatedQUICConn — лимит msgLen
- StreamConn — единое объявление в `internal/transport/`
- dialStream — общая функция подключения
- wsToTun — декомпозиция обработчика фреймов
- Config — MaxMessageSize, wsTunnelTimeout, defaultProxyConcurrency в ClientConfig
- Web UI — поле max_message_size в форме Advanced
- tun — netlink вместо exec.Command("ip")
- Без изменений: протокол фреймов, server bootstrap, Admin API (кроме config), QUIC obfuscation

## Implementation Surfaces

| Surface | Изменения | Статус |
|---|---|---|
| `internal/transport/quic/conn.go` | лимит msgLen, чтение MaxMessageSize из config | существующий |
| `internal/transport/quic/obfuscated.go` | лимит msgLen после XOR | существующий |
| `internal/transport/transport.go` | новый файл: StreamConn interface | новый |
| `internal/tunnel/stream.go` | удалить дублирующий interface, импорт из transport | существующий |
| `internal/proxy/stream.go` | удалить дублирующий interface, импорт из transport | существующий |
| `internal/bootstrap/client/tun.go` | вызов dialStream() вместо дублирования | существующий |
| `internal/bootstrap/client/proxy.go` | вызов dialStream() вместо дублирования | существующий |
| `internal/bootstrap/client/helpers.go` | консолидация clientTLSConfig, parseBackoff, paddingSizeOrDefault | существующий |
| `internal/bootstrap/client/dial.go` | новая общая dialStream() | новый |
| `internal/tunnel/session.go` | декомпозиция wsToTun | существующий |
| `internal/tun/tun.go` | netlink вместо exec.Command | существующий |
| `internal/config/client.go` | поля MaxMessageSize, TunnelTimeout, ProxyConcurrency + defaults | существующий |
| `internal/webui/handler_config.go` | defaultConfig() — default MaxMessageSize | существующий |
| `internal/webui/frontend/src/App.tsx` | поле max_message_size в форме | существующий |

## Bootstrapping Surfaces

`internal/transport/transport.go` — должен быть создан до изменений tunnel/stream.go и proxy/stream.go.

## Влияние на архитектуру

- `internal/transport/` получает единый интерфейс StreamConn — все потребители импортируют оттуда.
- `internal/config/` расширяется новыми полями — без ломающих изменений (опциональные поля с defaults).
- `internal/quic/` добавляет зависимость от `internal/config/` (через параметры, не глобально).
- `internal/tun/` добавляет зависимость от `github.com/vishvananda/netlink` — новая внешняя зависимость.

## Acceptance Approach

- **AC-001** (QUIC OOM): unit test с msgLen > MaxMessageSize; проверка закрытия соединения. Surface: `conn.go:ReadMessage`.
- **AC-002** (Obfuscated OOM): unit test. Surface: `obfuscated.go:ReadMessage`.
- **AC-003** (StreamConn единый): grep-проверка. Surfaces: `transport.go`, `stream.go` (tunnel + proxy).
- **AC-004** (dialStream): grep по tun.go + proxy.go — отсутствие дублированных блоков. Surface: `dial.go`.
- **AC-005** (wsToTun): unit test + `defer f.Release()` в каждом обработчике. Surface: `session.go`.
- **AC-006** (magic numbers): integration test с кастомным конфигом. Surfaces: `config/client.go`, `handler_config.go`.
- **AC-007** (netlink): grep отсутствия exec.Command; test проход. Surface: `tun.go`.

## Данные и контракты

- ClientConfig расширяется опциональными полями — обратная совместимость YAML сохранена (старые конфиги работают, берут defaults).
- StreamConn interface — идентичная сигнатура, код всех потребителей компилируется без изменений.
- dialStream() — новая внутренняя функция, внешний API не затронут.
- netlink — полная замена внутренней имплементации, наблюдаемое поведение идентично.
- См. `data-model.md`.

## Стратегия реализации

### DEC-001 MaxMessageSize в ClientConfig

- Why: конфигурируемый лимит без хардкода; оператор может менять без пересборки.
- Tradeoff: дополнительное поле в структуре + чтение из конфига на каждый ReadMessage. Чтение — один раз при создании соединения.
- Affects: `config/client.go`, `transport/quic/conn.go`, `transport/quic/obfuscated.go`.
- Validation: тест с msgLen = MaxMessageSize + 1 возвращает ошибку.

### DEC-002 Единый StreamConn в internal/transport

- Why: один контракт для всех транспортов. При добавлении HTTP/3 — достаточно реализовать интерфейс.
- Tradeoff: единый пакет для интерфейса — транспортные пакеты должны импортировать `internal/transport`. Нет циклических зависимостей.
- Affects: `transport/transport.go` (новый), `tunnel/stream.go` (удалить interface), `proxy/stream.go` (удалить interface).
- Validation: `grep -r "type StreamConn interface" internal/ | wc -l` == 1.

### DEC-003 dialStream() — общая функция подключения

- Why: устранение ~100 строк дублирования tun.go + proxy.go. Единая точка изменений при добавлении нового транспорта.
- Tradeoff: dialStream() принимает `*config.ClientConfig` — небольшая связанность bootstrap с config (уже есть).
- Affects: `bootstrap/client/dial.go` (новый), `bootstrap/client/tun.go`, `bootstrap/client/proxy.go`.
- Validation: grep показывает отсутствие `if transport == "quic"` в tun.go и proxy.go.

### DEC-004 wsToTun — короткий dispatcher + handler methods

- Why: монолитная wsToTun с ручным Release() — risk при добавлении новых типов фреймов.
- Tradeoff: оставить минимальный switch по FrameType как dispatcher, каждый case вызывает отдельный метод. Это чуть больше кода, но каждый метод тестируем изолированно.
- Affects: `tunnel/session.go`.
- Validation: `defer f.Release()` присутствует в каждом handler; `go test -race ./internal/tunnel/` проходит.

### DEC-005 netlink вместо exec.Command("ip")

- Why: устранение fork+exec на каждое изменение маршрута; отвязка от iproute2.
- Tradeoff: новая зависимость `github.com/vishvananda/netlink`; netlink API требует прав (CAP_NET_ADMIN) — те же права что и для ip.
- Affects: `tun/tun.go`.
- Validation: grep не находит `exec.Command.*"ip"`; `go test -race ./internal/tun/` проходит.

### DEC-006 Магические числа: config vs const

- Why: wsTunnelTimeout, defaultProxyConcurrency, MaxMessageSize — должны быть в конфиге (оператор меняет). CIDR masks, read limit — именованные константы в пакетах (не меняются без пересборки).
- Tradeoff: разделение по природе параметра (runtime config vs compile-time tuning).
- Affects: `config/client.go`, `tunnel/session.go`, `proxy/listener.go`.
- Validation: grep не находит оригинальные константы; значения читаются из config.

## Incremental Delivery

### MVP (Первая ценность)

QUIC OOM fix + StreamConn interface. Задачи:
1. MaxMessageSize config field + default.
2. `conn.go` и `obfuscated.go` — проверка msgLen.
3. `transport/transport.go` + удаление дублирующих interface.
- AC: AC-001, AC-002, AC-003.
- Validation: `go test -race ./internal/transport/quic/...` + grep.

### Итеративное расширение 1 — dialStream + magic numbers

Задачи:
4. `dial.go` — реализация dialStream.
5. tun.go + proxy.go — переключение на dialStream.
6. Config поля для timeout/concurrency. Web UI поле MaxMessageSize.
- AC: AC-004, AC-006.
- Validation: `go test -race ./internal/bootstrap/client/...`.

### Итеративное расширение 2 — wsToTun рефакторинг

Задача 7: декомпозиция wsToTun.
- AC: AC-005.
- Validation: `go test -race ./internal/tunnel/...`.

### Итеративное расширение 3 — netlink migration

Задача 8: замена exec.Command на netlink.
- AC: AC-007.
- Validation: `go test -race ./internal/tun/...`.

## Порядок реализации

1. **MVP (1-3)** — QUIC OOM и StreamConn. Безопасно и критично. Независимо.
2. **dialStream + magic numbers (4-6)** — требует StreamConn (готов). Web UI поле можно параллельно с 5.
3. **wsToTun (7)** — независим от всего, можно параллелить с 4-6.
4. **netlink (8)** — независим, но самый рискованный по регрессии. Выполнять после всех остальных.

Параллельно можно: 3 (StreamConn), 7 (wsToTun), 8 (netlink) — если ресурсы позволяют.

## Риски

- **Риск 1: netlink регрессия** — TUN-маршруты перестанут работать на системах без netlink или с другой версией libnl.
  Mitigation: сохранение exec.Command как fallback под флагом; coverage тестами на добавление/удаление маршрутов; тестирование на реальном TUN до merge.
- **Риск 2: wsToTun утечка фреймов** — неверный defer Release() при早ем return или панике.
  Mitigation: явный defer сразу после проверки типа; recover в dispatcher; race-тесты.
- **Риск 3: Совместимость ObfuscatedQUICConn** — старые клиенты с oversized фреймами.
  Mitigation: ожидаемо и желаемо (AC-002 spec). Лимит в 10MB — щадящий для существующего трафика.

## Rollout and compatibility

- Все изменения internal-only, без изменения публичного API.
- ClientConfig — новые поля опциональны; defaults совместимы.
- netlink migration — влияет на runtime, но идентичное поведение при успехе.
- Специальных rollout-действий не требуется. Достаточно `go test -race ./...` + `golangci-lint run ./...`.

## Проверка

| Шаг | Проверка | AC/DEC |
|---|---|---|
| unit: quic conn | msgLen > limit → error | AC-001 / DEC-001 |
| unit: quic obfuscated | oversize → error | AC-002 |
| grep: StreamConn | ровно 1 объявление | AC-003 / DEC-002 |
| grep: dial | нет дублированного dial | AC-004 / DEC-003 |
| unit: tunnel session | wsToTun dispatcher + defer Release | AC-005 / DEC-004 |
| integration: config | кастомные значения применяются | AC-006 / DEC-006 |
| unit: tun | netlink вместо exec.Command | AC-007 / DEC-005 |
| lint | golangci-lint run ./... без ошибок | SC-002 |
| race | go test -race ./... | SC-001 |

## Соответствие конституции

- нет конфликтов. Все изменения в `internal/`, код на Go, без глобального мутабельного состояния. Trace-маркеры `@sk-task` / `@sk-test` обязательны. Docker-сборка не ломается.
