# Production Hardening

## Scope Snapshot

- In scope: клиентский reconnect/keepalive/kill-switch, серверный rate limiting/session expiry/IP persistence, observability (Prometheus + health + audit), operational (config reload, CLI flags).
- Out of scope: горизонтальное масштабирование, распределённый пул IP, GUI/TUI, BGP-пиринг.

## Цель

Оператор получает production-ready VPN-сервис: клиент не теряет соединение без переподключения, сервер не даёт себя забрутфорсить, брошенные сессии освобождают IP, метрики и health endpoint позволяют мониторить сервис, а конфиг можно менять без перезапуска. Успех измеряется 24h стабильной работы с активным трафиком без падений.

## Основной сценарий

1. Клиент подключается к серверу, при обрыве — переподключается с exponential backoff + jitter.
2. PING/PONG фреймы обмениваются каждые N секунд; при таймауте >30s соединение рвётся.
3. При падении соединения kill-switch блокирует нефайловый трафик (route leakage).
4. Сервер лимитирует auth-попытки (rate limiter) и пакеты на сессию.
5. Idle-сессии (>таймаут) и сессии с истёкшим TTL удаляются; IP возвращается в пул.
6. IP-пул персистентен через BoltDB — после рестарта сервера IP не теряются.
7. `/metrics` (Prometheus) и `/health` (liveness + readiness) доступны на сервере.
8. SIGHUP перезагружает конфиг (лимиты, токены) без остановки сервиса.
9. Все ошибки auth/ACL логируются структурированно.
10. CLI flags (`--config`, `KVN_SERVER_ADDR`) переопределяют YAML.

## User Stories

- P1 Администратор настраивает мониторинг Prometheus + health check в Kubernetes — видит active sessions, throughput, error rate.
- P2 Клиент теряет Wi-Fi на 10 секунд — reconnect срабатывает, трафик не утекает (kill-switch).
- P3 Атакующий шлёт 1000 auth-запросов в секунду — rate limiter блокирует после 5 попыток с одного IP.

## MVP Slice

Server-side: rate limiting + session expiry + health endpoint. Клиент успешно подключается и отключается без утечек. AC-004, AC-005, AC-008.

## First Deployable Outcome

Сервер с rate limiter + session expiry + Prometheus /metrics — можно запустить в staging, увидеть метрики, убедиться что брошенные сессии освобождают IP.

## Scope

- `src/internal/protocol/control/` — PING/PONG, keepalive
- `src/internal/session/` — expiry, reclaim, BoltDB persistence
- `src/internal/metrics/` — Prometheus counters/gauges/histograms
- `src/internal/config/` — graceful reload (SIGHUP), CLI flags + env
- `src/internal/transport/websocket/` — reconnect, kill-switch
- `src/internal/transport/tls/` — (minimal, если нужно)
- `src/cmd/server/main.go` — health endpoint, /metrics, SIGHUP handler
- `src/cmd/client/main.go` — reconnect, kill-switch, CLI flags

## Контекст

- BoltDB уже есть в go.sum как транзитивная зависимость? Нет, нужно добавить `go.etcd.io/bbolt`.
- Prometheus client уже есть? Нет, нужно добавить `github.com/prometheus/client_golang`.
- Session manager уже реализован с IP pool (core-tunnel-mvp).
- WebSocket transport уже реализован с dial/accept.
- Config loader (viper) уже поддерживает env override.

## Требования

- RQ-001 Клиент ДОЛЖЕН автоматически переподключаться при обрыве с exponential backoff (min 1s, max 30s) + jitter.
- RQ-002 Протокол ДОЛЖЕН поддерживать PING/PONG фреймы с таймаутом детекции <30s.
- RQ-003 Клиент ДОЛЖЕН блокировать нефайловый трафик (kill-switch) при потере соединения с сервером.
- RQ-004 Сервер ДОЛЖЕН лимитировать auth-попытки (rate limiter, 5/минута/IP) и пакеты на сессию.
- RQ-005 Сервер ДОЛЖЕН удалять idle-сессии (>таймаут) и сессии с истёкшим TTL, возвращая IP в пул.
- RQ-006 IP-пул ДОЛЖЕН сохраняться в BoltDB и восстанавливаться после рестарта сервера.
- RQ-007 Сервер ДОЛЖЕН экспортировать Prometheus метрики на /metrics (active sessions, throughput, errors).
- RQ-008 Сервер ДОЛЖЕН отвечать на GET /health (200 OK для liveness, дополнительные проверки для readiness).
- RQ-009 Сервер ДОЛЖЕН перезагружать конфиг по SIGHUP без остановки (лимиты, токены).
- RQ-010 Каждый auth/ACL failure ДОЛЖЕН логироваться структурированно (zap, JSON, с session ID и reason).
- RQ-011 CLI flags и env-переменные ДОЛЖНЫ переопределять YAML-конфиг.

