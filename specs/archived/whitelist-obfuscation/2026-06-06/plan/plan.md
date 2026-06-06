# Whitelist & Obfuscation Hardening — План

## Цель

Добавить многослойную защиту от DPI и whitelist-блокировок в клиент и сервер. Каждый слой (uTLS, WS path, SNI, padding, QUIC obfuscation) реализуется независимо, с feature-флагом и обратной совместимостью.

## MVP Slice

- uTLS для WS (AC-001, AC-002) + кастомный WS path (AC-003)
- Закрывает 3 из 6 AC — минимальный срез, который уже обходит JA3-блокировку и path-фильтрацию

## First Validation Path

1. Поднять сервер с `ws_paths: ["/tunnel", "/api/events"]`
2. Подключиться клиентом с `tls.utls.enabled: true`, `server: wss://example.com/api/events`
3. Проверить tcpdump: JA3 = Chrome, HTTP Upgrade path = `/api/events`
4. Подключиться на `/secret` — получить 404

## Scope

- Конфигурация клиента: новый блок `obfuscation: { enabled, ... }`, поля в `tls: { utls, sni }`, WS path из URL
- Конфигурация сервера: `ws_paths` allowlist
- WS: uTLS, padding, кастомный path
- QUIC: усиленная обфускация
- Web UI: новые поля в настройках
- НЕ трогаем: transport/quic/quic-go TLS, crypto/tls для QUIC, server-side response obfuscation

## Implementation Surfaces

| Surface | Куда | Изменение | Статус |
|---------|------|-----------|--------|
| `config/client.go` | `ClientConfig.Obfuscation` bool → struct | Новый тип `ObfuscationCfg`, поля `utls`, `sni`, `padding` | существующий |
| `config/server.go` | `ServerConfig.WSPaths` | Новое поле `[]string` | существующий |
| `transport/tls/tls.go` | uTLS dial wrapper | Новый `DialWithUTLS` или опция в `TLSDialer` | существующий |
| `transport/websocket/websocket.go` | WS path, padding в BatchWriter | Padding framing в `BatchWriter.Write`, path из URL | существующий |
| `bootstrap/server/server.go` | WS handler path validation | Проверка path по `ws_paths` allowlist | существующий |
| `transport/quic/obfuscated.go` | Enhanced obfuscation | XOR всего payload, TLS Exporter nonce | существующий |
| `bootstrap/client/tun.go` | QUIC dial nonce setup | Передать `tls.ConnectionState` в `obfuscated.go` | существующий |
| `webui/frontend/src/App.tsx` | UI settings | Поля uTLS, padding, SNI, WS path | существующий |

## Bootstrapping Surfaces

`none` — все нужные файлы и пакеты уже существуют.

## Влияние на архитектуру

- **Config**: `obfuscation` из bool становится блоком — flat bool `mapstructure` может сломаться при загрузке старых конфигов. Нужен кастомный decoder или промежуточный парсинг.
- **TLS**: uTLS возвращает `*utls.UConn`, не `*tls.Conn` — `gorilla/websocket` использует `tls.Conn` через `NetConn()`. Решение: передавать `utls.UConn` как `net.Conn` (имплементирует интерфейс).
- **QUIC nonce**: `ExportKeyingMaterial` доступен только после полного handshake — nonce генерируется при первом `WriteMessage`, не при создании `ObfuscatedQUICConn`.
- **WS path**: gorilla/websocket уже поддерживает path в URL — изменений на клиенте нет, только на сервере.

## Acceptance Approach

### AC-001 uTLS для WS

- Surfaces: `transport/tls/tls.go`, `config/client.go`
- Подход: новый тип `ObfuscationCfg` с `utls.enabled`. В `tls.go` — функция `DialTLSWithUTLS`, заменяющая стандартный `tls.Dial`. `gorilla/websocket.Dialer.NetDial` или `DialTLS` кастомизируется.
- Validation: tcpdump JA3

