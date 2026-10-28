# Context — src/internal/transport/tls

## Purpose

Сборка `*tls.Config` для v2 server/client: TLS 1.3, mTLS и явные trust-политики (`verify`/`insecure`). uTLS-профили живут отдельно в `src/internal/tlsprofile`.

## Responsibilities

- Владеет: серверным TLS-конфигом (cert/key, client CA, `client_auth`), клиентским (`verify_mode`, CA, ServerName).
- Не владеет: uTLS-диалером (`tlsprofile`), чтением конфига (`config`), HTTP/2/QUIC транспортом.

## How it works

- `NewServerTLSConfig`/`FromMaterial`: TLS 1.3; `client_auth` = `""|request|require|verify`; для `require|verify` обязателен CA.
- `NewClientTLSConfigFromSettings`: `verify` (default) или `insecure`; `ca_file` → `RootCAs`; `server_name` → SNI/verify name.
- `NewClientTLSConfig(skipVerify)` — низкоуровневый конструктор; в v2 client используется settings-вариант.
- `NextProtos` (`h2`) выставляет transport/client; uTLS профиль выбирает `tlsprofile.Dialer`.

## Key files

- `tls.go` — конструкторы и mTLS-политика.

## Gotchas

- `verify_mode=insecure` отключает проверку сертификата, но не TLS-шифрование и не DPI-маскировку.
- `tls.sni` при `verify` должен совпадать с сертификатом; иначе используйте `server_name` или insecure.
- uTLS-конфиг не копирует `Certificates` (mTLS) в текущем пути.
