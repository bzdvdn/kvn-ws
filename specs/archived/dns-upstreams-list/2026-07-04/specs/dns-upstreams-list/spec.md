# Configurable DNS Upstreams List

## Scope Snapshot

- In scope: замена единственного `upstream` (string) на список `upstreams` ([]string) во всех компонентах (клиент, сервер, relay, web-ui, dnsproxy) с backward compatibility.
- Out of scope: Hot-reload DNS upstreams, динамическое добавление upstream через API, health-checking DNS upstream.

## Цель

Администратор/пользователь получает возможность указать несколько DNS-резолверов в конфиге. При недоступности первого используется следующий (fallback). Текущие адреса (`1.1.1.1:53`, `8.8.8.8:53`) остаются дефолтными. Фича устраняет хардкод и повышает отказоустойчивость DNS-разрешения.

## Основной сценарий

1. Оператор указывает в YAML: `dns_proxy: { upstreams: ["1.1.1.1:53", "8.8.8.8:53"] }`.
2. Клиент/сервер загружают конфиг, DNS proxy и server-side DNS forward используют первый адрес, при ошибке переходят ко второму.
3. Если указан старый `upstream: "x.x.x.x:53"` — он работает без изменений (мигрируется в `upstreams[0]`).
4. Если не указано ничего — работают дефолтные значения.

## User Stories

- P1: DevOps указывает 2+ DNS upstream для отказоустойчивости в TUN/прокси-режиме.
- P2: Разработчик использует старый конфиг с `upstream` — ничего не ломается.

## MVP Slice

Замена `DNSProxyCfg.Upstream` на `Upstreams []string` + server-side `dns_upstreams` в `ServerConfig`. Fallback-логика при недоступности первого DNS.

## First Deployable Outcome

После имплементации можно указать `upstreams: [10.0.0.1:53, 1.1.1.1:53]` и проверить, что при падении первого резолвера запрос идёт ко второму.

## Scope

- `DNSProxyCfg` — замена `Upstream string` на `Upstreams []string` с backward compat (unmarshal both)
- `RelayDNSCfg.Upstream` → `Upstreams []string` с backward compat
- `ServerConfig` — добавление `DNSUpstreams []string` (дефолт `["1.1.1.1:53", "8.8.8.8:53"]`), проброс в `tunnel.Session.handleDNSFrame`
- `dnsproxy.New(listen, upstreams ...string)` — принимает вариативный список upstream, fallback при ошибке
- `defaultWebUIConfig()` — обновление дефолта
- WebUI фронтенд (`App.tsx`): замена поля `upstream` на список `upstreams` с add/remove
- WebUI TypeScript тип: обновление `dns_proxy` интерфейса
- WebUI `handler_connect.go`: обновление `mergeConfig` для `Upstreams`
- WebUI `handler_connect.go`: обновление проверки мерджа DNS-прокси
- YAML-конфиги (`configs/*.yaml`, `examples/*.yaml`) — обновление примеров
- Документация (`docs/ru/`, `docs/en/`) — обновление описания полей

## Контекст

- Существующий конфиг с полем `dns_proxy.upstream` должен продолжать работать без изменений.
- Централизованный `DefaultDNSUpstreams` в `config` пакете как единый source of truth.
- Сервер-side DNS (`session.go:392`) сейчас использует `8.8.8.8:53` — его тоже нужно сделать конфигурируемым.

## Зависимости

- `none`

## Требования

- RQ-001 Клиент/relay/web-ui ДОЛЖНЫ принимать `dns_proxy.upstreams` ([]string) на замену `dns_proxy.upstream` (string)
- RQ-002 Старый формат `dns_proxy.upstream: "x.x.x.x:53"` ДОЛЖЕН работать без изменений (backward compat)
- RQ-003 Если `upstreams` пуст — используются дефолтные `["1.1.1.1:53", "8.8.8.8:53"]`
- RQ-004 Сервер (ServerConfig) ДОЛЖЕН принимать `dns_upstreams` ([]string) для server-side DNS forward
- RQ-005 DNS proxy (`dnsproxy.Server`) ДОЛЖЕН при недоступности первого upstream пробовать следующие по порядку
- RQ-006 Server-side `Session.handleDNSFrame` ДОЛЖЕН использовать конфигурируемый список upstream вместо хардкода `8.8.8.8:53`
- RQ-007 `Relay.Routing.DNS.Upstream` → `Upstreams` с той же backward compat
- RQ-008 WebUI TypeScript интерфейс `dns_proxy` ДОЛЖЕН поддерживать `upstreams: string[]` на замену `upstream: string`
- RQ-009 WebUI форма DNS proxy ДОЛЖЕН отображать список upstreams с add/remove кнопками
- RQ-010 WebUI `mergeConfig` ДОЛЖЕН корректно мерджить `Upstreams` при server override
- RQ-011 WebUI ДОЛЖЕН принимать старый формат YAML с `upstream` (backward compat)

## Вне scope

- Graceful fallback между upstream в DNS proxy (простой перебор по порядку)
- Hot-reload DNS upstreams через SIGHUP
- Автоматическое обнаружение DNS серверов из системы
- Health-check / маркировка недоступных upstream

