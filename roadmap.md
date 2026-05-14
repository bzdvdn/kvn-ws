# RoadMap — KVN-over-WS (kvn-ws)

> **Цель:** Продуктовый релиз (v1.0.0) — клиент и сервер, работающие в production-среде.
> **Стратегия:** Сначала работающий end-to-end tunnel, затем routing/rules, затем production hardening.
> **Принцип:** Каждый этап даёт измеримый прогресс — от `nc -vz` до полноценного продакшена.

---

## Этап 0 — Фундамент (Sprint 0, 1–2 дня)

| #   | Задача                                                            | Результат                                            | Зависимости |
| --- | ----------------------------------------------------------------- | ---------------------------------------------------- | ----------- |
| 0.1 | Инициализация Go-модуля `github.com/bzdvdn/kvn-ws`                | `go.mod`, `go.sum`, базовый `main.go` в `src/cmd/`   | —           |
| 0.2 | Скелет структуры `src/internal/*`                                 | Пустые пакеты с `package` и заглушками               | 0.1         |
| 0.3 | Dockerfile (multi-stage) + docker-compose.yml                     | Сборка образа, `docker compose up` поднимает процесс | 0.1         |
| 0.4 | CI-пайплайн (GitHub Actions): `go test ./...`, lint, build        | PR-проверки                                          | 0.2         |
| 0.5 | Конфигурация: `spf13/viper` парсинг `client.yaml` / `server.yaml` | Загрузка и валидация конфига                         | 0.2         |
| 0.6 | Логгер: `uber-go/zap` с JSON-output                               | Структурированные логи                               | 0.2         |

**Gate:** `go build ./src/...` проходит, `docker compose build` успешен.

---

## Этап 1 — Core Tunnel MVP (Sprint 1, 3–5 дней)

**Самый короткий путь к работающему туннелю.**

| #    | Задача                                                | Результат                                            | Зависимости   |
| ---- | ----------------------------------------------------- | ---------------------------------------------------- | ------------- |
| 1.1  | TUN-абстракция (Linux)                                | `TunDevice` interface, чтение/запись IP-пакетов      | 0.2           |
| 1.2  | WebSocket transport (client dial + server accept)     | `gorilla/websocket`, HTTP Upgrade, WSS               | 0.2           |
| 1.3  | TLS 1.3 (server cert, client config)                  | TLS listener на сервере, TLS dial клиента            | 1.2           |
| 1.4  | Фрейминг: бинарные фреймы (Type/Flags/Length/Payload) | `Frame.Encode()` / `Decode()`, чтение/запись в WS    | 1.2           |
| 1.5  | Client-Server handshake                               | Client Hello → Server Hello, session_id, assigned IP | 1.4           |
| 1.6  | Auth: bearer-token                                    | Проверка токена на сервере, отказ при невалидном     | 1.5           |
| 1.7  | Packet forwarding client→server→TUN                   | IP-пакет из TUN клиента доходит до TUN сервера       | 1.1, 1.4, 1.5 |
| 1.8  | Packet forwarding server→client→TUN (reply)           | Ответный трафик идёт обратно                         | 1.7           |
| 1.9  | IP Pool Manager (in-memory)                           | Allocate/Release/Resolve session → IP                | 1.5           |
| 1.10 | Graceful shutdown (SIGTERM, контексты)                | Никаких брошенных TUN/сокетов                        | 1.1, 1.2      |

**Gate:** `ping <server_assigned_ip>` проходит через туннель. Client → Server → Internet.

---

## Этап 2 — Routing & Split Tunnel (Sprint 2, 3–4 дня)

| #   | Задача                                                     | Результат                                         | Зависимости   |
| --- | ---------------------------------------------------------- | ------------------------------------------------- | ------------- |
| 2.1 | Default route: `server` / `direct`                         | Весь трафик через туннель или напрямую (bypass)   | 1.8           |
| 2.2 | Split tunnel по CIDR (`include_ranges` / `exclude_ranges`) | IP-пакеты в указанных диапазонах идут по правилу  | 2.1           |
| 2.3 | Routing по отдельным IP (`include_ips` / `exclude_ips`)    | Правила для отдельных адресов                     | 2.1           |
| 2.4 | DNS resolver (кеш, TTL) для доменных правил                | Разрешение доменов в IP, кеширование              | 0.2           |
| 2.5 | Routing по доменам (`include_domains` / `exclude_domains`) | Трафик на домен идёт по DNS→IP→правило            | 2.4, 2.1      |
| 2.6 | Ordered rules engine (exclude→include, first match)        | Движок последовательного применения правил        | 2.2, 2.3, 2.5 |
| 2.7 | Server-side NAT (iptables/nftables MASQUERADE)             | Пакеты из клиента выходят в интернет с IP сервера | 1.7           |
| 2.8 | DNS override для full-tunnel режима                        | DNS-запросы не текут мимо туннеля                 | 2.1           |

