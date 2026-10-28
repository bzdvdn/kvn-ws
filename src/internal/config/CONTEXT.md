# Context — src/internal/config

## Purpose

Схемы и загрузчики конфигурации KVN 2.0: v2 server/client для CLI и единый `WebUIConfig` для `kvn-web`. Legacy-типы v1 сохранены только для чтения/миграции.

## Responsibilities

- Владеет: `V2ServerConfig`, `V2ClientConfig`, `TUNConfig`, `UTLSConfig`, `V2StealthConfig`, `WebUIConfig`/`ServerEntry`, legacy `ClientConfig`/`ServerConfig`/`RelayConfig`, YAML/JSON-маршалингом и дефолтами.
- Не владеет: запуском транспорта/клиента, DNS-резолвом, TUN-маршрутами и UI-рендерингом.

## How it works

- `LoadV2ServerConfig` читает YAML через viper, применяет defaults (`listen 0.0.0.0:443`, `max_streams 4096`, fallback) и валидирует TLS, токены и TUN pool.
- `LoadV2ClientConfig` применяет defaults (`mtu 1200`, `mode tun`, uTLS Chrome/Firefox/Safari, adaptive padding 64) и валидирует серверы, TLS и stealth path.
- `EnsureClientID` хранит сгенерированный `client_id` в sidecar `<config>.client_id` с правами `0600`; ID не попадает в QR.
- `LoadWebUIConfig`/`LoadWebUIConfigOptional` читают единый `config.yaml`; `SaveWebUIConfig` пишет `0600` в каталог `0700`.
- `WebUIConfig` = глобальный `ClientConfig` + `Servers[]` + `ActiveServer`; v2 webui-store фильтрует `protocol: v2`, старый v1-файл сохраняет как `.v1.bak`, legacy `~/.config/kvn/v2/config.yaml` импортирует один раз.
- `LoadClientConfig`/`LoadServerConfig`/`LoadRelayConfig` — legacy-лоадеры старых типов; v2 entrypoints используют `LoadV2*`.

## Key files

- `v2.go` — v2 server/client schema, defaults, validation, client ID sidecar.
- `webui.go` — unified multi-server Web UI config и миграция.
- `client.go` — `ClientConfig`, routing/TLS/obfuscation, legacy client/relay loaders.
- `server.go` — legacy server config и общие TLS/token типы.
- `config.go` — `LogConfig`, `AtomicConfig[T]`.

## Public contracts

- `LoadV2ServerConfig`, `LoadV2ClientConfig`, `LoadWebUIConfig`, `LoadWebUIConfigOptional`, `SaveWebUIConfig`.
- `DefaultV2ServerConfig`, `DefaultV2ClientConfig`, `EnsureClientID`.
- v2 JSON-теги — контракт для `kvn-web`, Android и QR; YAML-теги — для файлов.
- `V2ClientConfig` содержит `quic_enabled`/`quic_endpoint`, `utls`, `stealth`, `routing`, `dns_proxy`, `tls`.

## Gotchas

- v2 не читает и не перезаписывает v1 `config.yaml`; v1-файл сохраняется как `.v1.bak`.
- `protocol` для v2-серверов всегда `v2`; запись без серверов — валидное first-run состояние.
- `tls.sni` с `verify_mode: verify` не совпадёт с сертификатом; нужен `insecure` или корректный `server_name`.
- Секреты и токены не логируются; файл конфига `0600`, каталог `0700`.
- `AtomicConfig` безопасен только при замене указателя целиком.
