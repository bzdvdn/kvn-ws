# Local Proxy Mode — Summary

**Goal:** Добавить режим клиента как локального SOCKS5/HTTP CONNECT прокси (default port 2310) для кросс-платформенной работы без root/админ прав.

## Acceptance Criteria

| AC | Description | Proof |
|---|---|---|
| AC-001 | SOCKS5 listener на localhost | Тест SOCKS5 round-trip с эхо-сервером |
| AC-002 | HTTP CONNECT handler | Тест CONNECT через http.Client |
| AC-003 | Mode flag --mode proxy/tun | Тест с mock config проверяет ветку |
| AC-004 | Порт/bind в конфиге | Тест проверяет listener на указанном адресе |
| AC-005 | Опциональная auth на прокси | Тест с неверным паролем получает отказ |
| AC-006 | Cross-platform без CGO | CI собирает на 3 ОС, smoke-test Linux |
| AC-007 | CIDR/domain exclusion | Тест Route() решения для proxy-режима |

## Out of Scope

- UDP через SOCKS5, TUN-режим, серверная часть (кроме FrameTypeProxy handler)
- Изменения handshake, сессий, аутентификации, IP-пула

## Open Questions

none
