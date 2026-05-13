# Constitution Summary — kvn-ws (github.com/bzdvdn/kvn-ws)

**Purpose:** Производительный VPN-туннель через HTTPS/WebSocket + TUN на Go с маскировкой под обычный веб-трафик.

**Non-negotiables:**
- Весь код только на Go, без глобального мутабельного состояния
- DDD + Clean Architecture: domain не зависит от infrastructure
- Traceability: `@sk-task` в коде и `@sk-test` в тестах обязательны
- Разделение документации: `docs/ru/` и `docs/en/`
- Docker multi-stage build — основной способ поставки

**Stack/Architecture:**
- Go 1.22+, gorilla/websocket, viper, zap, prometheus, wireguard/tun
- `src/` — весь код, `docs/{ru,en}/` — двуязычная документация, `examples/` — примеры
- TCP 443 + TLS 1.3 + WebSocket Binary Frames
- IP-пул с динамическим выделением, BoltDB/SQLite persistence
- Клиентская маршрутизация: server/direct, CIDR, DNS-имена, отдельные IP с ordered rules

**Workflow/DoD:**
- Каждая фича в отдельной ветке `feature/<slug>`
- DoD: observable proof (файлы, тесты, команда)
- Trace-маркеры на объявлениях функций/методов/структур
- `go test ./...` + race detector + golangci-lint перед archive
- Docker-сборка и smoke-test обязателен

**Repo Map Policy:**
- `REPOSITORY_MAP.md` — компактный индекс, обновляется in-place
- Исключены: `.speckeep/`, `specs/archived/`, `vendor/`, `node_modules/`

**Languages:** docs=ru/en, agent=ru, comments=en
