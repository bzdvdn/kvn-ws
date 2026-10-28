# Конституция проекта KVN 2.0 (Go core + Android)

## Назначение

KVN 2.0 — v2-only VPN для защиты трафика в сетях с ограничениями: TLS 1.3 + HTTP/2 для TCP, опциональный QUIC для UDP, TUN-режим, локальный proxy, browser-like uTLS/HTTP2 surface и fallback. Клиенты: Go CLI, `kvn-web`/WebView desktop и Android на общем Go mobile core.

## Ключевые принципы

### I. v2-only и дисциплина scope
- Текущее дерево и релиз 2.0.0 содержат только v2 runtime; v1 доступен только в старых GitHub Releases.
- Реализация идёт по активным spec/plan/tasks и не расширяет scope без явного решения.
- Публичное поведение сначала отражается в spec/tasks, затем в коде.

### II. Архитектурные границы
- `src/cmd/{server,client,web,desktop}` — entrypoints; `src/internal/*` — Go-пакеты ядра; `src/mobile` — gomobile AAR boundary; `src/android` — Kotlin-приложение.
- Go-пакеты группируются по ответственности (transport, tcpstack, tlsprofile, tunpool, client, server, config, routing, webui); циклические зависимости запрещены.
- Внешние зависимости изолируются за интерфейсами; глобальное мутабельное состояние запрещено, DI через конструкторы.
- Android использует Go core через gomobile; Kotlin `VpnService` владеет TUN fd и lifecycle, Go core не закрывает чужой fd.
- `kvn-desktop` — WebView-обёртка `kvn-web`; Windows installer по умолчанию ставит desktop-клиент.

### III. Traceability (NON-NEGOTIABLE)
- Каждая нетривиальная задача имеет `@sk-task` в коде и `@sk-test` в тестах.
- Маркеры размещаются на owning symbol (func/type/test/class), не на `package`/`import`/file-header.
- Завершённая задача в `tasks.md` обязана иметь `Proof: <kind> <path> [anchor]`.

### IV. Безопасность и честность по DPI
- Транспорт: TLS 1.3 + ALPN `h2`; uTLS-профиль Chrome по умолчанию, Firefox/Safari доступны; при ошибке uTLS — fallback на стандартный TLS.
- По умолчанию нет KVN-специфичных путей/заголовков; неизвестные запросы получают fallback-сайт.
- `verify_mode=insecure` отключает только проверку сертификата; он не должен быть default и не даёт гарантии обхода DPI.
- Секреты и токены не логируются; QR/JSON с токеном считается чувствительным конфигом.
- Заявлять «обходит DPI» можно только после полевого прогона; без него — только «нет явных отпечатков».

### V. Verify перед Archive
- Observable proof: существующий файл, вывод теста или команды.
- Go: `go test ./src/...`, `go vet ./src/...`, `gosec ./...`, `golangci-lint run ./...`.
- Android: `./gradlew testDebugUnitTest`, `./gradlew assembleDebug`.
- DPI: `scripts/dpi-check.sh`; lab-прогон обязателен, полевой — релизный гейт.

## Непересматриваемые правила

- Правила этого раздела имеют статус `MUST` / `MUST NOT` и проверяются на практике.
- v1 runtime, v1 entrypoints и v1-бинари `MUST NOT` попадать в текущую сборку.
- Единый конфиг `MUST` храниться в `~/.config/kvn/config.yaml`; v1-файл сохраняется как `config.yaml.v1.bak` и не перезаписывается.
- v2 `MUST` использовать TLS 1.3/HTTP2 и не оставлять KVN-маркеры в default surface.
- Завершённая задача `MUST` иметь `Proof:`; без proof задача не закрыта.
- Изменения публичного поведения `MUST` отражаться в spec/tasks до merge.
- При конфликте реализации с конституцией сначала обновляется конституция.

## Ограничения

- Без iOS-клиента, full mesh и возврата v1 relay/runtime.
- Go — ядро/сервер; Kotlin — Android. Cgo допускается для TUN и desktop WebView.
- Android: minSdk 26, targetSdk 35.
- Windows: WebView2 desktop по умолчанию; browser-режим — только явный `-Browser`.
- Один writer на сокет; lifecycle и отмена должны быть безопасными.
- DPI-обход не гарантируется для любого вендора/страны.