## Вне scope

- Горизонтальное масштабирование сервера
- Распределённый IP pool (несколько инстансов)
- Web dashboard / GUI / TUI
- BGP-пиринг / anycast
- Dynamic DNS для клиента
- Windows Service / systemd integration

## Критерии приемки

### AC-001 Auto-reconnect с exponential backoff + jitter

- Почему это важно: при обрыве сети клиент восстанавливает соединение без ручного вмешательства.
- **Given** подключённый клиент
- **When** соединение с сервером обрывается (например, kill серверного процесса)
- **Then** клиент переподключается с exponential backoff: 1s → 2s → 4s → ... → 30s max, с random jitter ±500ms
- **When** сервер снова доступен
- **Then** клиент успешно подключается и возобновляет работу
- Evidence: лог клиента показывает `reconnect attempt N in Xs`, затем `connected`

### AC-002 Keepalive PING/PONG, детекция <30s

- Почему это важно: dead соединения детектируются и не висят вечно.
- **Given** активное клиент-сервер соединение
- **When** прошло N секунд без трафика
- **Then** клиент шлёт PING, сервер отвечает PONG
- **When** сервер не отвечает на PING в течение 30s
- **Then** клиент закрывает соединение и инициирует reconnect
- Evidence: лог показывает `ping timeout 30s, closing connection`

### AC-003 Kill-switch (route leakage protection)

- Почему это важно: при падении VPN трафик не должен утекать в открытый интернет.
- **Given** клиент с default_route: server (весь трафик через туннель)
- **When** соединение с сервером теряется
- **Then** клиент блокирует нефайловый трафик (добавляет reject-правило или отключает default route через TUN)
- **When** соединение восстановлено
- **Then** блокировка снимается, трафик снова идёт через туннель
- Evidence: `ping 8.8.8.8` не проходит при отключённом туннеле (в full-tunnel режиме)

### AC-004 Rate limiting (auth + packets)

- Почему это важно: защита от brute-force и abuse.
- **Given** атакующий IP шлёт auth-запросы
- **When** частота превышает 5 попыток в минуту
- **Then** сервер возвращает 429 Too Many Requests
- **When** сессия шлёт >1000 пакетов/сек
- **Then** сервер дропает избыточные пакеты
- Evidence: `curl -X POST https://server/ws -H "Authorization: Bearer wrong"` × 6 → последний возвращает 429

### AC-005 Session expiry + reclaim

- Почему это важно: брошенные сессии не блокируют IP-адреса.
- **Given** подключённая сессия
- **When** idle > timeout (по умолчанию 300s) или TTL истёк (по умолчанию 24h)
- **Then** сервер удаляет сессию и возвращает IP в пул
- **When** новый клиент запрашивает IP
- **Then** освобождённый IP может быть выделен снова
- Evidence: `TestSessionExpiryReclaimsIP` — создать сессию, дождаться expiry, проверить что IP доступен для нового выделения

### AC-006 IP Pool persistence (BoltDB)

- Почему это важно: после рестарта сервера IP-пул не теряется.
- **Given** сервер с активными сессиями
- **When** сервер перезапускается
- **Then** после рестарта allocated IP восстанавливаются из BoltDB
- **When** новый клиент подключается
- **Then** ему не выдаётся уже занятый IP
- Evidence: `TestPoolPersistence` — записать состояние, пересоздать pool из BoltDB, проверить allocated IP

### AC-007 Prometheus /metrics

- Почему это важно: мониторинг активных сессий, throughput, ошибок.
- **Given** запущенный сервер
- **When** `GET /metrics`
- **Then** возвращаются Prometheus-совместимые метрики: `kvn_active_sessions`, `kvn_throughput_bytes_total`, `kvn_errors_total` с labels (type, session_id)
- Evidence: `curl http://server:9090/metrics` возвращает метрики в текстовом формате

### AC-008 Health endpoint

- Почему это важно: Kubernetes/orchestration проверяет живость сервера.
- **Given** запущенный сервер
- **When** `GET /health`
- **Then** возвращается 200 OK с `{"status": "ok"}` (liveness)
- **When** сервер не готов принимать соединения (например, инициализация)
- **Then** readiness возвращает 503
- Evidence: `curl http://server:8080/health` возвращает `{"status":"ok"}`

