# Transparent Proxy — Задачи

## Phase Contract

Inputs: `plan.md`, `data-model.md`, `spec.md`
Outputs: упорядоченные исполнимые задачи с покрытием AC.
Stop if: задачи расплывчаты или coverage не удаётся сопоставить.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `internal/config/client.go` | T1.1 |
| `internal/transparent/transparent.go` | T1.2, T3.1 |
| `internal/transparent/iptables_linux.go` | T2.1, T4.1 |
| `internal/transparent/transparent_stub.go` | T2.1 |
| `internal/proxy/listener.go` | T2.2, T4.2 |
| `internal/dnsproxy/dnsproxy.go` | T1.3, T2.3, T4.3 |
| `internal/bootstrap/client/client.go` | T3.1, T4.5 |
| `internal/bootstrap/client/proxy.go` | T3.1 |
| `internal/config/client_test.go` | T4.4 |
| `internal/bootstrap/client/client_test.go` | T4.5 |

## Implementation Context

- **Цель MVP:** Linux transparent proxy (iptables REDIRECT, IPv4 TCP) + DNS-proxy + Docker bridge/host, без TUN mode и без macOS pf (P2).
- **Инварианты/семантика:**
  - `TransparentManager` следует паттерну `systemproxy.PlatformManager` (interface + platform-specific impl, Set/Restore).
  - Transparent-детекция через `SO_ORIGINAL_DST` (getsockopt) — третий тип в listener после SOCKS5 и HTTP CONNECT.
  - REDIRECT меняет dst на localhost → оригинальный dst читается из сокета.
  - Exclude rules: iptables NOT (не доходят до userspace), а не в listener.
  - DNS-proxy: UDP server на `127.0.0.53:53`, `/etc/resolv.conf` → `nameserver 127.0.0.53`, backup при старте, restore при стопе.
  - Transparent работает только в proxy mode; TUN mode не использует transparent.
  - `transparent: false` (default) — существующий proxy mode без изменений.
- **Ошибки/коды:**
  - Нет root/CAP_NET_ADMIN → warning "transparent proxy requires root, skipping", proxy mode продолжает.
  - iptables не найден → warning.
  - SO_ORIGINAL_DST не поддерживается → fallback, логировать.
  - Порт 53 занят → Warn, пользователь меняет `dns_proxy.listen` в конфиге.
- **Контракты/протокол:**
  - Поля конфига: `Transparent bool` + `DNSProxyCfg {Listen string}` (см. `data-model.md`).
  - При transparent: proxy listener слушает на `0.0.0.0:2310` вместо `127.0.0.1:2310`.
  - Правила iptables: `-t nat -A PREROUTING -p tcp ! -d <exclude> -j REDIRECT --to-port <proxy_port>`.
- **Границы scope:**
  - Не делаем UDP transparent proxy (TPROXY) — DNS решается через DNS-proxy.
  - Не делаем macOS pf anchor в этом инкременте (AC-007 → P2).
  - Не делаем IPv6 transparent proxy.
  - Не меняем SOCKS5/HTTP CONNECT хендлеры — только добавляем transparent detection.
- **Proof signals:**
  - `go build ./src/...` + `go vet ./src/...` проходят.
  - Unit test coverage для новых пакетов (mock exec).
  - Ручная проверка: curl через transparent proxy на Linux.
  - Trace-маркеры `@sk-task` в новом коде.
- **References:** DEC-001..DEC-004 (см. `plan.md`), DM (`data-model.md`), RQ-001..RQ-010 (см. `spec.md`).

## Фаза 1: Foundation

Цель: добавить поля конфига, создать пустые пакеты `internal/transparent/` и `internal/dnsproxy/`.

- [x] T1.1 Добавить `Transparent bool` и `DNSProxyCfg` в `ClientConfig` с дефолтами (`Transparent: false`, `DNSProxyCfg.Listen: "127.0.0.53:53"`). Touches: `internal/config/client.go`
- [x] T1.2 Создать `internal/transparent/transparent.go` с интерфейсом `TransparentManager {Set(ctx, logger, port) error; Restore(ctx, logger) error}`. Touches: `internal/transparent/transparent.go`
- [x] T1.3 Создать `internal/dnsproxy/dnsproxy.go` — каркас пакета (type Server struct, заглушки Run/Shutdown). Touches: `internal/dnsproxy/dnsproxy.go`

## Фаза 2: Core Implementation

Цель: реализовать iptables REDIRECT, transparent-детекцию в listener, DNS-proxy. Задачи независимы, можно параллелить.

- [x] T2.1 Реализовать `iptables_linux.go`: `Set()` — установка iptables REDIRECT правил (TCP 80/443 + полный диапазон, с exclude CIDR через NOT), `Restore()` — удаление по `--line-number` или сохранённому списку; `iptables` vs `iptables-legacy` detection; заглушку для не-Linux (`transparent_stub.go`). Touches: `internal/transparent/iptables_linux.go`, `src/internal/transparent/transparent_stub.go`
- [x] T2.2 Реализовать transparent-детекцию в `listener.go`: в `handleClient` после `default:` (не SOCKS/HTTP) попробовать `SO_ORIGINAL_DST` через `getsockopt`, извлечь оригинальный `ip:port` и передать в `onConn`. Touches: `internal/proxy/listener.go`
- [x] T2.3 Реализовать DNS-proxy: UDP listener, парсинг DNS запроса, форвард через TCP к upstream (через KVN tunnel), возврат ответа. `/etc/resolv.conf` backup/restore при старте/стопе. Touches: `internal/dnsproxy/dnsproxy.go`

