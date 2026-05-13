# Performance & Polish — Summary

**Inspect Verdict:** pass

**Goal:** Оптимизировать производительность WebSocket-туннеля kvn-ws: buffer pooling, TCP_NODELAY, batch writes, MTU/PMTU, компрессия, опциональное мультиплексирование. Gate: throughput ≥80% от raw TCP, latency overhead ≤15% при 1000+ сессий.

## Acceptance Criteria

| AC | Description | Proof |
|---|---|---|
| AC-001 | sync.Pool для буферов encode/decode | benchstat показывает снижение аллокаций ≥80% |
| AC-002 | TCP_NODELAY на WS-соединениях | Тест проверяет NoDelay() после upgrade |
| AC-003 | Batch writes | Тест с mock conn считает вызовы WriteMessage |
| AC-004 | MTU negotiation | Тест handshake проверяет согласованный MTU |
| AC-005 | PMTU strategy | Тест отправки фрейма >MTU проверяет сегментацию |
| AC-006 | Payload compression | Тест проверяет compressed size < original |
| AC-007 | Multiplex channels | Интеграционный тест с двумя независимыми каналами |
| AC-008 | Load testing gate 1000+ | CI-скрипт выводит throughput/latency pass/fail |

## Out of Scope

- Изменение протокола рукопожатия, архитектуры сессий, маршрутизации, шифрования, аутентификации
- Переход на другой транспорт (не WebSocket)
- Оптимизация TUN-устройства

## Open Questions

1. RESOLVED: permessage-deflate (WebSocket compression extension) — совместимо с прокси
2. RESOLVED: Multiplex через WebSocket subprotocol
3. RESOLVED: Отдельный конфигурационный файл для load testing стенда
