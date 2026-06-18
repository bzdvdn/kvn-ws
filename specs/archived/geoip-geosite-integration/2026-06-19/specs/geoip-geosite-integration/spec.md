# GeoIP / GeoSite / External Sources для роутинга

## Scope Snapshot

- In scope: поддержка динамических источников правил роутинга — GeoIP, GeoSite и кастомные URL-списки, которые резолвятся в CIDR и домены на старте клиента.
- Out of scope: runtime-матчинг geoip/geosite на каждый пакет, автообновление баз по расписанию (фон), drag-n-drop редактор правил, поддержка других форматов баз (GeoLite2 MMDB).

## Цель

Пользователь, мигрирующий с v2rayA, может указать `geoip: "ru"`, `geosite: "yandex"` или ссылку на кастомный список IP/доменов в конфиге роутинга. При старте клиент скачивает базы (geoip.dat, geosite.dat), раскрывает их в CIDR и домены, смерживает со статическими `exclude_ranges`/`include_ranges` и работает как раньше — без изменения движка роутинга.

## Основной сценарий

1. Пользователь добавляет в `routing` конфига новые поля: `geoip_url`, `geosite_url` и/или `include_sources`/`exclude_sources` с элементами `geoip`, `geosite`, `cidr`, `url`.
2. При старте клиент проверяет наличие локально закешированных geoip.dat/geosite.dat. Если их нет или истёк TTL — скачивает по `geoip_url`/`geosite_url`.
3. Проходит `include_sources`/`exclude_sources`: каждый `geoip: "ru"` раскрывается в список CIDR из geoip.dat, `geosite: "yandex"` — в список доменов из geosite.dat, `cidr: "10.0.0.0/8"` — напрямую, `url: "..."` — скачивает и парсит строки.
4. Раскрытые CIDR добавляются к `include_ranges`/`exclude_ranges`, домены — к `include_domains`/`exclude_domains`.
5. Движок роутинга (`RuleSet`) работает с итоговыми плоскими списками — без изменений.
6. Если скачивание базы не удалось (нет сети, битый файл) — клиент логирует warning и продолжает работу со статическими правилами. Падения нет.

## User Stories

- P1 (Import правил): пользователь может указать `geoip: "ru"` в exclude_sources, и при старте все российские IP автоматически исключаются из VPN.
- P2 (Кастомные списки): пользователь может указать `url: "https://example.com/blocked-ips.txt"` и клиент подтягивает свежий список CIDR при каждом старте.
- P3 (GeoSite): пользователь может указать `geosite: "yandex"` в include_sources, и трафик на Яндекс-домены идёт через VPN.

## MVP Slice

GeoIP + URL-источники (P1 + P2). Геосайт (P3) — во вторую очередь, так как geosite.dat имеет другой формат (protobuf) и требует отдельного парсера.

## First Deployable Outcome

После первого implementation pass: пользователь кладёт рядом с конфигом `geoip.dat`, указывает `geoip_url` и `geoip: "ru"` в `exclude_sources`. При старте клиент читает базу, раскрывает российские CIDR в `exclude_ranges`, трафик на эти IP идёт напрямую.

## Scope

- `RoutingCfg`: новые поля `GeoIPURL`, `GeoSiteURL`, `IncludeSources`, `ExcludeSources`
- `RelayRoutingCfg`: новые поля `DirectSources`, `GeoIPPath`, `GeoSitePath`, `GeoIPURL`, `GeoSiteURL`, `SourceTTL`
- Новый тип `SourceRule` с одним из: `GeoIP`, `GeoSite`, `CIDR`, `URL`
- Модуль разрешения источников (resolver): скачивание/кеширование geoip.dat/geosite.dat, парсинг формата v2fly, раскрытие в CIDR/домены
- Модификация bootstrap клиента (TUN и proxy режимы): перед созданием `RuleSet` — резолв источников, мерж с плоскими списками
- Модификация bootstrap релея (terminator): перед созданием `RuleSet` — резолв `direct_sources`, мерж в `DirectRanges`/`DirectDomains`
- Кеширование баз на диске (рядом с конфигом), перекачка по TTL или при изменении версии
- Graceful degradation: при ошибке загрузки/парсинга — warning, работа со статикой
- Trace: `@sk-task` на новых типах и функциях, `@sk-test` на тестах парсинга geoip.dat

## Контекст