## Критерии приемки

### AC-001 Новый конфиг с upstreams работает

- Почему это важно: пользователь может указать список DNS
- **Given** валидный YAML с `dns_proxy: { upstreams: ["10.0.0.1:53", "1.1.1.1:53"] }`
- **When** `LoadClientConfig(path)` выполняется
- **Then** `cfg.DNSProxy.Upstreams` содержит `["10.0.0.1:53", "1.1.1.1:53"]`
- Evidence: тест `TestDNSProxyCfgUpstreams`

### AC-002 Старый формат upstream работает

- Почему это важно: обратная совместимость
- **Given** YAML с `dns_proxy: { upstream: "1.1.1.1:53" }`
- **When** `LoadClientConfig(path)` выполняется
- **Then** `cfg.DNSProxy.Upstreams` содержит `["1.1.1.1:53"]`
- Evidence: тест `TestDNSProxyCfgBackwardCompat`

### AC-003 Дефолтные upstream при пустом конфиге

- Почему это важно: поведение по умолчанию
- **Given** YAML без секции `dns_proxy`
- **When** `LoadClientConfig(path)` выполняется
- **Then** `cfg.DNSProxy.Upstreams` содержит дефолтные значения
- Evidence: тест `TestDNSProxyCfgDefaults`

### AC-004 Server-side DNS upstreams конфигурируемы

- Почему это важно: сервер не должен хардкодить DNS
- **Given** server.yaml с `dns_upstreams: ["10.0.0.1:53"]`
- **When** `LoadServerConfig(path)` выполняется
- **Then** `cfg.DNSUpstreams` содержит `["10.0.0.1:53"]`
- Evidence: тест `TestServerDNSUpstreams`

### AC-005 DNS proxy fallback при недоступности первого upstream

- Почему это важно: отказоустойчивость
- **Given** dnsproxy создан с `["10.0.0.1:53", "1.1.1.1:53"]`, первый не отвечает
- **When** DNS запрос приходит в proxy
- **Then** запрос перенаправляется ко второму upstream
- Evidence: тест `TestDNSProxyFallback`

### AC-006 Server-side DNS forward использует конфигурируемый upstream

- Почему это важно: убрать хардкод `8.8.8.8:53`
- **Given** сервер сконфигурирован с `dns_upstreams: ["1.1.1.1:53"]`
- **When** клиент отправляет DNS frame
- **Then** сервер резолвит через `1.1.1.1:53`
- Evidence: тест `TestServerDNSForwardUsesConfig`

### AC-007 Relay DNS upstreams backward compat

- Почему это важно: relay не ломается
- **Given** relay.yaml с `relay.routing.dns.upstream: "1.1.1.1:53"`
- **When** `LoadRelayConfig(path)` выполняется
- **Then** `cfg.Relay.Routing.DNS.Upstreams` содержит `["1.1.1.1:53"]`
- Evidence: тест `TestRelayDNSUpstreamBackwardCompat`

### AC-008 WebUI показывает и сохраняет список upstreams

- Почему это важно: пользователь управляет DNS через UI
- **Given** WebUI открыт, в global config поле `dns_proxy.upstreams` содержит `["1.1.1.1:53", "8.8.8.8:53"]`
- **When** пользователь добавляет/удаляет upstream через UI и нажимает Save
- **Then** сохранённый YAML содержит `dns_proxy.upstreams` с актуальным списком
- Evidence: тест `TestWebUIDNSUpstreamsRoundtrip`

### AC-009 WebUI mergeConfig корректно мерджит DNS Upstreams

- Почему это важно: server override не теряет upstreams
- **Given** global config имеет `dns_proxy.upstreams: ["1.1.1.1:53"]`, server config имеет `dns_proxy.upstreams: ["10.0.0.1:53"]`
- **When** `mergeConfig(global, server)` выполняется
- **Then** merged `dns_proxy.upstreams` содержит `["10.0.0.1:53"]`
- Evidence: тест `TestMergeConfigDNSUpstreams`

### AC-010 WebUI принимает старый формат upstream

- Почему это важно: backward compat UI
- **Given** saved config содержит `dns_proxy: { listen: "...", upstream: "1.1.1.1:53" }`
- **When** WebUI загружает этот конфиг
- **Then** `upstream` отображается как `upstreams[0]` в UI
- Evidence: тест `TestWebUIDNSUpstreamBackwardCompat`

## Допущения

- Все upstream указываются в формате `host:port` (как и ранее)
- fallback — простой перебор по порядку без таймаутов/health-check
- WebUI использует дефолты из `ClientConfig` через embedding

## Критерии успеха

- SC-001 Все существующие тесты проходят без изменений после рефакторинга
- SC-002 Покрытие нового кода > 80%

## Краевые случаи

- `upstreams: []` — пустой массив → используются дефолты
- `upstreams: null` / неуказан → дефолты
- Одновременно указаны `upstream` и `upstreams` — приоритет у `upstreams`, warning в лог
- `upstream: ""` — игнорируется

## Открытые вопросы

- `none`
