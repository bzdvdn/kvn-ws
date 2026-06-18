# Конституция проекта KVN-over-WS (Go) + Android клиент

## Назначение

Создать производительный, безопасный и расширяемый VPN-туннель, использующий стандартный веб-трафик (HTTP/HTTPS + WebSocket) и QUIC как транспортный слой для передачи IP/IPv4/IPv6 пакетов между клиентом и сервером через TUN-интерфейсы. Маскировка под обычный HTTPS/WebSocket-трафик или QUIC с обфускацией для работы в сетях с ограничениями. Нативный Android-клиент с Jetpack Compose UI для подключения с мобильных устройств.

## Ключевые принципы

### I. Domain-Driven Design и Clean Architecture
<!-- DDD: чёткие bounded contexts, Ubiquitous Language в коде -->

- **Go (ядро/сервер):** Код организуется по доменным контекстам (tun, transport, protocol, routing, session, crypto), а не по техническим слоям. Внешние зависимости изолируются за портами (Go interfaces). Ядро не зависит от инфраструктуры. Слои: `domain` → `usecase` → `infrastructure`.
- **Android (клиент):** MVVM + Jetpack Compose. UI-слой отделён от бизнес-логики. ViewModel управляет состоянием, доменная логика — в repository/use case классах.
- `src/internal/` — только неэкспортируемые пакеты Go. `src/android/` — независимый Gradle-модуль.

### II. Границы архитектуры
<!-- Модульность, разделение ответственности -->

- `src/` — исходный код: `src/cmd/`, `src/internal/`, `src/pkg/` (Go), `src/android/` (Kotlin).
- `docs/` — документация `docs/ru/` и `docs/en/`.
- `examples/`, `configs/`, `scripts/` — без изменений.
- Каждый доменный пакет не имеет циклических зависимостей.
- Android-модуль изолирован от Go-кода; связь через JSON-сериализованные конфиги/QR.

### III. Traceability (NON-NEGOTIABLE)
<!-- Маркеры @sk-task / @sk-test для всех языков -->

- Каждая нетривиальная задача требует trace-маркеров в коде (`@sk-task`) и тестах (`@sk-test`).
- Маркеры размещаются на объявлениях функций/методов/структур/классов, не на `package`/`import`/file-header.
- Языковые правила размещения: Go (`//` над `func`/`type`/`Test`), Kotlin (`//` над `fun`/`class`/`@Test`), Shell (`#` над `function`/block).
- Изменения публичного поведения MUST отражаться в spec/tasks до merge.

### IV. Verify перед Archive
<!-- Проверка качества обязательна для обеих платформ -->

- Observable proof: изменённые файлы, вывод тестов, результат команды.
- Go: `go test -race ./...`, `go vet ./...`, `gosec ./...`, `golangci-lint run ./...`.
- Android: `./gradlew test`, `./gradlew assembleDebug`.
- Docker-сборка (Go) через `scripts/docker-build.sh`.

### V. Простота и эксплуатация
<!-- Docker, конфигурация, наблюдаемость -->

- Docker — основной способ поставки Go-сервера; multi-stage build.
- Конфигурация через YAML + env override (Go).
- Наблюдаемость: structured JSON logs (zap), Prometheus метрики, health endpoint.
- Android-клиент поставляется как APK; конфигурация через QR-код или ручной ввод.

## Непересматриваемые правила

- Правила этого раздела имеют статус `MUST` / `MUST NOT` и должны проверяться на практике.
- Реализация `MUST` идти по активным spec/plan/tasks и оставаться в заявленном scope.
- Работа `MUST NOT` продолжаться из неоднозначных требований или placeholder-контента.
- Изменения публичного поведения `MUST` отражаться в spec/tasks до merge.
- Если реализация конфликтует с конституцией, сначала обновляется конституция.
- Go — основной язык ядра и сервера. Kotlin — язык Android-клиента. Cgo допускается только для TUN (wireguard/tun).
- Никакого глобального мутабельного состояния. DI через конструкторы.
- Android-код следует официальным гайдлайнам: Material 3, Jetpack Compose, Coroutines + Flow.

## Ограничения

- MVP: без HTTP3, без iOS-клиента, без full mesh.
- Go-ядро: TCP 443 (TLS 1.3 + WebSocket Binary Frames) и/или UDP 443 (QUIC + TLS 1.3).
- Минимальный external dependency footprint — для обеих платформ.
- Один writer на сокет (горутин-безопасность).
- Android: minSdk 26, targetSdk 35. Только QR/ручной ввод конфига (без NFC/Cloud).

## Технологический стек