### AC-009 Graceful config reload (SIGHUP)

- Почему это важно: смена лимитов/токенов без перезапуска.
- **Given** запущенный сервер с конфигом
- **When** администратор шлёт SIGHUP
- **Then** сервер перезагружает конфиг (лимиты rate limiter, токены auth) без разрыва активных сессий
- **When** конфиг содержит ошибку
- **Then** сервер логирует ошибку и продолжает работать со старым конфигом
- Evidence: изменить токен в конфиге → `kill -HUP <pid>` → новый клиент подключается с новым токеном, старые сессии не рвутся

### AC-010 Structured error handling + audit trail

- Почему это важно: каждый auth/ACL failure можно追踪ровать.
- **Given** сервер с JSON-логированием (zap)
- **When** клиент шлёт неверный токен
- **Then** лог содержит: `{"level":"warn","time":"...","msg":"auth failed","session_id":"...","reason":"invalid token","remote_addr":"..."}`
- **When** rate limiter срабатывает
- **Then** лог содержит аналогичную структуру с `reason`
- Evidence: `grep auth.log -c '"auth failed"'` после нескольких неудачных попыток

### AC-011 CLI flags + env override

- Почему это важно: гибкость в Docker/Kubernetes окружениях.
- **Given** бинарный файл сервера
- **When** `server --config /etc/kvn-ws/server.yaml --listen :8443`
- **Then** `--listen` переопределяет `listen` из YAML
- **When** `KVN_SERVER_LISTEN=:9443 server --config /etc/kvn-ws/server.yaml`
- **Then** env-переменная переопределяет CLI flag
- Evidence: `server --help` показывает все флаги; `server --config test.yaml --listen :9090` слушает на :9090

### AC-012 24h stability gate

- Почему это важно: финальное доказательство production-readiness.
- **Given** кластер клиент-сервер с активным трафиком (iperf/curl loop)
- **When** система работает 24 часа
- **Then** нет падений процесса, утечек памяти (RSS стабилен), нет увеличения времени ответа
- Evidence: `uptime` процесса = 24h, `go test -race ./...` проходит, `pprof` не показывает утечек

## Допущения

- BoltDB добавляется как зависимость (`go.etcd.io/bbolt v1.4.x`)
- Prometheus client_golang добавляется как зависимость
- Клиент и сервер работают на Linux (kill-switch через iptables/nftables или TUN level)
- Rate limiter in-memory (не cluster-aware)
- SIGHUP доступен на Linux (контейнеры и systemd поддерживают)
- Config reload не меняет listening socket (listen address только при старте)

## Критерии успеха

- AC-001–AC-012 проходят в CI (unit + integration).
- 24h stability gate (AC-012) пройден в staging.
- Memory: RSS стабилен в течение 24h (±5%).
- Rate limiter: <1ms overhead на запрос в нормальном режиме.
- BoltDB: <10ms на загрузку пула из 1000 записей.
- Health endpoint: <1ms latency, p99.

## Краевые случаи

- Rate limiter: burst из 5 запросов разрешён, 6-й — 429
- Kill-switch при первом подключении (нет сессии — трафик блокирован до соединения)
- SIGHUP с битым конфигом — старый конфиг остаётся активным
- BoltDB: файл БД повреждён — сервер стартует с пустым пулом (graceful fallback)
- Prometheus: /metrics вызван до первой сессии — пустые/нулевые метрики
- Health: readiness fails если pool не инициализирован
- Reconnect: сервер постоянно падает — клиент не уходит в бесконечный loop (max retries или backoff stops)

## Открытые вопросы

- Какое максимальное число retry для auto-reconnect?
  - Бесконечно с capped exponential backoff (1s–30s). Админ может остановить процесс.
- Какой default для rate limiter: 5/min для auth, 1000/s для пакетов?
  - Да, конфигурируемо через YAML.
- BoltDB путь — дефолтный `/var/lib/kvn-ws/ip-pool.db`, переопределяемо.
- Kill-switch на macOS/Windows?
  - Linux-first. На macOS — pfctl. На Windows — Windows Filtering Platform. Отложено.
- Prometheus port — отдельный или встроенный?
  - Встроенный HTTP сервер (mux на listen port или отдельный `--metrics-addr`).
- SIGHUP на Windows?
  - Не поддерживается (Windows-first в отдельной задаче).
