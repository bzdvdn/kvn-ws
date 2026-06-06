# Data Model — import-export--qr-config-ui

## Config JSON Format

Импорт/экспорт использует тот же JSON, что и `/api/config` — `ClientConfig` интерфейс.

**Формат:** минимизированный JSON (без пробелов, без переносов).

## Example

```json
{"server":"wss://example.com/api/v1/events","transport":"tcp","obfuscation":{"enabled":true,"utls":{"enabled":true},"padding":{"enabled":true,"size":512}},"tls":{"verify_mode":"insecure","sni":["www.cloudflare.com","www.google.com"]},"auth":{"token":"secret"},"mtu":1400,"mode":"proxy","proxy_listen":"127.0.0.1:2310","log":{"level":"info"}}
```

## Constraints

- Поля, отсутствующие в JSON при импорте, не сбрасываются — остаются текущие значения из формы
- Неизвестные поля игнорируются (forward compat)
- Token экспортируется — пользователь предупреждён о чувствительных данных
