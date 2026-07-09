# Transport Factory — единый интерфейс создания транспортных соединений

## Scope Snapshot

- In scope: выделить создание transport.StreamConn за фабричный интерфейс, убрав `if transportType == "quic"` из bootstrap-кода
- Out of scope: изменение существующих transport.StreamConn, QUIC, WebSocket реализаций; добавление новых транспортов (gRPC и т.п.)

## Цель

Разработчик, добавляющий новый transport (gRPC, plain TCP, Relay поверх WSS), сейчас вынужден править `bootstrap/client/dial.go`, `bootstrap/relay/upstream.go`, `bootstrap/relay/bridge.go`, `bootstrap/server/handler.go` — каждый содержит прямые вызовы `websocket.Dial(...)`/`websocket.Accept(...)`/`quictp.Dial(...)` с копипастой логики выбора транспорта. Фича вводит `TransportFactory` interface + штатные реализации для WS и QUIC, после чего bootstrap-код оперирует только фабрикой. Успех фичи измеряется отсутствием прямых imports `websocket`/`quic` в пакетах bootstrap.

## Основной сценарий

1. Определён `transport.TransportFactory` interface с методами `Dial(ctx, endpoint) → (StreamConn, error)` и при необходимости `Listen(ctx, addr) → (Listener, error)` для server-side.
2. Реализована `WSFactory` (берёт TLS-конфиг, WSConfig, keepalive params) и `QUICFactory` (берёт TLS, QUICConfig).
3. В конфигурации сервера и клиента транспорт специфицируется строкой (`"ws"`, `"quic"`) — фабрика возвращается по типу.
4. Bootstrap-код вызывает `factory.Dial(ctx, endpoint)` без knowledge о том, WS это или QUIC.
5. Для server-side serve loop вызывает `factory.Listen(ctx, addr)` и в цикле `Accept()`.

## MVP Slice

Client-side Dial через TransportFactory (AC-001, AC-002, AC-003, AC-004, AC-007). Server-side Listen/Accept (AC-005, AC-006) — deferred до следующего цикла.

## First Deployable Outcome

После implementation можно запустить `go test ./src/internal/bootstrap/...` — старые тесты проходят без изменений; в `dial.go`/`upstream.go` нет прямых вызовов `websocket.Dial`/`quictp.Dial` — только через фабрику.

## Scope

- `src/internal/transport/transport.go` — добавление `TransportFactory` и `TransportListener` interface
- `src/internal/transport/websocket/` — `WSFactory` (Dial + Listen)
- `src/internal/transport/quic/` — `QUICFactory` (Dial + Listen)
- `src/internal/bootstrap/client/dial.go` — замена `dialStream()` на фабрику
- `src/internal/bootstrap/relay/upstream.go` — замена `dialUpstream()` на фабрику
- `src/internal/bootstrap/relay/bridge.go` — замена `dialRelayUpstream()` на фабрику
- `src/internal/bootstrap/server/handler.go` — замена `handleTunnel()` на фабрику
- `src/internal/config/` — не требуется изменений; существующее поле `Transport` покрывает выбор фабрики

## Контекст

- Существующий `transport.StreamConn` interface стабилен — менять его не нужно
- В bootstrap-коде транспорт выбирается через `cfg.Transport == "quic"` с fallback на WebSocket (пустая строка `""` / `"ws"`)
- Server-side сейчас всегда использует `websocket.Accept` (HTTP upgrade); QUIC-сервер должен принимать параллельно
- У WS и QUIC разные lifecycle: WS — одно соединение на upgrade, QUIC — listener с Accept
- Конфигурация TLS/обфускации/keepalive привязана к конкретному транспорту — фабрика должна параметризоваться конфигом

## Зависимости

- Никаких меж-спековых зависимостей
- Зависит от существующих пакетов `transport/websocket`, `transport/quic`, `config`

## Требования

- RQ-001 Система ДОЛЖНА предоставлять интерфейс `TransportFactory` с методом `Dial(ctx, endpoint) → (StreamConn, error)`
- RQ-002 Система ДОЛЖНА предоставлять интерфейс `TransportListener` с методами `Listen(ctx, addr) → (Listener, error)` и `Accept(ctx) → (StreamConn, error)`
- RQ-003 `WSFactory` ДОЛЖНА поддерживать все параметры существующего `websocket.Dial`/`Accept` (TLS, WSConfig, keepalive, origin checker)
- RQ-004 `QUICFactory` ДОЛЖНА поддерживать все параметры существующего `quictp.Dial`/`Listen` (TLS, QUIC config, obfuscation)
- RQ-005 Bootstrap-код НЕ ДОЛЖЕН напрямую импортировать `transport/websocket` или `transport/quic` для Dial/Accept; хелперы (типа `isWebSocketRequest`, `WSConfig`) могут оставаться через общий пакет `transport`
- RQ-006 Фабрика ДОЛЖНА поддерживать fallback: при ошибке QUIC Dial вызывать WS Dial с той же конфигурацией

## Вне scope

- Новые типы транспортов (gRPC, plain TCP без TLS, Relay-over-WS)
- Изменение wire protocol или протокола рукопожатия
- Рефакторинг `StreamConn`, `WSConn`, `QUICConn`
- Удаление старых функций `websocket.Dial`/`Accept`, `quictp.Dial`/`Listen` — они остаются для прямого использования
- Динамическая регистрация фабрик (plugin-архитектура)

