# Transparent Proxy — План

## Phase Contract

Inputs: `specs/active/transparent-proxy/spec.md`
Outputs: `plan.md`, `data-model.md`
Stop if: нет — spec чёткая, scope замкнут.

## Цель

Добавить transparent proxy mode в kvn-client: на Linux — iptables REDIRECT, на macOS — pf anchor. Весь TCP-трафик автоматически перенаправляется на локальный KVN proxy без настройки приложений. Встроенный DNS-proxy исключает DNS-утечки. Поддержка Docker (bridge + host) с `--cap-add=NET_ADMIN`.

## MVP Slice

Linux iptables REDIRECT (TCP 80/443 + полный диапазон) + DNS-proxy + Docker bridge/host. Закрывает AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-008, AC-009.

macOS pf anchor (AC-007) — второй инкремент.

## First Validation Path

```bash
# Хост Linux
sudo kvn-client --mode proxy --transparent

# Другой терминал:
curl -v http://httpbin.org/get   # → проходит через KVN, в логе "transparent proxy: intercepted"
curl -v https://example.com      # → то же
iptables -t nat -L PREROUTING    # → видно REDIRECT правило
```

## Scope

1. Пакет `internal/transparent/` — управление iptables/nftables правилами (Linux) и pf anchor (macOS).
2. Расширение `internal/proxy/listener.go` — добавление transparent-детекции через `SO_ORIGINAL_DST`.
3. Расширение `internal/bootstrap/client/client.go` — инициализация transparent proxy + cleanup.
4. Расширение `internal/config/client.go` — поля `Transparent` и `DNSProxy`.
5. Пакет `internal/dnsproxy/` (новый) — встроенный DNS-сервер, форвардящий запросы через KVN tunnel.
6. `internal/proxy/stream.go` — `ForwardToStream` остаётся без изменений (уже поддерживает transparent, так как dst из SO_ORIGINAL_DST подставляется как обычный адрес).

**Вне scope (первого инкремента):**
- macOS pf anchor (AC-007) — отложен на P2.
- IPv6 transparent proxy.
- GUI-переключатель.

## Implementation Surfaces

### Новые

| Surface | Назначение |
|---|---|
| `internal/transparent/iptables_linux.go` | Установка/удаление iptables REDIRECT правил |
| `internal/transparent/pf_macos.go` | Установка/удаление pf anchor (macOS P2) |
| `internal/transparent/transparent.go` | Общий интерфейс `TransparentManager` |
| `internal/dnsproxy/dnsproxy.go` | UDP DNS-сервер, форвард через KVN tunnel |

### Существующие (изменяемые)

| Surface | Изменение |
|---|---|
| `internal/config/client.go` | +`Transparent bool`, +`DNSProxyCfg` |
| `internal/proxy/listener.go` | + transparent-детекция: если первый байт не SOCKS/HTTP → `SO_ORIGINAL_DST`; +`SetLogFn` для отладки; fix: `syscall.Errno(0)` → nil interface bug |
| `internal/bootstrap/client/client.go` | + запуск `TransparentManager.Set()` перед proxy session, `Restore()` в defer |
| `internal/bootstrap/client/proxy.go` | `runProxySession`: при `transparent` слушать на `0.0.0.0`, передавать listener-у флаг transparent; создание routeSet до DNS proxy; передача `RouteFunc` и `OrigResolvers` в DNS proxy |
| `internal/dnsproxy/dnsproxy.go` | +`SetRouteFunc`, `SetOrigResolvers`, `resolveDirect`, `extractDNSDomain`; `ResolvConfBackup.Nameservers()` |

## Bootstrapping Surfaces

- `internal/transparent/` — создать директорию и пустой пакет.
- `internal/dnsproxy/` — создать директорию и пустой пакет.

## Влияние на архитектуру

- **Нет изменения протокола** — transparent-соединения обрабатываются тем же proxy listener, но dst адрес извлекается из SO_ORIGINAL_DST вместо SOCKS5/HTTP CONNECT.
- **PlatformManager параллель** — `TransparentManager` следует тому же паттерну, что `systemproxy.PlatformManager` (интерфейс + platform-specific имплементации).
- **DNS-proxy** — новый компонент, не влияет на существующий data plane.