- **Языки:** Go 1.22+ (ядро/сервер), Kotlin 1.9+ (Android-клиент)
- **WebSocket:** `github.com/gorilla/websocket` или `nhooyr.io/websocket`
- **QUIC:** `github.com/quic-go/quic-go` v0.50
- **Конфигурация:** `spf13/viper` (Go), DataStore Preferences (Android)
- **Логирование:** `uber-go/zap` (Go), `android.util.Log` (Android)
- **Метрики:** `prometheus/client_golang`
- **TUN:** `golang.zx2c4.com/wireguard/tun` (Go), `VpnService` (Android)
- **Android UI:** Jetpack Compose, Material 3, CameraX, ML Kit Barcode Scanning, ZXing
- **Контейнеризация:** Docker (multi-stage build) для Go
- **БД сессий (persistence):** BoltDB / SQLite (встраиваемая)

## Основная архитектура

```
[Android Client (Kotlin)] <-> [TLS + WebSocket/QUIC] <-> [Go Server Core] <-> [TUN/Router/NAT] <-> Internet/LAN
         |
    [QR Config / Manual Input]
```

Структура репозитория:

```
kvn-ws/            # github.com/bzdvdn/kvn-ws
├── src/
│   ├── cmd/                # точки входа
│   │   ├── client/         # Go-клиент (CLI)
│   │   ├── server/         # сервер
│   │   ├── relay/          # relay-терминатор
│   │   ├── web/            # web-ui сервер
│   │   ├── gatetest/       # интеграционные тесты
│   │   └── stability/      # stability test tool
│   ├── internal/           # Go-пакеты (ядро)
│   │   ├── acl/            # access control lists
│   │   ├── admin/          # admin API
│   │   ├── bootstrap/
│   │   │   ├── client/     # клиентский bootstrap (dial, proxy, killswitch, tun, reconnect)
│   │   │   ├── relay/      # relay bootstrap (bridge, handler, nat, router, upstream)
│   │   │   └── server/     # серверный bootstrap (handler)
│   │   ├── config/         # парсинг YAML + env (client, server)
│   │   ├── crypto/         # доп. шифрование (app-layer)
│   │   ├── dns/            # DNS resolver + cache
│   │   ├── dnsproxy/       # DNS proxy (встроенный DNS-сервер)
│   │   ├── logger/         # zap-логгер
│   │   ├── metrics/        # Prometheus метрики
│   │   ├── nat/            # MASQUERADE/SNAT + nftables
│   │   ├── protocol/
│   │   │   ├── auth/       # token, jwt, basic
│   │   │   ├── control/    # PING, CLOSE, ROUTE_UPDATE
│   │   │   └── handshake/  # Client/Server Hello
│   │   ├── proxy/          # transparent proxy listener
│   │   ├── ratelimit/      # rate limiting
│   │   ├── routing/        # маршрутизация (matcher, dns, domain, rule_set)
│   │   ├── session/        # менеджмент сессий + IP pool + BoltDB
│   │   ├── systemproxy/    # system proxy (linux/darwin/windows)
│   │   ├── transparent/    # iptables redirect
│   │   ├── transport/
│   │   │   ├── framing/    # бинарный фрейм-протокол
│   │   │   ├── quic/       # QUIC dial/listen + обфускация
│   │   │   ├── tls/        # TLS-конфиг
│   │   │   └── websocket/  # WS dial/accept
│   │   ├── tun/            # TUN device abstraction
│   │   ├── tunnel/         # session/stream/demux
│   │   └── webui/          # web-ui backend (Go) + frontend (React/TypeScript)
│   ├── integration/        # integration tests
│   └── android/            # Android-клиент (Kotlin, Gradle)
│       └── app/
│           ├── src/main/kotlin/com/kvn/client/
│           │   ├── config/      # ConnectionConfig, парсинг QR
│           │   ├── transport/   # WebSocket-клиент (OkHttp)
│           │   ├── vpn/         # VpnService, TUN fd
│           │   └── ui/          # Compose UI (экраны, ViewModel)
│           └── build.gradle.kts
├── protocol/
│   ├── frames.yaml         # спецификация фреймов
│   ├── handshake.yaml      # спецификация handshake
│   └── codegen/            # генератор типов по YAML-схемам
├── docs/
│   ├── ru/
│   └── en/
├── examples/
│   └── relay-terminator/   # пример конфигов для relay
├── configs/                # примеры конфигов
├── scripts/                # сборочные и установочные скрипты
├── certs/                  # тестовые TLS-сертификаты
├── relay/                  # runtime-данные relay (runtime)
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── go.sum
```

Формат кадра:

```
+--------+--------+--------+-------------------+
| Type   | Flags  | Length | Payload           |
+--------+--------+--------+-------------------+
| 1 byte | 1 byte | 2/4 b  | Variable          |
```

## Клиентская маршрутизация (Routing Policy)

Клиент MUST поддерживать гибкую настройку направления трафика — через VPN-сервер или напрямую (bypass). Политика задаётся в конфигурации:

