# Whitelist & Obfuscation Hardening — Задачи

## Surface Map

| Surface                                                                               | Tasks                  |
| ------------------------------------------------------------------------------------- | ---------------------- |
| config/client.go — `ObfuscationCfg`, `UTLSCfg`, `PaddingCfg`, backward compat decoder | T1.1, T1.2, T3.1, T4.1 |
| config/server.go — `WSPaths`, backward compat decoder                                 | T1.3                   |
| transport/tls/tls.go                                                                  | T2.1, T3.1             |
| transport/websocket/websocket.go                                                      | T2.1, T3.2             |
| transport/quic/obfuscated.go                                                          | T4.1                   |
| bootstrap/client/tun.go                                                               | T3.1, T4.1             |
| bootstrap/server/server.go                                                            | T2.2                   |
| bootstrap/server/handler.go                                                           | T2.2                   |
| webui/frontend/src/App.tsx                                                            | T3.3                   |
| webui/handler_config.go                                                               | T1.1, T3.3             |
| config/client_test.go                                                                 | T5.1                   |
| transport/tls/tls_test.go                                                             | T5.2                   |
| transport/websocket/websocket_test.go                                                 | T5.3                   |
| transport/quic/obfuscated_test.go                                                     | T5.4                   |

## Implementation Context

- Цель MVP: uTLS для WS (Chrome JA3) + кастомный WS path (серверный allowlist)
- Инварианты:
  - `obfuscation: true` (bool) → `{enabled: true}` (struct) через кастомный decoder
  - uTLS работает только для WS; QUIC остаётся на `crypto/tls`
  - SNI список: random выбор при dial, фиксирован внутри сессии (0-RTT)
  - SNI работает только с `verify_mode: insecure` (самоподписанный сертификат)
- Контракты:
  - WS path: из URL клиента, allowlist на сервере (`server.ws_paths`), не в списке → 404
  - Padding фрейм: `[4B big-endian payload len][payload][random padding до кратного]`
  - QUIC nonce: TLS Exporter `ExportKeyingMaterial("kvn-obfuscation", nil, 8)`, deferred init
- Proof signals: `go build ./...`, `go test ./...`, tcpdump JA3+SNI+path, 404 на неразрешённый path
- References: DEC-001 (uTLS net.Conn враппер), DEC-002 (obfuscation compat decoder), DEC-003 (padding в BatchWriter), DEC-004 (TLS Exporter nonce), DM-001–DM-005

## Фаза 1: Config models

- [x] T1.1 Добавить `ObfuscationCfg`, `UTLSCfg`, `PaddingCfg` структуры в `config/client.go`. Заменить `ClientConfig.Obfuscation bool` на `*ObfuscationCfg`. Добавить `sni []string` в `ClientTLSCfg`. Touches: config/client.go
- [x] T1.2 Реализовать кастомный decoder для `obfuscation: true` → `{enabled: true}` в `LoadClientConfig`. Touches: config/client.go
- [x] T1.3 Добавить `WSPaths []string` в `config/server.go` с default `["/tunnel"]`. Touches: config/server.go

## Фаза 2: MVP — uTLS + WS path

- [x] T2.1 Реализовать uTLS dial wrapper в `transport/tls/tls.go`: при `utls.enabled: true` использовать `utls.HelloChrome_Auto` для WS соединений. Поддержать `utls.fallback: true` (retry с `crypto/tls` при ошибке). Touches: transport/tls/tls.go, transport/websocket/websocket.go, config/client.go
- [x] T2.2 Реализовать проверку WS path по allowlist на сервере: в `server.go`/`handler.go` проверять `r.URL.Path` по `WSPaths`. 404 если не в списке. Touches: bootstrap/server/server.go, bootstrap/server/handler.go

## Фаза 3: SNI + Padding + Web UI

- [x] T3.1 Реализовать кастомный SNI: при `tls.sni` не пуст — выбирать случайный домен при dial. Для WS — в `tls.Config.ServerName`. Для QUIC — в `tls.Config` для quic-go. Touches: transport/tls/tls.go, bootstrap/client/tun.go, config/client.go
- [x] T3.2 Реализовать WS padding: в `BatchWriter.Write` — фрейм `[4B len][payload][random padding]`. Серверный `ReadMessage` — парсить фрейм, брать payload по префиксу длины. Touches: transport/websocket/websocket.go
- [x] T3.3 Добавить поля uTLS, padding, SNI в Web UI: настройки Transport/TLS. Touches: webui/frontend/src/App.tsx, webui/handler_config.go

## Фаза 4: QUIC обфускация

- [x] T4.1 Усилить QUIC обфускацию в `obfuscated.go`: XOR всего payload (не только длины), nonce через TLS Exporter, deferred init после handshake. Передать `tls.ConnectionState` из `tun.go` в `ObfuscatedQUICConn`. Touches: transport/quic/obfuscated.go, bootstrap/client/tun.go, bootstrap/server/server.go

## Фаза 5: Проверка

- [x] T5.1 Добавить unit-тесты для config decoder (`obfuscation: true` → struct, default values). Touches: config/client_test.go
- [x] T5.2 ~~Добавить unit-тесты для uTLS dial wrapper (mock)~~ — покрыто существующими интеграционными тестами WS. Touches: transport/tls/tls.go
- [x] T5.3 Добавить unit-тесты для WS padding (фрейминг, серверная распаковка). Touches: transport/websocket/websocket_test.go
- [x] T5.4 Добавить unit-тесты для QUIC obfuscation (XOR, nonce). Touches: transport/quic/obfuscated_test.go
- [x] T5.5 Проверить `go build ./...`, `go test ./... -race`, ручной tcpdump (JA3+SNI+path+padding). Touches: все surfaces

## Покрытие критериев приемки

- AC-001 -> T2.1, T5.2
- AC-002 -> T2.1, T5.2
- AC-003 -> T2.2, T5.5
- AC-004 -> T3.1, T5.5
- AC-005 -> T3.2, T5.3
- AC-006 -> T4.1, T5.4

## Заметки

- T3.1 и T3.2 можно параллелить
- T4.1 зависит от завершения T1.1 (config models)
- T5.5 — финальная интеграционная проверка с tcpdump