- Текущий `RoutingCfg` содержит только плоские списки `[]string`. Движок роутинга не умеет в geoip/geosite на лету.
- GeoIP.dat от v2fly — это protobuf-формат (v2ray.core.app.router.GeoIPList). GeoSite.dat — аналогично v2ray.core.app.router.GeoSiteList.
- Clash/Mihomo используют похожий механизм: `payload` + `type` в правилах, резолв на старте.
- v2rayA использует те же .dat файлы, пользователи уже знакомы с ними.
- База geoip.dat лицензирована под CC-BY-SA (v2fly), можно распространять и скачивать.
- Важно: `.dat` — это не текстовый файл, а proto. Нужна генерация Go-кода из .proto или готовый парсер.
- Альтернатива — не парсить .dat, а принимать CIDR-списки через URL (текстовый формат). Это проще и тоже полезно.

## Требования

- RQ-001 Система ДОЛЖНА поддерживать новый тип конфигурации `SourceRule` с полями `geoip`, `geosite`, `cidr`, `url`, где ровно одно поле задано
- RQ-002 Система ДОЛЖНА резолвить `SourceRule` на этапе bootstrap клиента до создания `RuleSet`
- RQ-003 Система ДОЛЖНА поддерживать `routing.geoip_path` — статический путь к geoip.dat без автообновления. Если указан — файл читается с диска, `geoip_url` игнорируется
- RQ-003b Система ДОЛЖНА скачивать geoip.dat по `routing.geoip_url` при старте, если `geoip_path` не указан, а файла нет локально или истёк TTL (24h по умолчанию), а также по явному запросу (кнопка "Refresh sources" в Web UI/Android)
- RQ-004 Система ДОЛЖНА парсить geoip.dat (v2fly protobuf-формат) и извлекать CIDR для указанного двухбуквенного кода страны
- RQ-005 Система ДОЛЖНА для `SourceRule.cidr` добавлять значение напрямую без внешних запросов
- RQ-006 Система ДОЛЖНА для `SourceRule.url` скачивать текстовый файл и парсить каждую непустую строку как CIDR или домен (по наличию `/`)
- RQ-007 Система ДОЛЖНА смерживать раскрытые CIDR с существующим `include_ranges`/`exclude_ranges` (без дубликатов) до передачи в `RuleSet`
- RQ-008 Система ДОЛЖНА смерживать раскрытые домены с существующим `include_domains`/`exclude_domains` (без дубликатов) до передачи в `RuleSet`
- RQ-009 Система ДОЛЖНА при ошибке загрузки/парсинга любого источника логировать warning и продолжать работу без него (не падать)
- RQ-010 Система ДОЛЖНА при указании `geoip` или `geosite` без `geoip_path`/`geosite_path` и без `geoip_url`/`geosite_url` логировать debug и пропускать источник
- RQ-011 Система ДОЛЖНА кешировать geoip.dat/geosite.dat на диске (рядом с конфигом, имена `geoip.dat`/`geosite.dat`) для работы офлайн
- RQ-012 Система ДОЛЖНА поддерживать `SourceRule.geosite` — раскрытие категории доменов из geosite.dat (v2fly protobuf-формат)
- RQ-013 Web UI (kvn-web) ДОЛЖЕН отображать `include_sources`/`exclude_sources` в разделе Routing как структурированные поля: карточки с выбором типа (GeoIP/GeoSite/CIDR/URL) и полем ввода
- RQ-014 Android-клиент ДОЛЖЕН при импорте конфига (QR/Paste) корректно парсить `include_sources`/`exclude_sources` (ignoreUnknownKeys=true — уже есть)
- RQ-015 Web UI и Android ДОЛЖНЫ иметь кнопку "Refresh sources" для принудительного перескачивания баз и перерезолва источников
- RQ-016 Система ДОЛЖНА поддерживать built-in alias `geoip: "private"` без внешней базы, раскрывающийся в частные диапазоны (RFC 1918 + link-local + unique-local)

## Вне scope

- Runtime geoip/geosite матчинг на каждый пакет/соединение — раскрытие происходит единожды на старте (или при ручном Refresh)
- Фоновое автообновление geoip.dat/geosite.dat по расписанию (только при старте или по кнопке Refresh)
- UI drag-n-drop или визуальный редактор с графами — структурированные поля в карточках
- Поддержка других баз (GeoLite2 MMDB от MaxMind)
- Валидация URL источников на корректность (невалидный URL = warning, пропуск)
- Многопоточное скачивание/парсинг источников (все последовательно на старте)

## Критерии приемки

### AC-001 SourceRule структура