**Gate:** Split tunnel: YouTube напрямую, корпоративные ресурсы через туннель. `dig` + `curl --resolve` подтверждают маршрутизацию.

---

## Этап 3 — Production Hardening (Sprint 3, 3–5 дней)

**Без этого этапа продукт нельзя выпускать в эксплуатацию.**

| #    | Задача                                                   | Результат                                       | Зависимости |
| ---- | -------------------------------------------------------- | ----------------------------------------------- | ----------- |
| 3.1  | Auto-reconnect (exponential backoff + jitter)            | Клиент переподключается при обрыве              | 1.2         |
| 3.2  | Keepalive (PING/PONG, session timeout)                   | Обрыв соединения детектируется за <30s          | 1.5         |
| 3.3  | Kill-switch (блокировка route leakage при падении)       | При отключении VPN трафик не утекает            | 1.1, 2.1    |
| 3.4  | Rate limiting (auth attempts, packets per session)       | Защита от brute-force и abuse                   | 1.6         |
| 3.5  | Session expiry + reclaim (idle + TTL)                    | Брошенные сессии освобождают IP                 | 1.9         |
| 3.6  | IP Pool persistence (BoltDB)                             | IP-пул восстанавливается после рестарта сервера | 1.9         |
| 3.7  | Prometheus метрики (active sessions, throughput, errors) | `/metrics` endpoint, дашборд                    | 1.8         |
| 3.8  | Health endpoint (liveness + readiness)                   | `GET /health` для orchestration                 | 1.5         |
| 3.9  | Graceful config reload (SIGHUP)                          | Смена лимитов/tokens без перезапуска            | 0.5         |
| 3.10 | Structured error handling + audit trail                  | Каждый auth/ACL failure логируется              | 0.6         |
| 3.11 | CLI flags + env override поверх YAML                     | `--config`, `KVN_SERVER_ADDR` и т.д.            | 0.5         |

**Gate:** 24h стабильной работы клиент-сервер с активным трафиком без падений и утечек памяти.

---

## Этап 4 — Security & ACL (Sprint 4, 2–3 дня)

| #   | Задача                                   | Результат                                           | Зависимости |
| --- | ---------------------------------------- | --------------------------------------------------- | ----------- |
| 4.1 | CIDR ACL на сервере (allow/deny subnets) | Сервер отклоняет пакеты из запрещённых подсетей     | 1.7         |
| 4.2 | Per-user bandwidth quota                 | Ограничение скорости для конкретного клиента        | 1.6         |
| 4.3 | Session limits per token/user            | `max_sessions` на токен                             | 1.6         |
| 4.4 | Origin/Referer validation (anti-abuse)   | Проверка HTTP Origin на сервере                     | 1.2         |
| 4.5 | Admin API (CLI over HTTP)                | Просмотр активных сессий, принудительный disconnect | 1.5         |
| 4.6 | Mutual TLS (mTLS) — опционально          | Сертификаты клиентов для аутентификации             | 1.3         |

**Gate:** Security audit checklist пройден (OWASP top-10 для WebSocket).

---

## Этап 5 — IPv6 & Dual-Stack (Sprint 5, 2–3 дня)

| #   | Задача                    | Результат                          | Зависимости |
| --- | ------------------------- | ---------------------------------- | ----------- |
| 5.1 | IPv6 TUN (Linux)          | TUN в режиме IPv6 или dual-stack   | 1.1         |
| 5.2 | IPv6 IP pool              | Allocate/Release fd00::/64 адреса  | 1.9         |
| 5.3 | IPv6 handshake + session  | Сервер назначает IPv6 + IPv4       | 1.5         |
| 5.4 | IPv6 routing + NAT        | IPv6 MASQUERADE на сервере         | 2.7         |
| 5.5 | Dual-stack routing policy | Раздельная маршрутизация IPv4/IPv6 | 2.1         |

**Gate:** `ping6` и IPv6-only сайты работают через туннель.

---

## Этап 6 — Performance & Polish (Sprint 6, 2–3 дня)