## Acceptance Approach

| AC | Подход | Surfaces | Валидация |
|---|---|---|---|
| AC-001 | iptables REDIRECT правило ставится при старте, удаляется при стопе | `iptables_linux.go`, `client.go` | `iptables -t nat -L` до/после |
| AC-002 | HTTP запрос перехватывается, оригинальный dst извлекается | `listener.go`, `iptables_linux.go` | curl + лог "intercepted" |
| AC-003 | HTTPS запрос перехватывается (CONNECT через proxy) | `listener.go`, `iptables_linux.go` | curl + лог "intercepted" |
| AC-004 | Exclude CIDR iptables NOT правилом | `iptables_linux.go`, `RoutingCfg` | curl к 10.0.0.0/8 не логируется |
| AC-005 | SIGTERM handler снимает правила | `client.go` | iptables после kill |
| AC-006 | Docker контейнер c `--cap-add=NET_ADMIN` | `iptables_linux.go` | curl внутри контейнера |
| AC-007 | macOS pf anchor (P2, deferred) | — | — |
| AC-008 | Без root → warning, transparent off | `client.go` | лог "requires root" |
| AC-009 | DNS-proxy слушает :53, форвардит через KVN | `dnsproxy.go`, `/etc/resolv.conf` | dig + лог DNS |
| AC-010 | SO_ORIGINAL_DST не падает с errno 0 | `listener.go` | `getOriginalDst` с `syscall.Errno(0)` не считается ошибкой |
| AC-011 | Exclude_domains резолвятся локально, не через туннель | `dnsproxy.go`, `proxy.go` | `nslookup corp.domain.ru 127.0.0.54` → IP от локального DNS, не от сервера |

## Данные и контракты

- **Data model**: `ClientConfig` расширяется полями `Transparent bool` и `DNSProxy DNSProxyCfg` (см. `data-model.md`).
- **Protocol**: без изменений — transparent не меняет wire format.
- **API/events**: без изменений.

## Стратегия реализации

### DEC-001 Единый пакет `internal/transparent/`

- **Why**: iptables/nftables и pf имеют общий интерфейс (Set/Restore). Platform-specific impl в отдельных файлах.
- **Tradeoff**: небольшое дублирование в тестах.
- **Affects**: `internal/transparent/`.
- **Validation**: `Set()` → iptables rule exists; `Restore()` → rule gone.

### DEC-002 Transparent-детекция через `SO_ORIGINAL_DST` в listener

- **Why**: REDIRECT меняет dst на localhost, оригинал доступен через `getsockopt(SO_ORIGINAL_DST)`. Не требует изменения протокола.
- **Tradeoff**: только IPv4, только TCP. Этого достаточно для MVP.
- **Affects**: `internal/proxy/listener.go`.
- **Validation**: curl через transparent proxy → лог с оригинальным `ip:port`.

### DEC-003 DNS-proxy как отдельный пакет `internal/dnsproxy/`

- **Why**: DNS-proxy ортогонален transparent proxy (может использоваться отдельно). Простой UDP forwarder без кеша.
- **Tradeoff**: нет кеширования/фильтрации DNS в MVP.
- **Affects**: `internal/dnsproxy/`.
- **Validation**: `dig google.com @127.0.0.53` → успешный ответ.

### DEC-004 Exclude-правила через iptables NOT, а не в listener

- **Why**: дешевле отбросить пакет в ядре, чем принимать в userspace и проверять. Используем существующие exclude CIDR.
- **Tradeoff**: требует root. Нельзя менять правила динамически (только при старте).
- **Affects**: `iptables_linux.go`.
- **Validation**: curl к 10.0.0.0/8 не попадает в лог прокси.

### DEC-005 Domain-based DNS routing в transparent mode

