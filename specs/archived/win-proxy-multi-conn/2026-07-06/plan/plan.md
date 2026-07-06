# Plan: Win Proxy — Multi-Connection Architecture

## Обзор

Фича устраняет contention на `wmu` (sync.Mutex в `WSConn.WriteMessage`) путём распараллеливания WebSocket-транспорта на N независимых соединений (слотов) в proxy mode. N задаётся в конфиге (`proxy_connections`, по умолчанию 10). Каждый слот владеет отдельным `transport.StreamConn` со своим `wmu`, а входящие прокси-стримы распределяются по слотам через атомарный round-robin счётчик.

---

## Что было реализовано

### 1. `ProxyConnections` — поле конфига (`client.go:47`)

```go
ProxyConnections int `json:"proxy_connections" mapstructure:"proxy_connections"`
```

Дефолт 10 в `setClientDefaults`. Опционально переопределяется в YAML/JSON.

### 2. `proxySlot` — структура слота (`proxy.go:29-32`)

```go
type proxySlot struct {
    stream transport.StreamConn
    mgr    *proxy.Manager
}
```

Каждый слот хранит:
- `stream` — собственное WS/QUIC-соединение (полный handshake)
- `mgr` — отдельный `proxy.Manager`, который мультиплексирует стримы внутри слота через `framing.FrameTypeProxy`

### 3. `dialProxySlots(N)` — создание N соединений (`proxy.go:67-96`)

- Последовательный dial N раз через общую `dialStream()` (переиспользует логику WS/QUIC из `dial.go`)
- Каждое соединение проходит полный ClientHello↔ServerHello handshake через `doHandshake()`
- Если хотя бы один handshake не удался — закрывает уже открытые и возвращает `nil` (триггер reconnect)

### 4. `runProxySessionMulti()` — сессия с N слотами (`proxy.go:155-461`)

Основные компоненты:

**Shared resources (один на сессию):**
- `routeSet` — routing rules
- `dnsTracker` — DNS cache tracker
- `dnsSrv` — DNS proxy server (при transparent mode)

**Per-slot initialization (`proxy.go:238-244`):**
```go
for i, slot := range slots {
    slot.mgr = proxy.NewManager(slot.stream, ...)
}
```

**Round-robin listener (`proxy.go:259-372`):**
```go
var nextSlot atomic.Uint64
numSlots := uint64(len(slots))
// ...
idx := nextSlot.Add(1) % numSlots
slot := slots[idx]
```

- `proxy.NewListener` создаёт SOCKS5/HTTP CONNECT listener
- Каждое входящее соединение получает слот через `nextSlot.Add(1) % numSlots`
- Routing bypass (direct) выполняется до round-robin
- DNS-прокси привязывается к слоту 0 (`slots[0].stream`) для отправки DNS-ответов

**Read-loops (`proxy.go:401-407`):**
```go
for _, slot := range slots {
    slot := slot
    eg.Go(func() error {
        return proxyReadLoop(gctx, slot.stream, slot.mgr, c)
    })
}
```

Одна read-loop горутина на слот. Каждая читает фреймы со своего `stream` и маршрутизирует в свой `Manager.HandleIncomingFrame`. Если один read-loop упадёт — `errgroup` завершит все, триггеря полный reconnect.

**QUIC keepalive (`proxy.go:409-442`):**
- Только на слоте 0 (одного достаточно для поддержания QUIC-соединения)
- PING-фрейм каждые 25s

**Graceful shutdown:**
- `defer` закрывает все N streams при выходе из `runProxySessionMulti`
- `errgroup` ожидает завершения всех read-loop и accept-loop

### 5. Reconnect — закрытие всех N слотов (`proxy.go:35-65`)

При ошибке любого соединения (падение read-loop, keepalive, listener):
- Все N слотов закрываются
- `runProxyMode` входит в backoff-цикл
- `dialProxySlots(N)` создаёт N новых соединений

### 6. TUN mode — без изменений (`client.go:269-285`)

TUN mode вызывает `reconnectLoop()` → `dialStream()` (один вызов) → `runSession()`. Multi-conn код (`dialProxySlots`, `runProxySessionMulti`) не используется. AC-004 подтверждён.

---

## Затронутые файлы

| Файл | Изменения |
|---|---|
| `src/internal/config/client.go` | Добавлено `ProxyConnections int` (`proxy_connections`), дефолт 10 |
| `src/internal/bootstrap/client/proxy.go` | `proxySlot`, `dialProxySlots(N)`, `doHandshake`, `runProxySessionMulti`, `proxyReadLoop` |
| `src/internal/bootstrap/client/dial.go` | Без изменений (уже существовала общая `dialStream`) |
| `src/internal/bootstrap/client/client.go` | `Run()` вызывает `runProxyMode` для proxy mode, `reconnectLoop` для TUN |
| `src/internal/proxy/stream.go` | Без изменений (Manager уже потокобезопасен) |
| `client.yaml` | Добавлен пример `proxy_connections: 10` |

---

## Ключевые архитектурные решения

1. **Последовательный dial** — каждый слот получает полный handshake до перехода к следующему. При ошибке все уже открытые закрываются.
2. **Per-slot Manager** — каждый слот имеет собственный `proxy.Manager` со своей картой streamID→Local и своим мьютексом. Нет разделяемых структур между слотами.
3. **Round-robin через `atomic.Uint64`** — лёгкий lock-free счётчик, достаточный для MVP. Не гарантирует равномерности при долгоживущих стримах.
4. **Одна read-loop на слот** — каждая читает только свой stream, что исключает contention на чтении.
5. **Полный reconnect при ошибке любого слота** — упрощает обработку ошибок (не нужно координировать частичное состояние).
6. **QUIC keepalive на слоте 0** — одного достаточно для поддержания QUIC-сессии; keepalive не зависит от количества слотов.
7. **Количество слотов конфигурируемо** — `proxy_connections` в конфиге, дефолт 10.

---

## Покрытие Acceptance Criteria

| AC | Статус | Доказательство |
|---|---|---|
| AC-001: N параллельных WS | ✅ | `dialProxySlots(N)` создаёт N streams, каждый с handshake |
| AC-002: Round-robin | ✅ | `nextSlot.Add(1) % numSlots` в хендлере `onConn` |
| AC-003: Полный reconnect | ✅ | Любая ошибка в `errgroup`→ `runProxySessionMulti` возвращает→ `runProxyMode` повторяет `dialProxySlots` |
| AC-004: TUN не затронут | ✅ | `Run()` выбирает `reconnectLoop` при `mode != "proxy"`, multi-conn код недоступен |

---

## Trace-маркеры

- `@sk-task win-proxy-multi-conn#T2.1` — `proxySlot` и конфигурация
- `@sk-task win-proxy-multi-conn#T2.2` — `doHandshake`
- `@sk-task win-proxy-multi-conn#T2.3` — `dialProxySlots`
- `@sk-task win-proxy-multi-conn#T3.1..T3.6` — `runProxySessionMulti`
- `@sk-task win-proxy-multi-conn#T3.3` — `proxyReadLoop`
- `@sk-task win-proxy-multi-conn#T4.1` — `runProxyMode`

---

## Артефакты

- `specs/active/win-proxy-multi-conn/spec.md` — spec
- `specs/active/win-proxy-multi-conn/plan.md` — plan (данный файл)
- `specs/active/win-proxy-multi-conn/tasks.md` — tasks
- `src/internal/config/client.go` — поле `ProxyConnections`
- `src/internal/bootstrap/client/proxy.go` — реализация