## Фаза 3: Integration

Цель: связать всё в `client.go` — root check, Set/Restore lifecycle, signal handling, Docker.

- [x] T3.1 Интегрировать transparent proxy в `client.go`:
  - Проверка root (`os.Geteuid() == 0` или `CAP_NET_ADMIN`).
  - Если `transparent: true` и root — `TransparentManager.Set()` перед proxy session.
  - Если не root — warning, transparent off, proxy mode продолжается.
  - `defer TransparentManager.Restore()` при остановке.
  - При transparent mode: proxy listener на `0.0.0.0:<port>`.
  - DNS-proxy: `Run()`/`Shutdown()` при старте/стопе proxy session.
  - Touches: `internal/bootstrap/client/client.go`, `src/internal/bootstrap/client/proxy.go`
- [x] T3.2 Проверить работу в Docker bridge mode: iptables внутри контейнера с `--cap-add=NET_ADMIN`. Правила уже устанавливаются через `iptables_linux.go` внутри network namespace контейнера — адаптация не требуется, только тестирование. Touches: (Dockerfile/test setup, без изменения кода)

## Фаза 4: Tests & Verification

Цель: unit-тесты для всех новых компонентов.

- [x] T4.1 Unit-тесты `iptables_test.go`: Set/Restore с mock exec.Command, проверка флагов, exclude CIDR, iptables-legacy detection. Touches: `internal/transparent/iptables_test.go`
- [x] T4.2 Unit-тесты `listener_test.go`: transparent-детекция с mock TCPConn (SO_ORIGINAL_DST через getsockopt), проверка что SOCKS5/HTTP CONNECT не затронуты. Touches: `internal/proxy/listener_test.go`
- [x] T4.3 Unit-тесты `dnsproxy_test.go`: UDP запрос/ответ через mock upstream. Touches: `internal/dnsproxy/dnsproxy_test.go`
- [x] T4.4 Unit-тесты `client_test.go` (config): парсинг `transparent: true/false`, `dns_proxy.listen`. Touches: `internal/config/client_test.go`
- [x] T4.5 Unit-тест `client_test.go` (bootstrap): root check → warning. Touches: `internal/bootstrap/client/client_test.go`
- [x] T4.6 Ручная верификация: собрать `go build ./src/...` + `go vet ./src/...`, `go test -race` — все PASS.

## Фаза 5: Transparent proxy fixes & domain DNS routing

Цель: исправить SO_ORIGINAL_DST errno баг, добавить domain-based DNS routing для excluded доменов в transparent mode.

- [x] T5.1 Fix `getOriginalDst()` — заменить `var opErr error` на `var errno syscall.Errno`, исправить классическую Go ошибку `syscall.Errno(0)` → non-nil interface. Touches: `internal/proxy/listener.go`
- [x] T5.2 Добавить `SetLogFn` в proxy listener для отладки transparent-соединений. Touches: `internal/proxy/listener.go`
- [x] T5.3 Добавить `ResolvConfBackup.Nameservers()` — сохранять оригинальные nameserver-ы из `/etc/resolv.conf` при backup. Touches: `internal/dnsproxy/dnsproxy.go`
- [x] T5.4 Добавить `SetRouteFunc`, `SetOrigResolvers`, `resolveDirect`, `extractDNSDomain` в DNS proxy. Для excluded доменов DNS запрос резолвится локально через оригинальные nameserver-ы, а не через туннель. Touches: `internal/dnsproxy/dnsproxy.go`
- [x] T5.5 В `proxy.go` создать routeSet до DNS proxy, передать `RouteFunc` и `OrigResolvers` в DNS proxy. Touches: `internal/bootstrap/client/proxy.go`

## Покрытие критериев приемки

- AC-001 -> T2.1, T3.1, T4.1
- AC-002 -> T2.2, T4.2
- AC-003 -> T2.2, T4.2
- AC-004 -> T2.1, T4.1
- AC-005 -> T3.1
- AC-006 -> T3.2
- AC-007 -> deferred (P2, macOS pf anchor)
- AC-008 -> T3.1, T4.5
- AC-009 -> T2.3, T4.3
- AC-010 -> T5.1 (SO_ORIGINAL_DST errno fix)
- AC-011 -> T5.3, T5.4, T5.5 (domain-based DNS routing)

## Заметки

- Фаза 2 задачи (T2.1, T2.2, T2.3) независимы и могут выполняться параллельно.
- T3.2 (Docker) — проверочная задача, код не меняется, только тестирование.
- T5.x задачи выполнены после verify:pass, поэтому verify.md нужно обновить.
- trace-маркеры `@sk-task transparent-proxy#T*.*` ставить на новые функции.

Готово к: `/speckeep.implement transparent-proxy`
