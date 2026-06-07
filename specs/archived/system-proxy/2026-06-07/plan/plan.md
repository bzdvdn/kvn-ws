# System Proxy Integration — План

## Phase Contract

Inputs: spec, inspect (pass), minimal repo-контекст.
Outputs: plan, data-model.md (no-change).
Stop if: spec слишком расплывчата — spec чёткая, продолжение безопасно.

## Цель

Автоматическая установка/восстановление системных прокси-настроек ОС при старте/стопе proxy-режима клиента. Без изменений data model — только новое поле в ClientConfig.

## MVP Slice

Linux env vars (AC-001, AC-002, AC-003) — установка HTTP_PROXY/HTTPS_PROXY/NO_PROXY при старте proxy listener, восстановление при остановке. Всё в одном PR, без поэтапной раскатки.

## First Validation Path

1. Запустить клиент на Linux с `mode: proxy`, `system_proxy: true`, `routing.exclude_ranges: ["10.0.0.0/8"]`
2. Проверить `echo $HTTP_PROXY` → `http://127.0.0.1:2310`
3. Проверить `echo $NO_PROXY` → содержит `10.0.0.0/8`
4. Нажать Ctrl+C — проверить что переменные очищены

## Scope

- Новый пакет `internal/systemproxy/` с платформозависимой реализацией
- Поле `SystemProxy` в ClientConfig + default config в webui
- Интеграция: set при старте proxy listener, restore при shutdown (через defer в Run)
- NO_PROXY из routing exclude-правил (exclude_ranges, exclude_ips, exclude_domains)
- Linux: env vars (os.Setenv) + systemd override (файл конфига)
- macOS: exec.Command("networksetup")
- Windows: syscall/registry WinHTTP API
- Graceful shutdown: сохранение оригиналов, восстановление в defer
- Best-effort recovery при старте: проверка что системный прокси указывает на нас

## Implementation Surfaces

| Surface | Статус | Зачем |
|---------|--------|-------|
| `internal/systemproxy/` | **новая** | Платформозависимая установка/восстановление системных прокси |
| `internal/systemproxy/systemproxy.go` | новая | Общий интерфейс `Manager`, `NOProxyBuilder`, сохранение состояния |
| `internal/systemproxy/proxy_linux.go` | новая | Linux: os.Setenv + systemd override |
| `internal/systemproxy/proxy_darwin.go` | новая | macOS: exec.Command networksetup |
| `internal/systemproxy/proxy_windows.go` | новая | Windows: WinHTTP/реестр |
| `internal/systemproxy/proxy_stub.go` | новая | Заглушка для неподдерживаемых платформ |
| `internal/config/client.go` | существующая | Добавить `SystemProxy *bool` в ClientConfig |
| `internal/bootstrap/client/client.go` | существующая | Вызов systemproxy.Set/Restore в Run |
| `internal/bootstrap/client/proxy.go` | существующая | Точка старта proxy listener |
| `internal/webui/handler_config.go` | существующая | Default config: SystemProxy включаем по умолчанию для proxy mode |
| `internal/webui/frontend/src/App.tsx` | существующая | Чекбокс "Use as system proxy" |

## Bootstrapping Surfaces

- `internal/systemproxy/` — создать директорию и файлы

## Влияние на архитектуру

- Локальное: новый пакет без зависимостей от других internal-пакетов (кроме config для RoutingCfg)
- Интеграции: Client.Run() вызывает systemproxy.Set/Restore в proxy mode
- Совместимость: обратная, новое поле опционально
- systemd override требует root — логируем warning при отказе, не блокируем

## Acceptance Approach

- AC-001: Linux env vars — unit test + manual `/proc/self/environ`
- AC-002: Restore на SIGTERM — defer restore в Run, тест через временную замену env
- AC-003: NO_PROXY из exclude — `NOProxyBuilder` unit test с разными комбинациями exclude
- AC-004: macOS — manual на реальной macOS (CI нет); unit test для парсинга вывода networksetup
- AC-005: Windows — manual; unit test для вызова WinHTTP API
- AC-006: Recovery — проверка лога при старте, unit test для проверки "наш ли прокси"
- AC-007: systemd permission — unit test для ошибочного пути, проверка warning в логе

