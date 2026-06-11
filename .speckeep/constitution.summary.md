# Constitution Summary — kvn-ws (github.com/bzdvdn/kvn-ws)

**Purpose:** VPN-туннель HTTPS/WS + QUIC + TUN (Go) + Android-клиент (Kotlin, Jetpack Compose).

**Non-negotiables:**
- Go 1.22+ (ядро/сервер) и Kotlin 1.9+ (Android); Cgo только для TUN
- DDD + Clean Architecture (Go), MVVM + Compose (Android); никакого глобального мутабельного состояния
- Traceability: `@sk-task` в коде и `@sk-test` в тестах обязательны; запрещены на `package`/`import`/file-header
- Разделение документации: `docs/ru/` и `docs/en/`
- Docker multi-stage build — поставка Go-сервера; APK — поставка Android

**Stack/Architecture:**
- Go 1.22+ (ядро), Kotlin 1.9+ (Android, Gradle), gorilla/websocket, quic-go v0.50, viper, zap, prometheus, wireguard/tun
- Android: Jetpack Compose + Material 3, CameraX, ML Kit Barcode, ZXing, OkHttp WebSocket, DataStore
- `src/` — Go-код, `src/android/` — Android-модуль; `docs/{ru,en}/` — документация
- TCP 443 + TLS 1.3 + WS Binary Frames / UDP 443 + QUIC + TLS 1.3, IP-pool, BoltDB/SQLite

**Workflow/DoD:**
- Каждая фича в `feature/<slug>`; observable proof (файлы, тесты, команда)
- Trace-маркеры на объявлениях функций/методов/структур/классов
- Go: `go test ./...` + race + golangci-lint; Android: `./gradlew test` + `assembleDebug`
- Docker-сборка и smoke-test для Go; APK-сборка для Android

**Repo Map Policy:**
- `REPOSITORY_MAP.md` — компактный индекс, обновляется in-place
- Исключены: `.speckeep/`, `specs/archived/`, `vendor/`, `node_modules/`

**Languages:** docs=ru/en, agent=ru, comments=en
