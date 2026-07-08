---
report_type: inspect
slug: mac-tun
status: pass
docs_language: ru
generated_at: 2026-07-08
---

# Inspect Report: mac-tun

## Scope

- snapshot: проверка spec macOS TUN support — 11 AC, 9 RQ, scope, допущения
- artifacts:
  - CONSTITUTION.md
  - specs/active/mac-tun/spec.md

## Verdict

- status: pass

## Errors

- none

## Warnings

- none (оба замечания исправлены: AC-002 дополнен unit-тестами, build.sh добавлен target `darwin`)
- Упоминание `golang.zx2c4.com/wireguard/tun` — допустимо (repo constraint, уже в go.mod).

## Questions

- Нет

## Suggestions

- RQ-006 ("корректно чистить") и AC-006 (Cleanup on disconnect) — OK, но на плане стоит явно указать, что `CleanupExcludeRoutes()` вызывается в `Close()` (паттерн из win-tun).
- LaunchDaemon plist: рекомендую положить готовый `.plist` в `scripts/` (как `build.sh`), а не генерировать скриптом — пользователь копирует в `/Library/LaunchDaemons/` и грузит.
- Для macOS имеет смысл сразу сделать `darwin` target в build.sh, но arm64 пока опционально — amd64 работает через Rosetta 2.

## Traceability

- 11 AC, 9 RQ — coverage: каждый RQ покрыт ≥ 1 AC, каждый AC имеет Given/When/Then
- Покрытие AC по surface (предварительно):
  - AC-001..AC-006, AC-010 → `src/internal/tun/tun_darwin.go`
  - AC-007 → `scripts/com.kvn.tun.plist`
  - AC-008 → `scripts/build.sh`
  - AC-009 → `src/internal/tun/tun_stub.go` (update build tag)
  - AC-011 → `src/internal/webui/server.go`

## Next Step

- safe to continue to plan
