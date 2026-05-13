# RoadMap — KVN-over-WS (kvn-ws)

> **Цель:** Продуктовый релиз (v1.0.0) — клиент и сервер, работающие в production-среде.
> **Стратегия:** Сначала работающий end-to-end tunnel, затем routing/rules, затем production hardening.
> **Принцип:** Каждый этап даёт измеримый прогресс — от `nc -vz` до полноценного продакшена.

---

## Этап 0 — Фундамент (Sprint 0, 1–2 дня)

| # | Задача | Результат | Зависимости |
|---|--------|-----------|-------------|
| 0.1 | Инициализация Go-модуля `github.com/bzdvdn/kvn-ws` | `go.mod`, `go.sum`, базовый `main.go` в `src/cmd/` | — |
| 0.2 | Скелет структуры `src/internal/*` | Пустые пакеты с `package` и заглушками | 0.1 |
| 0.3 | Dockerfile (multi-stage) + docker-compose.yml | Сборка образа, `docker compose up` поднимает процесс | 0.1 |
| 0.4 | CI-пайплайн (GitHub Actions): `go test ./...`, lint, build | PR-проверки | 0.2 |
| 0.5 | Конфигурация: `spf13/viper` парсинг `client.yaml` / `server.yaml` | Загрузка и валидация конфига | 0.2 |
| 0.6 | Логгер: `uber-go/zap` с JSON-output | Структурированные логи | 0.2 |

**Gate:** `go build ./src/...` проходит, `docker compose build` успешен.

---

## Этап 1 — Core Tunnel MVP (Sprint 1, 3–5 дней)

**Самый короткий путь к работающему туннелю.**

| # | Задача | Результат | Зависимости |
|---|--------|-----------|-------------|
| 1.1 | TUN-абстракция (Linux) | `TunDevice` interface, чтение/запись IP-пакетов | 0.2 |
| 1.2 | WebSocket transport (client dial + server accept) | `gorilla/websocket`, HTTP Upgrade, WSS | 0.2 |
| 1.3 | TLS 1.3 (server cert, client config) | TLS listener на сервере, TLS dial клиента | 1.2 |
| 1.4 | Фрейминг: бинарные фреймы (Type/Flags/Length/Payload) | `Frame.Encode()` / `Decode()`, чтение/запись в WS | 1.2 |
| 1.5 | Client-Server handshake | Client Hello → Server Hello, session_id, assigned IP | 1.4 |
| 1.6 | Auth: bearer-token | Проверка токена на сервере, отказ при невалидном | 1.5 |
| 1.7 | Packet forwarding client→server→TUN | IP-пакет из TUN клиента доходит до TUN сервера | 1.1, 1.4, 1.5 |
| 1.8 | Packet forwarding server→client→TUN (reply) | Ответный трафик идёт обратно | 1.7 |
| 1.9 | IP Pool Manager (in-memory) | Allocate/Release/Resolve session → IP | 1.5 |
| 1.10 | Graceful shutdown (SIGTERM, контексты) | Никаких брошенных TUN/сокетов | 1.1, 1.2 |

**Gate:** `ping <server_assigned_ip>` проходит через туннель. Client → Server → Internet.

---

## Этап 2 — Routing & Split Tunnel (Sprint 2, 3–4 дня)

| # | Задача | Результат | Зависимости |
|---|--------|-----------|-------------|
| 2.1 | Default route: `server` / `direct` | Весь трафик через туннель или напрямую (bypass) | 1.8 |
| 2.2 | Split tunnel по CIDR (`include_ranges` / `exclude_ranges`) | IP-пакеты в указанных диапазонах идут по правилу | 2.1 |
| 2.3 | Routing по отдельным IP (`include_ips` / `exclude_ips`) | Правила для отдельных адресов | 2.1 |
| 2.4 | DNS resolver (кеш, TTL) для доменных правил | Разрешение доменов в IP, кеширование | 0.2 |
| 2.5 | Routing по доменам (`include_domains` / `exclude_domains`) | Трафик на домен идёт по DNS→IP→правило | 2.4, 2.1 |
| 2.6 | Ordered rules engine (exclude→include, first match) | Движок последовательного применения правил | 2.2, 2.3, 2.5 |
| 2.7 | Server-side NAT (iptables/nftables MASQUERADE) | Пакеты из клиента выходят в интернет с IP сервера | 1.7 |
| 2.8 | DNS override для full-tunnel режима | DNS-запросы не текут мимо туннеля | 2.1 |

**Gate:** Split tunnel: YouTube напрямую, корпоративные ресурсы через туннель. `dig` + `curl --resolve` подтверждают маршрутизацию.

---

## Этап 3 — Production Hardening (Sprint 3, 3–5 дней)

**Без этого этапа продукт нельзя выпускать в эксплуатацию.**

| # | Задача | Результат | Зависимости |
|---|--------|-----------|-------------|
| 3.1 | Auto-reconnect (exponential backoff + jitter) | Клиент переподключается при обрыве | 1.2 |
| 3.2 | Keepalive (PING/PONG, session timeout) | Обрыв соединения детектируется за <30s | 1.5 |
| 3.3 | Kill-switch (блокировка route leakage при падении) | При отключении VPN трафик не утекает | 1.1, 2.1 |
| 3.4 | Rate limiting (auth attempts, packets per session) | Защита от brute-force и abuse | 1.6 |
| 3.5 | Session expiry + reclaim (idle + TTL) | Брошенные сессии освобождают IP | 1.9 |
| 3.6 | IP Pool persistence (BoltDB) | IP-пул восстанавливается после рестарта сервера | 1.9 |
| 3.7 | Prometheus метрики (active sessions, throughput, errors) | `/metrics` endpoint, дашборд | 1.8 |
| 3.8 | Health endpoint (liveness + readiness) | `GET /health` для orchestration | 1.5 |
| 3.9 | Graceful config reload (SIGHUP) | Смена лимитов/tokens без перезапуска | 0.5 |
| 3.10 | Structured error handling + audit trail | Каждый auth/ACL failure логируется | 0.6 |
| 3.11 | CLI flags + env override поверх YAML | `--config`, `KWN_SERVER_ADDR` и т.д. | 0.5 |

