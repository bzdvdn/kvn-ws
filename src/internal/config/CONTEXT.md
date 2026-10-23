# Context — src/internal/config

## Purpose

Единый источник типов и загрузчиков конфигурации для всех бинарей KVN (client, server, relay, web). Инкапсулирует YAML + env-override, дефолты, backward-compat миграции старых ключей и многосерверную модель Web UI.

## Responsibilities

- Владеет: структурами `ClientConfig`, `ServerConfig`, `RelayConfig`, `WebUIConfig`, `RoutingCfg`/`DNSProxyCfg`/`ObfuscationCfg` и их JSON/YAML-маршалингом, загрузкой/сохранением, нормализацией значений, предупреждениями о секретах в файле.
- Не владеет: применением конфига (это вызывающие bootstrap-пакеты), загрузкой geoip/geosite-источников по сети (это `src/internal/routing`), валидацией маршрутов/rules.

## How it works

- Все лоадеры (`LoadClientConfig`, `LoadServerConfig`, `LoadRelayConfig`) используют `viper` с `SetEnvPrefix` + `SetEnvKeyReplacer(".", "_")` + `AutomaticEnv`: env-переменные `KVN_CLIENT_*`, `KVN_SERVER_*`, `KVN_RELAY_*` переопределяют YAML.
- `LoadClientConfig` дополнительно нормализует legacy-формы: `obfuscation: true` → `{enabled:true}`, `dns_proxy.upstream` → `upstreams`, `routing.dns_cache` → `dns_routing`; вызывает `SetClientDefaults`, проверяет relay-блок и печатает предупреждения (SNI+verify, секреты в файле).
- `LoadServerConfig`: дефолты rate-limit/MTU/admin, `normalizeTLS` (семантика `tls.client_auth`), `normalizeTokens` (имена и секреты-фолбэки), токены из `KVN_SERVER_AUTH_TOKENS_JSON`, обязательный `crypto.key` при `crypto.enabled`.
- `SetClientDefaults` — идемпотентная нормализация: MTU 1400, `mode: proxy`, `proxy_listen 127.0.0.1:2310`, `auto_reconnect: true`, `max_message_size` 10 МиБ, `tunnel_timeout` 30, `proxy_max_concurrency` 1000, `proxy_connections` 10, DNS-прокси `127.0.0.153:53` и upstreams `1.1.1.1:53, 8.8.8.8:53`; всегда добавляет `DefaultExcludeRanges` в `exclude_ranges`.
- `SaveClientConfig` декодирует через `mapstructure` в map и пишет YAML с правами `0600`, предварительно нормализуя `routing`.
- `WebUIConfig` = глобальный `ClientConfig` (inline) + `Servers[]` (`ServerEntry{Name + ClientConfig}`) + `ActiveServer`. `LoadWebUIConfig` при отсутствии файла возвращает `defaultWebUIConfig()` (не ошибку), при legacy-файле оборачивает глобальный конфиг в `servers[0].Name="Default"`, чинит `active_server` и мигрирует `routing.dns_cache`.
- `SaveWebUIConfig` создаёт каталог `0700` и пишет файл `0600`.
- `AtomicConfig[T]` — обёртка над `atomic.Pointer[T]` с `Load`/`Store` для hot-reload (используется сервером для SIGHUP-перечитывания).

## Key files

- `client.go` — `ClientConfig` и вложенные типы, `LoadClientConfig`, `SetClientDefaults`, `SaveClientConfig`, `LoadRelayConfig`, глобальные дефолты.
- `server.go` — `ServerConfig` и его секции, `LoadServerConfig`, `normalizeTLS`/`normalizeTokens`.
- `webui.go` — `WebUIConfig`, `ServerEntry`, load/save и миграция многосерверной модели.
- `config.go` — общий viper-лоадер, `AtomicConfig`, работа с секретами из env.

## Public contracts

- Загрузчики: `LoadClientConfig(path)`, `LoadServerConfig(path)`, `LoadRelayConfig(path)`, `LoadWebUIConfig(path)`; сохранение — `SaveClientConfig`, `SaveWebUIConfig` (обе создают каталог и пишут `0600`).
- Нормализация: `SetClientDefaults(*ClientConfig)` — вызывают и CLI, и Web UI перед показом/сохранением.
- Значения по умолчанию: `DefaultDNSUpstreams`, `DefaultExcludeRanges`, `DefaultServerMTU`.
- Хелперы: `(*ClientConfig).UDPDatagramsEnabled()` (nil → true), `(*ServerConfig).UDPDatagramsEnabled()`, `SourceRule.Type/Value/Valid`, `(*PaddingCfg).RealtimeUnpaddedEnabled()`.
- JSON-теги — фактический контракт для Web UI/desktop API; YAML-теги — для файлов конфигурации.

## Dependencies

- `github.com/spf13/viper`, `github.com/go-viper/mapstructure/v2`, `go.yaml.in/yaml/v3` — внешние.
- Внутри репозитория зависимостей нет (листовой пакет); его импортируют почти все остальные.

## Consumed by

- `src/internal/bootstrap/{client,server,relay}`, `src/internal/routing`, `src/internal/protocol/auth`, `src/internal/systemproxy`, `src/internal/webui`, `src/cmd/{client,server,relay,web,desktop,gatetest,stability}`.

## Gotchas

- `LoadClientConfig` **мутирует** конфиг: `routing.exclude_ranges` всегда дополняется `DefaultExcludeRanges` (loopback, multicast, link-local), убрать их нельзя.
- `LoadServerConfig`/`LoadClientConfig` не падают на отсутствующий файл только в Web UI (`LoadWebUIConfig`) — остальные возвращают ошибку чтения.
- `dns_proxy.listen` по умолчанию `127.0.0.153:53`, а не `127.0.0.53/54` — эти адреса занимает systemd-resolved (systemd ≥ 250).
- Секреты: `KVN_*_AUTH_TOKENS_JSON` полностью заменяет `auth.tokens` из YAML; `KVN_SERVER_AUTH_TOKENS_JSON` — JSON-массив `TokenCfg`.
- Legacy-ключи поддерживаются на чтении, но при сохранении пишутся только новые (`dns_routing`, `upstreams`); `WebUIConfig` мигрирует `dns_cache` и при отсутствии `servers` создаёт «Default».
- `crypto.enabled` без `crypto.key` в клиенте молча выключается, а на сервере — фатальная ошибка.
- `tls.sni` при `verify_mode: verify` не сработает по сертификату — лоадер печатает предупреждение, но не меняет поведение.
- `AtomicConfig` не сериализует доступ: безопасен только при замене указателя целиком (конфиг считается immutable после `Store`).