| #   | Задача                                       | Результат                               | Зависимости |
| --- | -------------------------------------------- | --------------------------------------- | ----------- |
| 6.1 | `sync.Pool` для буферов                      | Снижение GC pressure                    | 1.7         |
| 6.2 | Batch writes / TCP_NODELAY                   | Уменьшение latency                      | 1.2         |
| 6.3 | MTU negotiation + PMTU strategy              | Фрагментация под контролем              | 1.1         |
| 6.4 | Compress (permessage-deflate / zlib payload) | Снижение bandwidth                      | 1.4         |
| 6.5 | Multiplex channels (опционально)             | Несколько логических потоков в одном WS | 1.2         |
| 6.6 | Load testing (1000+ sessions)                | Стабильность под нагрузкой              | 3.1–3.11    |

**Gate:** throughput ≥ 80% от raw TCP, latency overhead ≤ 15%.

---

## Этап 7 — Docs & Release (Sprint 7, 2–3 дня)

| #   | Задача                  | Результат                                             | Зависимости |
| --- | ----------------------- | ----------------------------------------------------- | ----------- |
| 7.1 | Документация `docs/en/` | README, quickstart, config reference, architecture    | всё         |
| 7.2 | Документация `docs/ru/` | Полный перевод на русский                             | 7.1         |
| 7.3 | Примеры в `examples/`   | docker-compose.yml, client.yaml, server.yaml, скрипты | всё         |
| 7.4 | CHANGELOG.md            | Список изменений по версиям                           | всё         |
| 7.5 | GitHub Release v1.0.0   | Tag + Release Notes                                   | 7.1–7.4     |
| 7.6 | README.md (корневой)    | Бейджи, быстрый старт, ссылки на docs                 | 7.1         |

**Gate:** Пользователь за 5 минут поднимает сервер через `docker compose up` и подключает клиента следуя `docs/en/quickstart.md`.

---

## Production Gap — состояние на 2026-05-14

**Текущий вердикт (2026-05-15):** базовая сборка, unit/race-проверки проходят, критические утечки ресурсов устранены. Продукт готов к **ограниченному production** (≤500 сессий, uptime >72ч).

---

## Технический долг — Post-Hardening (Sprint 3.5, 2–3 дня)

**Задачи, оставшиеся после `production-readiness-hardening`, не блокирующие MVP, но рекомендуемые перед v1.0-stable.**

| # | Приоритет | Задача | Результат | Зависимости |
| --- | --- | --- | --- | --- |
| 1 | P1 | `sm.Stop()` — sync.Once для защиты от panic при двойном вызове | `close(sm.stopCh)` безопасен при повторном вызове | production-readiness-hardening |
| 2 | P1 | Session expiry → cancel WS goroutines: per-session context.CancelFunc | При idle/ttl expiry WS горутины завершаются немедленно, а не через 30s deadline | session expiry |
| 3 | P1 | Origin checker: заменить `path.Match` на корректный glob/pattern matcher | `*.example.com` не отвергается из-за `/` в URL | 4.4 |
| 4 | P1 | Информационная безопасность: общие сообщения об ошибках вместо "invalid token"/"max sessions exceeded" | Error response не раскрывает внутреннее состояние | auth |
| 5 | P2 | `SetWriteLimit` для WebSocket — ограничить буфер записи | Защита от OOM при медленных читателях | 1.2 |
| 6 | P2 | Worker pool / semaphore для proxy goroutines | 1000 сессий × 100 proxy streams не создаёт 100k горутин | local-proxy-mode |
| 7 | P2 | Rate limiting для `/metrics` endpoint | Prometheus endpoint не может быть использован для DoS | metrics |
| 8 | P3 | `TokenBandwidthManager.Allow` — устранить race condition lock-unlock-lock | Rate limiter точен при конкурентном доступе | 4.2 |
| 9 | P3 | Proxy goroutine context-awareness | TCP read горутина завершается по ctx.Done(), а не только по CloseAll() | local-proxy-mode |
| 10 | P3 | Prometheus latency histograms (p50/p95/p99) | Метрики пригодны для алертинга по задержкам | 3.7 |
| 11 | P3 | Runtime log level change через SIGHUP или admin API | Уровень логов меняется без перезапуска | logging |
| 12 | P3 | `sessionProxyStreams` вынести в отдельный пакет `proxy/streams.go` | Переиспользуемый, тестируемый тип | local-proxy-mode |

### Рекомендуемый порядок

1. sync.Once для sm.Stop() + per-session cancel — закрывают последствия таймаутов.
2. Origin checker bugfix — potential security issue.
3. Общие ошибки auth — защита от перебора.
4. Остальные P2/P3 — по мере необходимости перед v1.0-stable.