**Gate:** 24h стабильной работы клиент-сервер с активным трафиком без падений и утечек памяти.

---

## Этап 4 — Security & ACL (Sprint 4, 2–3 дня)

| # | Задача | Результат | Зависимости |
|---|--------|-----------|-------------|
| 4.1 | CIDR ACL на сервере (allow/deny subnets) | Сервер отклоняет пакеты из запрещённых подсетей | 1.7 |
| 4.2 | Per-user bandwidth quota | Ограничение скорости для конкретного клиента | 1.6 |
| 4.3 | Session limits per token/user | `max_sessions` на токен | 1.6 |
| 4.4 | Origin/Referer validation (anti-abuse) | Проверка HTTP Origin на сервере | 1.2 |
| 4.5 | Admin API (CLI over HTTP) | Просмотр активных сессий, принудительный disconnect | 1.5 |
| 4.6 | Mutual TLS (mTLS) — опционально | Сертификаты клиентов для аутентификации | 1.3 |

**Gate:** Security audit checklist пройден (OWASP top-10 для WebSocket).

---

## Этап 5 — IPv6 & Dual-Stack (Sprint 5, 2–3 дня)

| # | Задача | Результат | Зависимости |
|---|--------|-----------|-------------|
| 5.1 | IPv6 TUN (Linux) | TUN в режиме IPv6 или dual-stack | 1.1 |
| 5.2 | IPv6 IP pool | Allocate/Release fd00::/64 адреса | 1.9 |
| 5.3 | IPv6 handshake + session | Сервер назначает IPv6 + IPv4 | 1.5 |
| 5.4 | IPv6 routing + NAT | IPv6 MASQUERADE на сервере | 2.7 |
| 5.5 | Dual-stack routing policy | Раздельная маршрутизация IPv4/IPv6 | 2.1 |

**Gate:** `ping6` и IPv6-only сайты работают через туннель.

---

## Этап 6 — Performance & Polish (Sprint 6, 2–3 дня)

| # | Задача | Результат | Зависимости |
|---|--------|-----------|-------------|
| 6.1 | `sync.Pool` для буферов | Снижение GC pressure | 1.7 |
| 6.2 | Batch writes / TCP_NODELAY | Уменьшение latency | 1.2 |
| 6.3 | MTU negotiation + PMTU strategy | Фрагментация под контролем | 1.1 |
| 6.4 | Compress (permessage-deflate / zlib payload) | Снижение bandwidth | 1.4 |
| 6.5 | Multiplex channels (опционально) | Несколько логических потоков в одном WS | 1.2 |
| 6.6 | Load testing (1000+ sessions) | Стабильность под нагрузкой | 3.1–3.11 |

**Gate:** throughput ≥ 80% от raw TCP, latency overhead ≤ 15%.

---

## Этап 7 — Docs & Release (Sprint 7, 2–3 дня)

| # | Задача | Результат | Зависимости |
|---|--------|-----------|-------------|
| 7.1 | Документация `docs/en/` | README, quickstart, config reference, architecture | всё |
| 7.2 | Документация `docs/ru/` | Полный перевод на русский | 7.1 |
| 7.3 | Примеры в `examples/` | docker-compose.yml, client.yaml, server.yaml, скрипты | всё |
| 7.4 | CHANGELOG.md | Список изменений по версиям | всё |
| 7.5 | GitHub Release v1.0.0 | Tag + Release Notes | 7.1–7.4 |
| 7.6 | README.md (корневой) | Бейджи, быстрый старт, ссылки на docs | 7.1 |

**Gate:** Пользователь за 5 минут поднимает сервер через `docker compose up` и подключает клиента следуя `docs/en/quickstart.md`.

---

## Таблица приоритетов (как быстрее в production)

| Уровень | Этапы | Что даёт |
|---------|-------|----------|
| 🔴 Critical Path | 0 → 1 → 2 → 3 | **Работающий production-готовый VPN с routing** |
| 🟡 Important | 4 (security) | Защита от злоупотреблений |
| 🟢 Nice-to-have | 5, 6 | IPv6 + performance |
| 📦 Release | 7 | Документация и релизный процесс |

**Минимальный путь до v1.0-beta:** Этапы 0–1–2–3 (≈14–20 дней при full-time).
**До v1.0-stable:** +Этапы 4–7 (≈22–30 дней).

---

## Принятые решения (Architecture Decision Records — быстрые)

- **Go 1.22+** — контексты, generics, `errors.Join`.
- **gorilla/websocket** vs nhooyr: gorilla — стабильнее и больше production-tested для нашего кейса.
- **BoltDB** для IP-pool: встраиваемая, zero deps, crash-safe.
- **iptables MASQUERADE** для NAT: стандартный Linux facility, не требует сложного кода.
- **split tunnel на клиенте** (не на сервере): сервер не знает о client-side routing, меньше поверхность атаки.

---

*RoadMap утверждён: 2026-05-13.*
*Следующий шаг: `/speckeep.spec core-tunnel` — спецификация Этапа 1.*