- Почему это важно: основа фичи — правильная модель данных для источника правил
- **Given** конфиг с `include_sources: [{geoip: "ru"}, {cidr: "10.0.0.0/8"}, {url: "https://example.com/ips.txt"}]`
- **When** клиент парсит конфиг
- **Then** каждый SourceRule распознаётся с корректным типом и значением
- Evidence: unit-тест десериализации YAML/JSON в `[]SourceRule`

### AC-002 GeoIP резолв

- Почему это важно: пользователь может указать страну и получить CIDR
- **Given** geoip.dat рядом с конфигом и `geoip_url` не указан (offline-режим)
- **When** клиент резолвит `exclude_sources: [{geoip: "ru"}]`
- **Then** в `exclude_ranges` появляются все CIDR для `ru` из базы
- Evidence: unit-тест с тестовым geoip.dat (2-3 записи) проверяет раскрытие

### AC-003 URL источник

- Почему это важно: пользователь может использовать кастомные списки
- **Given** `exclude_sources: [{url: "file:///tmp/custom.txt"}]` и файл с содержимым `10.0.0.0/8\n192.168.0.0/16`
- **When** клиент резолвит источники
- **Then** оба CIDR добавляются в `exclude_ranges`
- Evidence: unit-тест с file:// URL

### AC-004 CIDR источник

- Почему это важно: статическое правило, не требующее внешних запросов
- **Given** `include_sources: [{cidr: "172.16.0.0/12"}]`
- **When** клиент резолвит источники
- **Then** `172.16.0.0/12` добавляется в `include_ranges`
- Evidence: unit-тест прямого добавления CIDR

### AC-005 Мерж с плоскими списками

- Почему это важно: статика и источники работают вместе, без дубликатов
- **Given** `exclude_ranges: ["10.0.0.0/8"]` и `exclude_sources: [{cidr: "10.0.0.0/8"}, {cidr: "192.168.0.0/16"}]`
- **When** клиент резолвит и мержит
- **Then** `exclude_ranges = ["10.0.0.0/8", "192.168.0.0/16"]` (без дубликатов)
- Evidence: unit-тест проверяет merge + dedup

### AC-006 Graceful degradation

- Почему это важно: битый URL не должен валить клиент
- **Given** `geoip_url: "https://invalid.example/nonexistent.dat"` и `exclude_sources: [{geoip: "ru"}]`
- **When** клиент стартует и не может скачать базу
- **Then** клиент логирует warning и продолжает работу, `geoip: "ru"` игнорируется
- Evidence: в логах есть warning, клиент не упал, статические правила работают

### AC-007 Кеширование баз

- Почему это важно: не скачивать базы при каждом старте
- **Given** geoip.dat уже существует локально (возраст < 24h)
- **When** клиент стартует
- **Then** клиент не скачивает базу заново, использует локальную
- Evidence: unit-тест с mock HTTP-сервером проверяет conditional GET или TTL

### AC-008 GeoSite резолв

- Почему это важно: пользователь может указать категорию доменов
- **Given** geosite.dat рядом с конфигом и `include_sources: [{geosite: "yandex"}]`
- **When** клиент резолвит источники
- **Then** в `include_domains` появляются все домены категории `yandex` из geosite.dat
- Evidence: unit-тест с тестовым geosite.dat (2-3 записи) проверяет раскрытие

### AC-009 Пропуск geoip без базы

- Почему это важно: geoip/geosite — опциональный функционал, отсутствие базы не должно пугать пользователя
- **Given** `geoip_path` и `geoip_url` не указаны, но есть `exclude_sources: [{geoip: "ru"}]`
- **When** клиент стартует
- **Then** клиент логирует debug и пропускает этот источник
- Evidence: в логах есть debug-сообщение, клиент не упал

### AC-010 Web UI отображение sources

- Почему это важно: пользователь kvn-web должен видеть источники в интерфейсе
- **Given** в конфиге сервера есть `include_sources`/`exclude_sources`
- **When** пользователь открывает страницу редактирования конфига
- **Then** поля `Include Sources` и `Exclude Sources` отображаются в секции Routing
- Evidence: визуальная проверка, JSON/YAML содержит корректные поля

### AC-011 Refresh sources button