---

### Блокер 1 — TLS trust chain и проверка сервера

| Приоритет | Задача | Результат | Зависимости |
| --- | --- | --- | --- |
| P0 | Убрать hardcoded `InsecureSkipVerify=true` из client runtime | Клиент валидирует сертификат сервера по CA/SAN | 1.3 |
| P0 | Добавить в client config явные поля CA / SNI / verify mode | Production-конфиг не требует insecure fallback | 1.3 |
| P0 | Добавить e2e-тест на отказ при недоверенном сертификате | Есть observable proof TLS verification | задача выше |

**Gate:** клиент успешно подключается только к доверенному сертификату; MITM с self-signed cert без CA trust отвергается.

### Блокер 2 — Исправление mTLS semantics

| Приоритет | Задача | Результат | Зависимости |
| --- | --- | --- | --- |
| P0 | Заменить `RequireAnyClientCert` на корректный verify-режим для production | `client_auth=require` реально проверяет client cert по CA | 4.6 |
| P0 | Развести режимы `request` / `require` / `verify` без двусмысленности | Поведение mTLS явно описано и предсказуемо | задача выше |
| P1 | Добавить тесты на reject unknown client cert | Есть proof, что mTLS не фиктивный | задача выше |

**Gate:** сервер отклоняет клиентский сертификат, отсутствующий в доверенном `client_ca_file`.

### Блокер 3 — Secrets hygiene и безопасные примеры

| Приоритет | Задача | Результат | Зависимости |
| --- | --- | --- | --- |
| P0 | Удалить `key.pem` и другие приватные ключи из git-истории и рабочего tree examples | Репозиторий не содержит production-like secret material | 7.3 |
| P0 | Перевести examples/compose на runtime-generated certs или documented mount-from-secret | Примеры безопасны по умолчанию | задача выше |
| P1 | Добавить `.gitignore`/secret-scan guardrails | Повторное коммитирование ключей предотвращается | задача выше |

**Gate:** в `git ls-files` отсутствуют приватные ключи; compose-примеры не зависят от закоммиченных секретов.

### Блокер 4 — Release governance по SpecKeep

| Приоритет | Задача | Результат | Зависимости |
| --- | --- | --- | --- |
| P0 | Привести active spec root к ожидаемому `.speckeep` формату | `check-verify-ready.sh` больше не падает на missing `spec.md` / `tasks.md` | process |
| P0 | Сформировать verify artifact с observable proof по release-критериям | Production release имеет формально подтверждённый gate | задача выше |
| P1 | Согласовать roadmap/spec/tasks для production hardening slug | Нет рассинхрона между кодом и процессом | задача выше |

**Gate:** `./.speckeep/scripts/check-verify-ready.sh .` проходит без ошибок.

### Блокер 5 — Operational hardening перед первым релизом

| Приоритет | Задача | Результат | Зависимости |
| --- | --- | --- | --- |
| P1 | Ограничить exposure `/metrics` и admin API | Операционные endpoints не открыты миру без контроля доступа | 3.7, 4.5 |
| P1 | Прогнать privileged e2e smoke для TUN + NAT + reconnect | Есть доказательство работы в реальном runtime, а не только unit tests | 1.7, 2.7, 3.1 |
| P1 | Прогнать `golangci-lint` и зафиксировать результат в verify | Выполнены конституционные quality gates | process |

**Gate:** privileged smoke-test проходит, operational endpoints имеют осознанную модель доступа.

### Рекомендуемый порядок закрытия

1. TLS verification в клиенте.
2. Корректная mTLS-проверка на сервере.
3. Удаление секретов из репозитория и безопасные примеры.
4. Восстановление SpecKeep verify path.
5. Privileged e2e smoke + lint + финальный verify.

---

## Таблица приоритетов (как быстрее в production)

| Уровень          | Этапы         | Что даёт                                        |
| ---------------- | ------------- | ----------------------------------------------- |
| 🔴 Critical Path | 0 → 1 → 2 → 3 | **Работающий production-готовый VPN с routing** |
| 🟡 Important     | 4 (security)  | Защита от злоупотреблений                       |
| 🟢 Nice-to-have  | 5, 6          | IPv6 + performance                              |
| 📦 Release       | 7             | Документация и релизный процесс                 |

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

_RoadMap утверждён: 2026-05-13._
_Следующий шаг: `/speckeep.spec core-tunnel` — спецификация Этапа 1._