## Критерии приемки

### AC-001 TransportFactory interface определён

- Почему это важно: единый контракт для всех транспортов
- **Given** пакет `transport`
- **When** определён тип `TransportFactory` с методом `Dial(ctx context.Context, endpoint string) (StreamConn, error)`
- **Then** код компилируется без ошибок
- Evidence: `go build ./src/internal/transport/...`

### AC-002 WSFactory реализует TransportFactory

- Почему это важно: WebSocket остаётся основным транспортом
- **Given** конфигурация с `Transport: "ws"` и TLS-настройками
- **When** вызван `WSFactory.Dial(ctx, "wss://example.com/tunnel")`
- **Then** возвращается `*websocket.WSConn`, реализующий StreamConn
- Evidence: интеграционный тест, который коннектится через WSFactory

### AC-003 QUICFactory реализует TransportFactory

- Почему это важно: QUIC — второй транспорт с тем же контрактом
- **Given** конфигурация с `Transport: "quic"` и TLS-настройками
- **When** вызван `QUICFactory.Dial(ctx, "example.com:443")`
- **Then** возвращается `*quic.QUICConn`, реализующий StreamConn
- Evidence: интеграционный тест, который коннектится через QUICFactory

### AC-004 Bootstrap client использует фабрику вместо прямых вызовов

- Почему это важно: клиентский bootstrap больше не содержит `if transportType == "quic"`
- **Given** `bootstrap/client/dial.go`
- **When** код переписан на использование `TransportFactory`
- **Then** в файле нет прямых импортов `websocket`/`quic` и условие выбора транспорта заменено на `factory := transport.GetFactory(cfg.Transport)`
- Evidence: `go build ./src/internal/bootstrap/client/...` успешен; grep `\"websocket\"` не находит совпадений в dial.go

### AC-005 Bootstrap relay использует фабрику вместо прямых вызовов

- Почему это важно: relay-mode единообразно выбирает транспорт
- **Given** `bootstrap/relay/upstream.go` и `bridge.go`
- **When** код переписан на использование `TransportFactory`
- **Then** в файлах нет прямых импортов `websocket`/`quic` для Dial
- Evidence: `go build ./src/internal/bootstrap/relay/...` успешен; grep `websocket.Dial`/`quictp.Dial` не находит совпадений

### AC-006 Bootstrap server использует фабрику для Accept

- Почему это важно: сервер может принимать WS и QUIC через единый интерфейс
- **Given** `bootstrap/server/handler.go`
- **When** код переписан на использование `TransportListener`
- **Then** handler не импортирует напрямую `websocket`
- Evidence: `go build ./src/internal/bootstrap/server/...` успешен; grep `\"websocket\"` не находит совпадений в handler.go для Accept

### AC-007 Fallback QUIC → WS работает через фабрику

- Почему это важно: фабрика инкапсулирует fallback-логику
- **Given** конфигурация с `Transport: "quic"` и недоступным QUIC-сервером
- **When** вызван `TransportFactory.Dial(ctx, endpoint)`
- **Then** фабрика пробует QUIC, при ошибке вызывает WS Dial, возвращает WSConn
- Evidence: тест с mock-сервером, где QUIC порт закрыт, WS порт открыт — соединение успешно установлено

## Допущения

- Существующая строка `Transport` в `ClientConfig` и `RelayConfig` остаётся способом выбора транспорта; новых полей конфига не вводится
- `websocket.Dial`/`Accept` и `quictp.Dial`/`Listen` остаются публичными — фабрика их оборачивает
- Server-side QUIC слушает на том же порту, что и HTTP (ALPN-negotiation) — не меняется
- Keepalive конфигурация остаётся внутри фабрики; bootstrap не занимается настройкой keepalive после Dial

## Критерии успеха

- SC-001 Все существующие тесты `bootstrap/client`, `bootstrap/relay`, `bootstrap/server` проходят без изменений тестовых данных

## Краевые случаи

- Пустой/неизвестный `Transport` в конфиге — фабрика возвращает WS по умолчанию
- QUIC fallback к WS при недоступности QUIC — фабрика логирует ошибку и вызывает WS
- Оба транспорта недоступны — фабрика возвращает последнюю ошибку (WS), не маскируя QUIC-ошибку полностью
- После Dial у WS-соединения вызывается `SetKeepalive` — фабрика делает это сама, bootstrap не дублирует
- Server-side с `Transport: "quic"` — Listen возвращает TransportListener; Accept блокируется до нового соединения

## Открытые вопросы

Решены в рамках inspect:

1. `TransportListener.Accept()` возвращает `(StreamConn, error)` — для QUIC это естественно; для WS на серверной стороне Accept остаётся через HTTP upgrade внутри реализации, фабрика это скрывает.
2. Один интерфейс `TransportFactory` с методами `Dial` и `Listen` — меньше сущностей, достаточная абстракция.
3. Конфигурация передаётся в конструктор фабрики (`NewWSFactory(tlsCfg, wsCfg, ...)`) — фабрика хранит настройки, не требуя их при каждом Dial.
