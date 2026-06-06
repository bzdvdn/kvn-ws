---
report_type: inspect
slug: whitelist-obfuscation
status: pass
docs_language: ru
generated_at: 2026-06-06
---

# Inspect Report: whitelist-obfuscation

## Scope

- snapshot: усиление защиты VPN-туннеля от DPI и whitelist-блокировок: uTLS, кастомный WS path, SNI, WS padding, усиленная QUIC обфускация
- artifacts:
  - CONSTITUTION.md
  - specs/active/whitelist-obfuscation/spec.md

## Verdict

- status: pass

## Errors

- none

## Warnings

- none

## Questions

- none

## Suggestions

- none

## Traceability

- AC-001 — uTLS для WS (Chrome fingerprint)
- AC-002 — uTLS fallback на crypto/tls
- AC-003 — кастомный WS path из URL клиента, allowlist на сервере
- AC-004 — кастомный SNI для TLS handshake
- AC-005 — WS padding через BatchWriter (4B len prefix + payload + padding)
- AC-006 — усиленная QUIC обфускация (XOR всего payload, nonce через TLS Exporter RFC 5705)

Все 6 AC в формате Given/When/Then. Все open questions закрыты. Scope, вне scope, допущения, краевые случаи описаны.

## Next Step

- safe to continue to plan