- **Why**: exclude_domains из конфига не работают в transparent mode, т.к. destination — IP от SO_ORIGINAL_DST. Решение: перехватывать DNS-запросы excluded доменов и резолвить локально.
- **Tradeoff**: требует CIDR в exclude_ranges для TCP (iptables не умеет домены); DNS-запрос excluded домена идёт локально, а не через туннель.
- **Affects**: `internal/dnsproxy/dnsproxy.go`, `internal/bootstrap/client/proxy.go`.
- **Validation**: `nslookup corp.internal.ru 127.0.0.54` → ответ от локального DNS (не через туннель).

## Incremental Delivery

### MVP (Первая ценность)

1. Config: `Transparent bool`, `DNSProxyCfg`.
2. `internal/transparent/iptables_linux.go`: установка/удаление правил REDIRECT.
3. `internal/proxy/listener.go`: transparent-детекция + SO_ORIGINAL_DST.
4. `internal/bootstrap/client/client.go`: интеграция (Set/Restore + проверка root).
5. `internal/dnsproxy/`: базовый UDP DNS-forwarder.
6. Docker: адаптация iptables для контейнера.

**AC:** AC-001..AC-006, AC-008, AC-009.

### Итеративное расширение

- **P2**: macOS pf anchor (AC-007).
- **P3**: Domain-based DNS routing — exclude_domains в transparent mode теперь работают на уровне DNS proxy (локальный резолв).
- **P4 (future)**: TPROXY для UDP.
- **P5 (future)**: IPv6.

## Порядок реализации

1. **Config** — добавить поля (без них нельзя собрать).
2. **`internal/transparent/iptables_linux.go`** — core engine.
3. **`internal/proxy/listener.go`** — transparent-детекция.
4. **`internal/dnsproxy/`** — DNS-proxy.
5. **`client.go`** — интеграция (Set/Restore + root check).
6. **Docker** — тестирование и адаптация.
7. **Тесты** — unit + manual validation.

Пункты 2-4 можно безопасно параллелить.

## Риски

| Риск | Mitigation |
|---|---|
| iptables-legacy vs nft | Проверять `iptables --version`, пробовать оба |
| Docker без `--network host` — REDIRECT может не сработать | Тестировать в bridge mode; альтернатива — nftables с `meta mark` |
| SO_ORIGINAL_DST работает не на всех ядрах | Проверять errno, логировать и fallback |
| DNS-proxy порт 53 занят | Проверять `Listen()` и Warn; пользователь меняет `dns_proxy.listen` |
| /etc/resolv.conf восстановление после краша | Сохранять backup, восстанавливать при старте |

## Rollout и compatibility

- `transparent: false` (default) — существующий proxy mode без изменений.
- `--transparent` флаг только для proxy mode.
- Специальных rollout-действий не требуется.

## Проверка

### Automated tests

| Тест | Покрывает |
|---|---|
| `internal/transparent/iptables_test.go` | Set/Restore правил (mock exec) | DEC-001, AC-001 |
| `internal/proxy/listener_test.go` | transparent-детекция + SO_ORIGINAL_DST mock | DEC-002, AC-002 |
| `internal/dnsproxy/dnsproxy_test.go` | форвард DNS запроса | DEC-003, AC-009 |
| `internal/config/client_test.go` | парсинг `transparent`, `dns_proxy` | — |
| `internal/bootstrap/client/client_test.go` | root check → warning | AC-008 |

### Manual checks

1. `go build ./src/...` + `go vet ./src/...`
2. Запуск `kvn-client --mode proxy --transparent` на Linux
3. `curl -v http://httpbin.org/get` — проходит через прокси
4. `curl -v https://example.com` — проходит через прокси
5. `iptables -t nat -L PREROUTING` — видно правило
6. Остановка клиента — правила удалены
7. Docker: `docker run --cap-add=NET_ADMIN ... curl example.com`

## Соответствие конституции

- нет конфликтов
- Фича на Go, без глобального мутабельного состояния
- Платформенный код в `internal/transparent/` (Clean Architecture: infrastructure в internal)
- Traceability: `@sk-task` на новых функциях
- Docker multi-stage: расширяется для `--cap-add=NET_ADMIN`