### AC-002 uTLS fallback

- Surfaces: `transport/tls/tls.go`
- Подход: при ошибке uTLS handshake — переподключение с `crypto/tls`. `utls.fallback: true` (default).
- Validation: имитация отказа uTLS (неверный fingerprint), проверка лога

### AC-003 Кастомный WS path

- Surfaces: `bootstrap/server/server.go`, `config/server.go`
- Подход: сервер читает path из `*http.Request.URL.Path`, проверяет по `ws_paths`. 404 если не в списке.
- Validation: curl на `/secret` → 404

### AC-004 Кастомный SNI

- Surfaces: `config/client.go`, `transport/tls/tls.go`, `bootstrap/client/tun.go` (QUIC)
- Подход: при наличии `tls.sni` — подставить в `tls.Config.ServerName` случайный домен из списка. Для QUIC — тот же `tls.Config` передаётся в quic-go.
- Validation: tcpdump SNI, разные SNI в разных сессиях

### AC-005 WS padding

- Surfaces: `transport/websocket/websocket.go`, `config/client.go`
- Подход: в `BatchWriter.Write` — фрейм `[4B len][payload][random padding до кратного]`. Сервер в `ReadMessage` — парсит фрейм, берёт payload по префиксу длины.
- Validation: размер пакета = кратному

### AC-006 Усиленная QUIC обфускация

- Surfaces: `transport/quic/obfuscated.go`, `bootstrap/client/tun.go`, `bootstrap/server/server.go`
- Подход: убрать plaintext nonce, XOR весь payload. Nonce через `ExportKeyingMaterial` после handshake.
- Validation: в начале QUIC stream нет 8 читаемых байт

## Данные и контракты

- **Config**: `ObfuscationCfg` — новый тип, меняет data model. `obfuscation: true` → парсится как `{enabled: true}` (кастомный decoder для compatibility).
- **Server config**: новое поле `ws_paths []string` — опционально.
- **API/Events**: без изменений. SSE логи уже расширены ранее.

`data-model.md` описывает изменения config structs.

## Стратегия реализации

### DEC-001 uTLS через кастомный `net.Conn` враппер

- **Why**: `gorilla/websocket.Dialer.TLSClientConfig` ожидает `*tls.Config`, uTLS использует `*utls.Config`. Вместо модификации Dialer — оборачиваем `net.Dial` через `DialTLS` с uTLS.
- **Tradeoff**: utls.UConn не полностью совместим с `tls.Conn` (нет `ConnectionState()` в том же виде). Придётся враппить.
- **Affects**: `transport/tls/tls.go`, `transport/websocket/websocket.go`
- **Validation**: WS upgrade success + JA3 Chrome в tcpdump

### DEC-002 Обратная совместимость `obfuscation` поля

- **Why**: старые конфиги с `obfuscation: true` должны работать. Новый формат — `obfuscation: { enabled: true }`.
- **Tradeoff**: кастомный unmarshal — лишняя логика. Альтернатива: сломать старые конфиги и мигрировать через env.
- **Affects**: `config/client.go`
- **Validation**: `obfuscation: true` загружается как `enabled: true`

### DEC-003 Padding в BatchWriter, не отдельным слоем

- **Why**: единый write path для WS, без дополнительной аллокации.
- **Tradeoff**: padding привязан к WS. Для QUIC своя обфускация — не пересекаются.
- **Affects**: `transport/websocket/websocket.go` (BatchWriter)
- **Validation**: размер пакета = padding.size

### DEC-004 QUIC nonce через TLS Exporter

- **Why**: 0 байт на wire, уникален на сессию, привязан к TLS keys. Работает при любом verify_mode.
- **Tradeoff**: nonce доступен только после полного handshake — deferred инициализация.
- **Affects**: `transport/quic/obfuscated.go`, `bootstrap/client/tun.go`
- **Validation**: wireshark: нет plaintext nonce, payload нечитаем

