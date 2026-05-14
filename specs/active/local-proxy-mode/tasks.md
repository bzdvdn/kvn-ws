# Local Proxy Mode — Задачи

## Phase Contract

Inputs: plan.md, spec.md
Outputs: 4 фазы, 10 задач, покрытие всех 7 AC
Stop if: —

## Implementation Context

- **Цель MVP:** SOCKS5 listener + WS forwarding + mode config (AC-001, AC-003, AC-004, AC-006)
- **Инварианты:** новый пакет `internal/proxy/`; TUN-режим не меняется; сервер получает новый handler
- **Контракты/протокол:** FrameTypeProxy=0x05; wire: `[streamID:4][len:2][dst_len:2][dst][data]`
- **Границы scope:** не трогаем handshake, session, routing (кроме exclusion), tun
- **Proof signals:** SOCKS5 round-trip, curl --socks5, mode switching test
- **References:** DEC-001—DEC-004, AC-001—AC-007

## Surface Map

| Surface | Tasks |
|---------|-------|
| src/internal/transport/framing/framing.go | T1.1 |
| src/internal/proxy/listener.go | T2.1, T3.1, T3.2, T3.3 |
| src/internal/proxy/stream.go | T2.2 |
| src/internal/config/client.go | T1.1 |
| src/cmd/client/main.go | T1.1 |
| src/cmd/server/main.go | T1.2 |

## Фаза 1: Основа

Цель: подготовить FrameTypeProxy, конфиг, серверный forwarder.

- [x] T1.1 Добавить `FrameTypeProxy = 0x05` в `framing/const.go`; добавить поля `Mode`, `ProxyListen`, `ProxyAuth` в `config.ClientConfig`; добавить mode-переключатель в `cmd/client/main.go` (`if cfg.Mode == "proxy"`). Touches: src/internal/transport/framing/framing.go, src/internal/config/client.go, src/cmd/client/main.go
- [x] T1.2 Реализовать серверный обработчик FrameTypeProxy: парсинг `[streamID+dst+data]`, `net.Dial("tcp", dst)`, bidir copy между dst и WS-потоком (map streamID→conn, cleanup при закрытии WS). Touches: src/cmd/server/main.go

## Фаза 2: MVP Slice

Цель: SOCKS5 listener + WS forwarding.

- [x] T2.1 Реализовать SOCKS5 listener (RFC 1928): `net.Listen("tcp", cfg.ProxyListen)`, принять TCP, выполнить SOCKS5 handshake (NO AUTH), извлечь dst, передать stream-менеджеру. Touches: src/internal/proxy/listener.go
- [x] T2.2 Реализовать ProxyStream: структура с streamID, dst, conn; методы Read/Write (через WS-соединение); менеджер потоков с map для route по streamID. Touches: src/internal/proxy/stream.go

## Фаза 3: Основная реализация

Цель: HTTP CONNECT, auth, exclusion.

- [x] T3.1 Реализовать HTTP CONNECT handler: парсинг `CONNECT host:port HTTP/1.1`, ответ `200 Connection established`, затем bidir copy через stream. Touches: src/internal/proxy/listener.go
- [x] T3.2 Реализовать SOCKS5 auth (RFC 1929): при `ProxyAuth` в конфиге — метод 0x02 (username/password), проверка на сервере, отказ при несовпадении. Touches: src/internal/proxy/listener.go
- [x] T3.3 Интегрировать CIDR/domain exclusion: перед созданием stream — `routing.RuleSet.Route(dstIP)`, если `RouteDirect` — не шлём через WS, а коннектимся напрямую из клиента. Touches: src/internal/proxy/listener.go, src/internal/proxy/stream.go

## Фаза 4: Проверка

Цель: доказать, что всё работает на всех платформах.

- [x] T4.1 Добавить cross-platform CI: matrix `os: [ubuntu, windows, macos]` в `.github/workflows/ci.yml`, сборка + smoke test proxy-режима. Touches: .github/workflows/ci.yml
- [x] T4.2 Прогнать `go test -race ./...` и `golangci-lint run ./...`, исправить проблемы. Touches: все затронутые файлы

## Покрытие критериев приемки

- AC-001 -> T2.1, T2.2
- AC-002 -> T3.1
- AC-003 -> T1.1
- AC-004 -> T1.1
- AC-005 -> T3.2
- AC-006 -> T4.1
- AC-007 -> T3.3

## Заметки

- T1.1/T1.2 — можно параллелить
- T2.1 зависит от T2.2 (stream), но сами listener и stream можно делать в одном T2
- T3.1/T3.2/T3.3 — можно параллелить (разные секции listener)
- T4.1 зависит от proxy-сборки (T2.x)
- T4.2 — финальная, после всех задач