- Почему это важно: пользователь может обновить базы без перезапуска клиента
- **Given** клиент запущен, sources загружены, в Web UI есть кнопка "Refresh sources"
- **When** пользователь нажимает "Refresh sources"
- **Then** клиент вызывает `resolver.Refresh()` (сброс кеша + перескачивание geoip.dat/geosite.dat по URL), перерезолвит все источники, создаёт новый `RuleSet` с актуальными CIDR/доменами, и атомарно подменяет ссылку через `TunRouter.SetRuleSet()` / proxy. Существующие соединения продолжают использовать старый RuleSet до своего завершения
- Evidence: после нажатия новые CIDR/домены применяются к новым соединениям, старые соединения не рвутся

## Допущения

- Парсер geoip.dat и geosite.dat использует protobuf-схемы v2fly (v2ray.com.core.router.GeoIPList, GeoSiteList)
- Go-код для proto генерируется из .proto файлов или используется сторонняя библиотека
- geoip.dat/geosite.dat хранятся в той же директории, что и конфиг клиента
- TTL кеша по умолчанию — 24 часа, конфигурируемый через `routing.source_ttl_hours`
- Размер geoip.dat ~4MB сжато, geosite.dat ~1MB — скачивание на старте добавляет 1-5 секунд к времени запуска
- Формат URL-списка: одна запись на строку, строки начинающиеся с `#` — комментарий, пустые строки игнорируются

## Критерии успеха

- SC-001 Время запуска клиента с одним geoip-источником (локальный файл, тестовый geoip.dat ~50 записей) — не более +100ms
- SC-002 HTTP-клиент для скачивания баз имеет таймаут 30s. При превышении — warning в лог, fallback на кеш (если есть)

## Краевые случаи

- Пустой список источников: `sources: []` — ничего не меняется, клиент работает как обычно
- Неизвестный двухбуквенный код страны: `geoip: "zz"` — warning, пропуск
- Битая geoip.dat: error в лог, пропуск всех `geoip`-источников, клиент продолжает работу
- Несколько одинаковых `geoip` в разных `include_sources`/`exclude_sources`: каждый резолвится независимо, при мерже — dedup
- `geoip` и `geosite` без локальной базы и без URL: debug, пропуск (опциональный функционал)
- Очень большой URL-список (>100k строк): парсинг без ограничения по времени, но не блокируя основной поток (разумный timeout)
- Пересекающиеся CIDR в include и exclude: exclude имеет приоритет (текущее поведение `RuleSet`)
- SourceRule с двумя полями (напр. `geoip` и `cidr` одновременно): ошибка валидации, SourceRule игнорируется, warning

## Принятые решения (refinement)

### Парсинг .dat без внешней зависимости

Не тащим `github.com/v2fly/domain-list-community`. Формат geoip.dat — protobuf с одной схемой `GeoIPList`:
```protobuf
message GeoIPList { repeated GeoIP entry = 1; }
message GeoIP {
  string country_code = 1;
  repeated CIDR cidr = 2;
}
message CIDR { bytes ip = 1; uint32 prefix = 2; }
```

Генерируем Go-код из .proto через `protoc` + `protoc-gen-go` один раз, кладём в `src/internal/routing/geoip/`. Никакой runtime-зависимости от v2fly; только `google.golang.org/protobuf` (уже может быть в go.sum, если нет — одна лёгкая зависимость).

GeoSite.dat — аналогично: `GeoSiteList { repeated GeoSite entry = 1; }`, `GeoSite { string category_code = 1; repeated Domain domain = 2; }`, `Domain { string value = 1; Type type = 2; }`.

### Кеш баз — все платформы

geoip.dat и geosite.dat кешируются на диске рядом с конфигом на всех платформах (Linux, macOS, Windows). Размер ~5MB на обе — несущественно.

### Обновление баз — при старте + кнопка в UI

- При старте: если базы нет или возраст > 24h — скачать. Если скачивание не удалось — использовать кеш (если есть) или warning.
- Кнопка "Refresh sources" в Web UI (секция Routing) — перезагружает все базы и источники без перезапуска клиента.
- Android: кнопка "Refresh" на экране настроек.

### Web UI — структурированные поля

Sources отображаются как список карточек, каждая с выпадающим списком типа (GeoIP / GeoSite / CIDR / URL) и соответствующим полем ввода (country code / category / CIDR / URL). Кнопки Add / Delete для управления списком.

### Built-in alias: `geoip: "private"`

Резолвится в частные диапазоны без внешней базы:
```
10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 100.64.0.0/10 (CGNAT),
127.0.0.0/8, 169.254.0.0/16, ::1/128, fc00::/7, fe80::/10
```
Совпадает с `DefaultExcludeRanges` + CGNAT + ULA.

## Открытые вопросы

- none