## Данные и контракты

- Data model: ClientConfig добавляет `SystemProxy *bool` (nil = auto: true для proxy mode)
- NO_PROXY строится из `RoutingCfg.ExcludeRanges + ExcludeIPs + ExcludeDomains`
- Никаких новых API/event контрактов
- `data-model.md` создаётся как no-change stub (только новое поле в конфиге)

## Стратегия реализации

- DEC-001 Пакет internal/systemproxy с платформенными файлами
  Why: изоляция платформозависимого кода, build tags управляют сборкой.
  Tradeoff: дублирование интерфейса между файлами — но это стандартная Go-практика.
  Affects: internal/systemproxy/
  Validation: `GOOS=windows go build ./src/...`, `GOOS=darwin go build ./src/...`

- DEC-002 Set/Restore через сохранение предыдущих значений в памяти
  Why: не нужно читать файлы при восстановлении; стек вызовов гарантирует порядок.
  Tradeoff: при краше (kill -9) восстановление не происходит — используем recovery при следующем старте.
  Affects: internal/systemproxy/
  Validation: unit test: Set → изменить окружение → Restore → проверить оригинал

- DEC-003 NO_PROXY строится из routing.RuleSet
  Why: RuleSet уже хранит обработанные exclude-правила; не нужно дублировать парсинг.
  Tradeoff: RuleSet нужно экспортировать из пакета routing.
  Affects: internal/routing/rule_set.go, internal/systemproxy/
  Validation: unit test с exclude_ranges=["10.0.0.0/8"], exclude_domains=[".example.com"]

- DEC-004 Systemd override — best-effort, не блокирует старт
  Why: пользователь без root не должен терять возможность использовать system proxy.
  Tradeoff: systemd override может не примениться без явной ошибки.
  Affects: internal/systemproxy/proxy_linux.go
  Validation: AC-007

## Incremental Delivery

### MVP (Первая ценность)

- Пакет `internal/systemproxy/` с реализацией для Linux (env vars + systemd)
- Поле `SystemProxy` в конфиге
- Интеграция в `Client.Run()`
- Покрытие AC-001, AC-002, AC-003, AC-007
- Frontend: чекбокс в UI

### Итеративное расширение

- macOS (AC-004): отдельный PR, требует мака для теста
- Windows (AC-005): отдельный PR, требует винды для теста
- Recovery при краше (AC-006): добавить проверку при старте

## Порядок реализации

1. `internal/systemproxy/` — интерфейс + Linux impl + тесты
2. `internal/config/client.go` — поле SystemProxy
3. `internal/bootstrap/client/client.go` — интеграция Set/Restore
4. `internal/webui/` — default config + UI чекбокс
5. macOS impl
6. Windows impl
7. Recovery (AC-006)

## Риски

- macOS networksetup требует прав root → при запуске без root логируем warning, не применяем
  Mitigation: fallback на env vars (как на Linux) доступен всегда
- Windows WinHTTP API может не отработать в некоторых редакциях Windows
  Mitigation: fallback на реестр HKCU
- При краше (kill -9) env vars не восстанавливаются — дочерние процессы наследуют прокси
  Mitigation: recovery при следующем запуске; если прокси указывает на нас — очищаем

## Rollout и compatibility

- Новое поле `system_proxy` в YAML опционально — обратная совместимость
- По умолчанию `nil` = auto (true для proxy mode, false для tun)
- Специальных rollout-действий не требуется

## Проверка

- `internal/systemproxy/`: unit tests для Linux env, NOProxyBuilder, restore, systemd permission
- `internal/config/`: test default config содержит SystemProxy
- `go vet ./src/...`, `go build ./src/...`, `GOOS=windows go build ./src/...`
- `go test -race ./src/...`
- Manual: запуск клиента на Linux, проверка env, Ctrl+C, проверка очистки

## Соответствие конституции

- нет конфликтов
