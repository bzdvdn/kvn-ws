# Summary: core-tunnel-mvp

## Goal

Самый короткий путь к работающему VPN-туннелю: TUN → WSS/TLS → handshake → auth → packet forwarding → IP pool. Gate: `ping <server_assigned_ip>`.

## Acceptance Criteria

| AC | Description |
|----|-------------|
| AC-001 | TUN device открывается и читает/пишет IP-пакеты |
| AC-002 | WebSocket-соединение клиент-сервер устанавливается |
| AC-003 | TLS 1.3 listener на сервере и TLS dial клиента |
| AC-004 | Бинарные фреймы кодируются и декодируются |
| AC-005 | Client-Server handshake с назначением session_id и IP |
| AC-006 | Bearer-token аутентификация отклоняет невалидный токен |
| AC-007 | IP-пакет клиента доходит до TUN сервера |
| AC-008 | Ответный IP-пакет возвращается клиенту |
| AC-009 | IP пул выделяет и освобождает адреса |
| AC-010 | Graceful shutdown закрывает все ресурсы без ошибок |

## Out of Scope

Split-tunnel, NAT, DNS resolver, keepalive/reconnect, метрики, persistence, IPv6, multi-client, admin API.

## Artifacts

- `specs/active/core-tunnel-mvp/spec.md`
- `specs/active/core-tunnel-mvp/spec.digest.md`
- `specs/active/core-tunnel-mvp/summary.md`
