# Context — src/internal/transport/tls

## Purpose

Сборка `*tls.Config` для сервера и клиента KVN: TLS 1.3, mTLS-режимы, явные trust-политики (`verify`/`insecure`), маскировка ClientHello через uTLS и выбор SNI.

## Responsibilities

- Владеет: серверным TLS-конфигом (cert/key, client CA, `client_auth`), клиентским конфигом (`verify_mode`, CA, ServerName), uTLS-диалом, случайным выбором браузерного fingerprint'а и SNI.
- Не владеет: чтением конфига (`src/internal/config`), самим WebSocket/QUIC-соединением, решением, когда включать uTLS (это `FactoryConfig`/`WSConfig`).

## How it works

- `NewServerTLSConfig(certFile, keyFile, clientCAFile, clientAuth)` читает пару и вызывает `newServerTLSConfig`; `NewServerTLSConfigFromMaterial` — та же логика с готовым `tls.Certificate` и CA-PEM (для тестов).
- `client_auth`: `""` → `NoClientCert`, `request` → `RequestClientCert`, `require` → `RequireAndVerifyClientCert`, `verify` → `VerifyClientCertIfGiven`; для `require`/`verify` обязателен `ClientCAs`, иначе ошибка.
- `NewClientTLSConfigFromSettings`: `verify_mode` по умолчанию `verify`; `insecure` → `InsecureSkipVerify`; `ca_file` → `RootCAs`; `server_name` → `ServerName`; неизвестный режим — ошибка.
- `DialWithUTLS(ctx, network, addr, tlsCfg, fallback)`: TCP-dial → `utls.UClient` со случайным пресетом (Chrome/Firefox/Edge/Safari) → handshake; при ошибке и `fallback=true` переоткрывает TCP и делает обычный `crypto/tls` handshake.
- `SelectSNI(list)` возвращает случайный элемент списка или `""` для пустого — вызывающий подставляет его в `tls.Config.ServerName`.
- Обе стороны фиксируют `MinVersion = TLS 1.3`.

## Key files

- `src/internal/transport/tls/tls.go` — все конструкторы, uTLS-диал, выбор SNI.

## Public contracts

- `NewServerTLSConfig(certFile, keyFile, clientCAFile, clientAuth string) (*tls.Config, error)`.
- `NewServerTLSConfigFromMaterial(cert *tls.Certificate, clientCAPEM []byte, clientAuth string) (*tls.Config, error)`.
- `NewClientTLSConfig(skipVerify bool) *tls.Config`, `NewClientTLSConfigFromSettings(ClientTLSSettings) (*tls.Config, error)`.
- `ClientTLSSettings{CAFile, ServerName, VerifyMode}`.
- `DialWithUTLS(ctx, network, addr string, tlsCfg *tls.Config, fallback bool) (net.Conn, error)`, `SelectSNI([]string) string`.

## Dependencies

- Стандартная библиотека (`crypto/tls`, `crypto/x509`), `github.com/refraction-networking/utls`.

## Consumed by

- `src/internal/bootstrap/client/helpers.go` (клиентский конфиг), `src/internal/bootstrap/server`/`relay` (серверный), `src/internal/transport/websocket` (uTLS-dial).

## Gotchas

- `verify_mode` по умолчанию `verify`; пустой `server_name` означает проверку по адресу из dial, а `insecure` полностью отключает проверку цепочки.
- `NewClientTLSConfig(skipVerify)` — низкоуровневый конструктор без CA/режима; в клиенте используется `NewClientTLSConfigFromSettings`.
- Для `client_auth=require`/`verify` отсутствие CA — ошибка конструктора, а не молчаливое ослабление.
- uTLS-конфиг копирует только `ServerName`, `InsecureSkipVerify`, `RootCAs`, `MinVersion`, `NextProtos`; остальные поля обычного `tls.Config` (например, `Certificates` для mTLS) в uTLS-путь **не попадают**.
- Пресет uTLS выбирается случайно на каждый dial; поведение недетерминировано между подключениями (это задумано для маскировки).
- `SelectSNI` использует `math/rand/v2` и не криптостоек — это балансировка/маскировка, не секрет.
- `fallback` в `DialWithUTLS` не повторяет uTLS: после неудачи один раз пробуется обычный TLS, и всё.
