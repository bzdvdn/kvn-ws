# System Proxy Integration — Задачи

## Phase Contract

Inputs: plan, data-model (no-change), минимальный repo-контекст.
Outputs: упорядоченные исполнимые задачи с покрытием всех AC.
Stop if: задачи расплывчаты или coverage не удаётся сопоставить.

## Surface Map

| Surface | Tasks |
|---------|-------|
| `internal/systemproxy/systemproxy.go` | T1.1, T2.1 |
| `internal/systemproxy/proxy_linux.go` | T2.1 |
| `internal/systemproxy/proxy_darwin.go` | T3.1 |
| `internal/systemproxy/proxy_windows.go` | T3.2 |
| `internal/systemproxy/proxy_stub.go` | T1.1 |
| `internal/systemproxy/systemproxy_test.go` | T4.1 |
| `internal/config/client.go` | T1.2 |
| `internal/bootstrap/client/client.go` | T2.1 |
| `internal/webui/handler_config.go` | T1.2 |
| `internal/webui/frontend/src/App.tsx` | T2.2 |

## Implementation Context

- Цель MVP: Linux env vars (AC-001–003, AC-007) + UI checkbox в блоке proxy-настроек
- Границы приемки: AC-001, AC-002, AC-003, AC-007
- Ключевые правила: build tags для платформ; Set/Restore через сохранение оригинала; конституция — trace-маркеры над объявлениями, не на уровне package
- Инварианты: system_proxy применяется только при `mode: proxy`; NO_PROXY пуст если exclude-правил нет
- Контракты: ClientConfig.SystemProxy *bool; RuleSet экспортирует exclude-списки для NOProxyBuilder
- Proof signals: `go build ./src/...` на всех платформах; `go test -race ./src/...`; `HTTP_PROXY` установлен после Set, очищен после Restore
- Вне scope: macOS/Windows impl (вынесены в итеративное расширение)

## Фаза 1: Основа

Цель: подготовить структуру пакета systemproxy и расширить ClientConfig.

- [x] T1.1 Создать пакет `internal/systemproxy/` с файлами:
  - `systemproxy.go` — интерфейс `Manager`, структура `State{origEnv map[string]string}`, функция `New()`, `NOProxyBuilder(rules *config.RoutingCfg) string`
  - `proxy_stub.go` — `//go:build !linux && !darwin && !windows` — заглушка, все методы no-op
  - `proxy_linux.go`, `proxy_darwin.go`, `proxy_windows.go` — скелеты функций (мин. stub impl для компиляции)
  Touches: internal/systemproxy/systemproxy.go, internal/systemproxy/proxy_linux.go, internal/systemproxy/proxy_darwin.go, internal/systemproxy/proxy_windows.go, internal/systemproxy/proxy_stub.go

- [x] T1.2 Добавить поле `SystemProxy *bool` в `ClientConfig`:
  - В `internal/config/client.go`: `SystemProxy *bool json:"system_proxy" mapstructure:"system_proxy"`
  - В `internal/webui/handler_config.go`: `defaultConfig()` → `SystemProxy: nil` (auto)
  Touches: internal/config/client.go, internal/webui/handler_config.go

## Фаза 2: MVP Slice

Цель: Linux env vars — установка/восстановление + UI checkbox в блоке proxy.

- [x] T2.1 Реализовать `Manager` для Linux:
  - `Set(ctx, logger, addr string, noProxy string)`: сохранить текущие HTTP_PROXY/HTTPS_PROXY/NO_PROXY, установить новые через `os.Setenv`
  - `Restore(ctx, logger)`: восстановить сохранённые оригиналы (в т.ч. unset если не было)
  - `SystemdOverride(logger, addr string)`: записать `/etc/systemd/system/kvn-client.service.d/system-proxy.conf` (best-effort, warning при ошибке)
  - В `internal/bootstrap/client/client.go` `Run()`: при `mode: proxy` и `cfg.SystemProxy != false` → создать Manager → `Set()` → defer `Restore()`
  - NO_PROXY строится из `cfg.Routing` (exclude_ranges → CIDR, exclude_ips → IP, exclude_domains → .domain)
  Touches: internal/systemproxy/systemproxy.go, internal/systemproxy/proxy_linux.go, internal/bootstrap/client/client.go

- [x] T2.2 Добавить чекбокс "Use as system proxy" в UI:
  - В `src/internal/webui/frontend/src/App.tsx`, внутри блока `{config.mode === "proxy" && <>...</>}`, после Proxy Password
  - `<Checkbox checked={config.system_proxy ?? true} onChange={(v) => update("system_proxy", v)} label="Use as system proxy" />`
  - Отображается только когда выбран Proxy mode (уже внутри условного блока)
  Touches: src/internal/webui/frontend/src/App.tsx

## Фаза 3: Основная реализация

Цель: macOS + Windows impl + recovery.

- [x] T3.1 Реализовать macOS: `exec.Command("networksetup", "-setwebproxy", iface, "127.0.0.1", port)` + `-setsecurewebproxy`. Определение активного интерфейса через `networksetup -listallnetworkservices`. Restore: сохранить состояние до изменений, вернуть при Restore.
  Touches: internal/systemproxy/proxy_darwin.go

- [x] T3.2 Реализовать Windows: WinHTTP API (`WinHttpSetDefaultProxyConfiguration`) или реестр `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`. Restore: сохранить оригинальные значения, вернуть при Restore.
  Touches: internal/systemproxy/proxy_windows.go

- [x] T3.3 Recovery при старте (AC-006): при `Set()` проверить, не указывает ли текущий системный прокси на наш listener (по адресу/порту). Если да — очистить/восстановить, залогировать recovery.
  Touches: internal/systemproxy/systemproxy.go

## Фаза 4: Проверка

Цель: тесты + verify.

- [x] T4.1 Написать unit-тесты:
  - `TestLinuxSetRestore` — установка env, проверка значений, restore, проверка очистки
  - `TestNOProxyBuilder` — разные комбинации exclude_ranges, exclude_ips, exclude_domains
  - `TestNOProxyBuilderEmpty` — пустые exclude → пустой NO_PROXY
  - `TestSystemdOverridePermissionDenied` — симуляция отказа в записи, проверка warning
  - `TestSystemdOverrideSuccess` — запись во временный файл, проверка содержимого
  - `TestRecovery` — установка чужого прокси → Set → проверка лога recovery
  Touches: internal/systemproxy/systemproxy_test.go

- [x] T4.2 Подтвердить сборку на всех платформах:
  - `go vet ./src/...`
  - `go build ./src/...`
  - `GOOS=darwin go build ./src/...`
  - `GOOS=windows go build ./src/...`
  - `go test -race ./src/...`
  Touches: internal/systemproxy/, internal/bootstrap/client/client.go, internal/config/client.go, internal/webui/

## Покрытие критериев приемки

- AC-001 -> T2.1, T4.1
- AC-002 -> T2.1, T4.1
- AC-003 -> T2.1, T4.1
- AC-004 -> T3.1
- AC-005 -> T3.2
- AC-006 -> T3.3, T4.1
- AC-007 -> T2.1, T4.1

## Заметки

- UI чекбокс рендерится только внутри `{config.mode === "proxy"}` — гарантирует что настройка не видна в TUN mode
- SystemProxy *bool: nil → auto (true для proxy, false для tun); false → явное отключение; true → принудительно даже в tun (но networksetup/env не имеют смысла без listener)
- По умолчанию в webui `SystemProxy: nil` — пользователь видит чекбокс включённым в proxy mode