- **default_route** — поведение по умолчанию: `server` (весь трафик через туннель) или `direct` (весь трафик мимо туннеля).
- **include_ranges** — CIDR-диапазоны через сервер.
- **exclude_ranges** — CIDR-диапазоны напрямую.
- **include_domains** — DNS-имена через сервер (встроенный DNS-resolver).
- **exclude_domains** — DNS-имена напрямую.
- **include_ips** — отдельные IP через сервер.
- **exclude_ips** — отдельные IP напрямую.

Правила применяются в порядке: `exclude_ips` → `include_ips` → `exclude_domains` → `include_domains` → `exclude_ranges` → `include_ranges` → `default_route`. Первое совпадение останавливает перебор.

Для разрешения доменов в IP используется локальный DNS-resolver, кеширующий результаты с TTL. При недоступности DNS-резолва политика SHOULD применять exclude для данного домена (fail-closed для сервера, fail-open для direct).

## Языковая политика

- Язык документации: русский (`docs/ru/`), английский (`docs/en/`)
- Язык общения с агентом: русский
- Язык комментариев в коде: английский (Go-код и Android/Kotlin код)

## Процесс разработки

- Каждая фича ДОЛЖНА разрабатываться в отдельной git-ветке.
- Именование веток SHOULD следовать принятому в проекте соглашению для feature-веток, например `feature/<slug>`.
- Реализация SHOULD начинаться с явной спецификации до начала кодинга.
- Планы и задачи SHOULD выводиться из актуальной спецификации и оставаться с ней согласованными.
- Реализация, спецификации, планы и задачи ДОЛЖНЫ соответствовать этой конституции.
- Если работа выявляет конфликт с этой конституцией, конституция ДОЛЖНА быть изменена до продолжения несовместимой реализации.

## Definition of Done

- Задача считается завершенной только при observable proof: измененные файлы, вывод целевых тестов или результат команды.
- Для нетривиальных правок обязательны traceability-маркеры:
  - код: `@sk-task <slug>#<TASK_ID>: <short> (<AC_ID>)`
  - тесты: `@sk-test <slug>#<TASK_ID>: <TestName> (<AC_ID>)`
  - если одну задачу подтверждают несколько тестов/кейсов, `@sk-test <slug>#<TASK_ID>` должен стоять на каждом таком тесте/кейсе, а не только на одном representative тесте.
- Правило размещения маркеров:
  - размещайте trace-маркеры на объявлениях функций/методов/структур/классов (или на заголовках поведенческих блоков), а не на строках полей.
  - запрещено ставить trace-маркеры на уровень `package`, `import` или file-header comment; маркер должен принадлежать конкретному owning symbol.
  - если язык поддерживает именованные объявления, ставьте маркер непосредственно над тем объявлением, которое реализует или проверяет поведение.
- Примеры размещения и стиля по языкам:
  - Go: `//` непосредственно над `func`, method receiver, `type`, `func Test...`; если несколько `Test...` проверяют одну задачу, `@sk-test` нужен на каждом таком тесте.
  - Kotlin: `//` непосредственно над `fun`, `class`, `@Test fun`; не над `package`, `import`, `class` header без owning behavior.
  - Java: `//` или `/* */` непосредственно над `class`, `interface`, `enum`, method, JUnit test method; не над `package`, `import`, полями.
  - Shell / Bash: `#` над `function name()` или первой строкой именованного behavior/test block.
- Существующие trace-маркеры сохраняются; покрытие новой задачи добавляется доп. маркерами (без перезаписи).
- Если один метод/тест покрывает несколько задач, на нем одновременно остаются несколько маркеров.
- Перед archive в verify должна быть подтверждена покрываемость acceptance criteria.
- После завершения каждой spec обязателен прогон: `go test -race ./...`, `go vet ./...`, `gosec ./...`, `golangci-lint run ./...` (Go); для Android — `./gradlew test`.

## Политика Repository Map

- `REPOSITORY_MAP.md` — компактный индекс навигации по коду, а не процессный документ.
- Карта обновляется только при существенном изменении структуры/навигации кода.
- Обновление карты выполняется in-place с минимальным diff; неизменные секции не переписываются.
- Операционные/spec-артефакты исключаются из индексации согласно политике проекта.

## Управление

- Эта конституция является авторитетным источником для проектных решений.
- Изменения архитектуры, спецификаций, планов и задач ДОЛЖНЫ соответствовать этим принципам.
- Если реализация конфликтует с конституцией, приоритет у конституции, пока она явно не изменена.
- Изменяйте этот файл patch-обновлениями, сохраняя обязательные секции и делая правила конкретными и проверяемыми.

## Метаданные конституции

- Version: 1.1.1
- Ratified: 2026-05-13
- Last Amended: 2026-06-18

## Последнее обновление

2026-06-18 — добавлены обязательные проверки go vet, gosec, golangci-lint после каждой spec
2026-06-11 — добавлен Android-клиент (Kotlin), обновлён стек и архитектура