## Технологический стек

- **Go 1.26**: `net/http` HTTP/2, `quic-go` (optional UDP), `refraction-networking/utls`, `viper`, `zap`, gVisor TCP stack, `wireguard/tun`, `webview_go`, `gomobile`.
- **Android**: Kotlin 1.9.20, Jetpack Compose + Material 3, Coroutines/Flow, DataStore, CameraX, ML Kit Barcode, ZXing.
- **Конфигурация**: YAML + env override (Go), DataStore (Android), unified `~/.config/kvn/config.yaml`.
- **Поставка**: Docker multi-stage для Go; APK/AAB для Android; PowerShell installer для Windows.

## Основная архитектура

```
[Android Kotlin VpnService] <-> [gomobile Go core] <-> [TLS 1.3 / HTTP2, QUIC optional] <-> [Go server] <-> [TUN/NAT] <-> Internet
[Go client / kvn-web / kvn-desktop] <--------------------------------------------------------^
```

```
src/
├── cmd/{server,client,web,desktop}   # entrypoints
├── internal/
│   ├── transport/                    # HTTP2 streams, sessions, padding, ICMP, QUIC
│   ├── tlsprofile/                   # uTLS profiles + fallback
│   ├── tcpstack/                     # gVisor TCP split
│   ├── tunpool/                      # unique TUN leases + persistence
│   ├── client/                       # TUN/proxy data plane, metrics, direct routing
│   ├── server/                       # fallback site
│   ├── config/                       # unified config + v2 schema
│   ├── routing/, dnsproxy/, proxy/   # split routing, DNS, local proxy
│   ├── tun/, webui/, logger/
├── mobile/                           # gomobile AAR boundary
└── android/                          # Kotlin app
configs/  scripts/  docs/{ru,en}/  .github/workflows/
```

Сервер выдаёт уникальный TUN lease на `client_id`; lease имеет TTL и сохраняется в JSON, чтобы переживать restart.

## Языковая политика

- Язык документации: русский (`docs/ru/`) и английский (`docs/en/`)
- Язык общения с агентом: русский
- Язык комментариев в коде: английский

## Процесс разработки

- Каждая фича разрабатывается в отдельной ветке `feature/<slug>`; ветку создаёт `/spk-spec` или `/spk-propose`.
- Реализация начинается с явной spec; plan/tasks выводятся из неё и остаются согласованными.
- Изменения публичного поведения отражаются в spec/tasks до merge.
- При конфликте с конституцией сначала обновляется конституция.

## Definition of Done

- Задача завершена только при observable proof: изменённые файлы, вывод тестов или команды.
- Trace-маркеры обязательны; при нескольких тестах одной задачи `@sk-test` стоит на каждом.
- `Proof:` в `tasks.md` обязателен для каждой `[x]`; отсутствующий файл — ошибка, отсутствующий anchor — предупреждение.
- Go-проверки: `go test ./src/...`, `go vet ./src/...`, `gosec ./...`, `golangci-lint run ./...`.
- Android: `./gradlew testDebugUnitTest` и `./gradlew assembleDebug`.
- DPI lab-прогон и отчёт обязательны; полевой прогон — до релиза.
- Перед archive подтверждается покрытие acceptance criteria.

## Политика Repository Map

- `REPOSITORY_MAP.md` — компактный индекс навигации, обновляется in-place минимальным diff.
- Исключены `.speckeep/`, `specs/`, `vendor/`, `node_modules/`, `dist/`, `bin/`.
- Карта обновляется при изменении entrypoints, модулей или границ подсистем.

## Управление

- Эта конституция — авторитетный источник проектных решений.
- Изменения архитектуры, spec, plan и tasks должны соответствовать конституции.
- При конфликте приоритет у конституции, пока она явно не изменена.
- Изменения вносятся patch-обновлениями с сохранением обязательных секций.

## Метаданные конституции

- Version: 2.0.0
- Ratified: 2026-05-13
- Last Amended: 2026-10-29

## Последнее обновление

2026-10-29 — переход на v2-only: TLS 1.3/HTTP2, uTLS, TUN pool, gomobile, WebView desktop, Windows installer и DPI-политика
2026-06-18 — обязательные проверки go vet, gosec, golangci-lint
2026-06-11 — добавлен Android-клиент (Kotlin), обновлён стек и архитектура