## Incremental Delivery

### MVP (AC-001, AC-002, AC-003)

- uTLS для WS + fallback
- Кастомный WS path (сервер allowlist)
- Config: `obfuscation` блок, `tls.utls`, `server.ws_paths`
- Validation: tcpdump JA3 Chrome + 404 на неразрешённый path

### Iteration 2 (AC-004)

- Кастомный SNI из списка — рандомный выбор при каждом подключении
- Config: `tls.sni: ["www.cloudflare.com", "www.google.com"]`
- Validation: tcpdump показывает разные SNI в разных сессиях

### Iteration 3 (AC-005)

- WS padding в BatchWriter
- Config: `obfuscation.padding`
- Validation: размер пакета кратен padding.size

### Iteration 4 (AC-006)

- Усиленная QUIC обфускация
- Config: `obfuscation.enabled` для QUIC
- Validation: нет plaintext nonce, payload нечитаем

## Порядок реализации

1. **Config models first** — новый `ObfuscationCfg`, `tls.utls`, `sni`, `ws_paths`. Без этого нельзя писать транспорт.
2. **Server WS path** (AC-003) — простейшее изменение, сразу проверяемо.
3. **uTLS + fallback** (AC-001, AC-002) — ключевой MVP, даёт 80% эффекта.
4. **SNI** (AC-004) — `tls.sni: ["domain1", "domain2"]`, рандомный выбор. Тривиально, `rand.Intn` при connect. Работает и для WS (`tls.Config.ServerName`), и для QUIC (quic-go использует тот же `tls.Config`).
5. **Padding** (AC-005) — требует BatchWriter + серверную распаковку.
6. **QUIC обфускация** (AC-006) — наиболее рискованная (меняется протокол), последней.

Пункты 2-4 можно безопасно параллелить.

## Риски

- **uTLS + quic-go несовместимость**: quic-go использует `tls.Config` внутренне. `tls.Config.GetConfigForClient` возвращает `*tls.Config`, uTLS несовместим. Решение: AC-001/AC-006 — QUIC остаётся на `crypto/tls`, uTLS только для WS.
- **`obfuscation: true` в старых конфигах**: mapstructure по умолчанию не разрулит bool→struct. Решение: кастомный decoder в `LoadClientConfig`.
- **TLS Exporter недоступен**: если `tls.ConnectionState` не вызван после handshake или handshake не завершён. Решение: deferred nonce init, проверка `ConnectionState.HandshakeComplete`.
- **gorilla/websocket compression + padding**: compression удалён из проекта — padding не требует отключения.

## Rollout и compatibility

- `obfuscation: true` → парсится как `{enabled: true}` — старые конфиги работают
- Все новые опции опциональны (feature-flag: `enabled: false` по умолчанию)
- Сервер без `ws_paths` → только `/tunnel` (текущее поведение)
- QUIC протокол меняется: старый клиент не соединится с новым сервером и наоборот. Решение: добавить версию протокола в handshake или флаг `obfuscation.enabled`.

## Проверка

| Шаг | Что проверяем | Инструмент | AC |
|-----|--------------|------------|----|
| 1 | JA3 Chrome, не Go | tcpdump + JA3 calc | AC-001 |
| 2 | fallback лог при отказе uTLS | провокация ошибки, `grep` лога | AC-002 |
| 3 | 404 на неизвестный WS path | curl | AC-003 |
| 4 | SNI в handshake | tcpdump | AC-004 |
| 5 | Размер WS пакета = кратному | tcpdump | AC-005 |
| 6 | Нет plaintext nonce в QUIC stream | tcpdump | AC-006 |
| 7 | Старый конфиг `obfuscation: true` загружается | go test | RQ-005 |
| 8 | Без новых опций — поведение не изменилось | go test ./... | RQ-008 |

## Соответствие конституции

нет конфликтов
